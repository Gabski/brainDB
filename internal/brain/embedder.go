// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// An Embedder turns text into vectors. The built-in hash embedder is deterministic, offline and free; a remote
// embedder calls any OpenAI-compatible /embeddings endpoint (OpenRouter, OpenAI, Ollama, LM Studio, vLLM, Jina,
// Voyage, a local TEI server). Every implementation returns L2-normalised vectors of Dim() floats.
type Embedder interface {
	Name() string
	Dim() int
	// Embed returns one vector per input text, in order. It never returns fewer vectors than inputs.
	Embed(texts []string) ([][]float32, error)
}

// ---------------------------------------------------------------- hash embedder

type hashEmbedder struct{ dim int }

func (h hashEmbedder) Name() string { return fmt.Sprintf("hash-%d", h.dim) }
func (h hashEmbedder) Dim() int     { return h.dim }

func (h hashEmbedder) Embed(texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = hashEmbedDim(t, h.dim)
	}
	return out, nil
}

// ---------------------------------------------------------------- remote embedder

// EmbedderOptions configures a remote embedder. Zero values fall back to sane defaults.
type EmbedderOptions struct {
	Provider  string // "hash" (default), or a name from EmbeddingProviders(): openai, cohere, gemini, voyage, …
	Model     string // provider model id, e.g. "openai/text-embedding-3-small" on OpenRouter
	BaseURL   string // overrides the provider's default endpoint; required for self-hosted providers
	APIKeyEnv string // name of the env var holding the key; the key itself is never stored in config or brain.xml
	Dim       int    // expected vector size; 0 = learn it from the first response
	Batch     int    // texts per request (default 64)
	Timeout   int    // seconds per request (default 30)
	Retries   int    // retries on 429/5xx; 0 = default (2), negative = never retry
	Fallback  string // "hash" (default) = degrade to the hash embedder on failure | "error" = fail loudly
	Cache     string // path to the on-disk vector cache; "" = cache next to the brain
	Title     string // optional client label sent as X-Title; defaults to "braindb"
	// InputType is the asymmetric-model hint: Cohere's input_type, Gemini's taskType, Voyage's input_type.
	// It defaults to the provider's "document" value, because a brain embeds what it stores.
	InputType string
	// APIVersion is the Azure OpenAI api-version query parameter; ignored by every other provider.
	APIVersion string
}

type remoteEmbedder struct {
	o         EmbedderOptions
	spec      EmbedProvider
	inputType string
	url       string
	key       string
	dim       int
	batch     int
	retries   int
	fallback  bool
	client    *http.Client
	cache     *embedCache
	backoff   time.Duration // base retry delay; doubled per attempt, capped at 8s

	mu       sync.Mutex
	degraded string // non-empty once the embedder has fallen back; surfaced in status lines
}

// NewEmbedder builds an Embedder from options. Provider "" or "hash" returns the built-in hash embedder, so a brain
// with no embedding configuration behaves exactly as before.
func NewEmbedder(o EmbedderOptions, cacheDir string) (Embedder, error) {
	switch strings.ToLower(strings.TrimSpace(o.Provider)) {
	case "", "hash", "local":
		d := o.Dim
		if d <= 0 {
			d = embDim
		}
		return hashEmbedder{dim: d}, nil
	}
	spec, ok := lookupEmbedProvider(o.Provider)
	if !ok {
		return nil, fmt.Errorf("embedding.provider: unknown provider %q (known: %s)", o.Provider, strings.Join(embedProviderNames(), ", "))
	}
	if o.Model == "" {
		return nil, fmt.Errorf("embedding.model is required for provider %q", spec.Name)
	}
	base := o.BaseURL
	if base == "" {
		base = spec.BaseURL
	}
	if base == "" {
		return nil, fmt.Errorf("embedding.baseUrl is required for provider %q: %s", spec.Name, spec.Note)
	}
	key := ""
	if o.APIKeyEnv != "" {
		key = strings.TrimSpace(os.Getenv(o.APIKeyEnv))
		if key == "" {
			return nil, fmt.Errorf("embedding.apiKeyEnv: %s is not set", o.APIKeyEnv)
		}
	}
	batch := orInt(o.Batch, 64)
	if spec.MaxBatch > 0 && batch > spec.MaxBatch {
		batch = spec.MaxBatch
	}
	inputType := o.InputType
	if inputType == "" {
		inputType = spec.InputType
	}
	e := &remoteEmbedder{
		o:         o,
		spec:      spec,
		inputType: inputType,
		url:       spec.endpoint(base, o.Model, o.APIVersion),
		key:       key,
		dim:       o.Dim,
		batch:     batch,
		retries:   retryCount(o.Retries),
		fallback:  !strings.EqualFold(o.Fallback, "error"),
		client:    &http.Client{Timeout: time.Duration(orInt(o.Timeout, 30)) * time.Second},
		backoff:   250 * time.Millisecond,
	}
	path := o.Cache
	if path == "" && cacheDir != "" {
		path = filepath.Join(cacheDir, "embeddings.cache")
	}
	if path != "" {
		e.cache = openEmbedCache(path, o.Model)
		if e.dim == 0 && e.cache != nil {
			e.dim = e.cache.dim
		}
	}
	return e, nil
}

// retryCount maps the option onto an attempt budget: 0 means "unset, use the default", negative means "none".
func retryCount(v int) int {
	if v < 0 {
		return 0
	}
	return orInt(v, 2)
}

func orInt(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

func (e *remoteEmbedder) Name() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.degraded != "" {
		return e.o.Model + " (degraded: hash)"
	}
	return e.o.Model
}

func (e *remoteEmbedder) Dim() int {
	if e.dim > 0 {
		return e.dim
	}
	return embDim
}

// Degraded reports the last failure that forced a fallback to hash vectors, or "" when the embedder is healthy.
func (e *remoteEmbedder) Degraded() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.degraded
}

func (e *remoteEmbedder) Embed(texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	miss, missIdx := []string{}, []int{}
	for i, t := range texts {
		if v := e.cache.get(t); v != nil {
			out[i] = v
			continue
		}
		miss = append(miss, t)
		missIdx = append(missIdx, i)
	}
	for off := 0; off < len(miss); off += e.batch {
		end := min(off+e.batch, len(miss))
		vecs, err := e.call(miss[off:end])
		if err != nil {
			if !e.fallback {
				return nil, err
			}
			e.mu.Lock()
			e.degraded = err.Error()
			e.mu.Unlock()
			for i := off; i < end; i++ {
				out[missIdx[i]] = hashEmbedDim(miss[i], e.Dim())
			}
			continue
		}
		for i, v := range vecs {
			out[missIdx[off+i]] = v
			e.cache.put(miss[off+i], v)
		}
	}
	e.cache.flush()
	return out, nil
}

type embedReq struct {
	Model          string   `json:"model"`
	Input          []string `json:"input"`
	EncodingFormat string   `json:"encoding_format"`
}

type embedResp struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (e *remoteEmbedder) call(texts []string) ([][]float32, error) {
	body, err := encodeEmbedRequest(e.spec, e.o.Model, e.inputType, texts)
	if err != nil {
		return nil, err
	}
	var last error
	for attempt := 0; attempt <= e.retries; attempt++ {
		if attempt > 0 {
			d := e.backoff << (attempt - 1)
			if d > 8*time.Second {
				d = 8 * time.Second
			}
			time.Sleep(d)
		}
		req, err := http.NewRequest(http.MethodPost, e.url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if e.key != "" {
			name, prefix := e.spec.authHeader()
			req.Header.Set(name, prefix+e.key)
		}
		title := e.o.Title
		if title == "" {
			title = "braindb"
		}
		req.Header.Set("X-Title", title)
		resp, err := e.client.Do(req)
		if err != nil {
			last = err
			continue
		}
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			last = fmt.Errorf("%s: %s", resp.Status, trimErr(data))
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s: %s", resp.Status, trimErr(data))
		}
		out, err := decodeEmbedResponse(e.spec, data, len(texts))
		if err != nil {
			return nil, err
		}
		for i, v := range out {
			if v != nil {
				out[i] = vnorm(v)
			}
		}
		for i, v := range out {
			if v == nil {
				return nil, fmt.Errorf("embedding api returned no vector for input %d", i)
			}
			if e.dim == 0 {
				e.dim = len(v)
				e.cache.setDim(e.dim)
			}
			if len(v) != e.dim {
				return nil, fmt.Errorf("embedding api returned %d dims, expected %d", len(v), e.dim)
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("embedding request failed after %d attempts: %v", e.retries+1, last)
}

func trimErr(d []byte) string {
	s := strings.TrimSpace(string(d))
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}

// ---------------------------------------------------------------- on-disk vector cache

// embedCache is an append-only file of sha256(model+text) → vector. Remote embeddings cost money and latency, so the
// cache is the difference between a one-off import bill and paying again on every restart. A corrupt or stale file
// (different model or dimension) is simply rewritten from scratch; the cache is never authoritative.
type embedCache struct {
	mu      sync.Mutex
	path    string
	model   string
	dim     int
	m       map[[32]byte][]float32
	pending []byte
	broken  bool
}

const embedCacheMagic = "BDBEMB01"

func openEmbedCache(path, model string) *embedCache {
	c := &embedCache{path: path, model: model, m: map[[32]byte][]float32{}}
	d, err := os.ReadFile(path)
	if err != nil || len(d) < len(embedCacheMagic)+6 {
		return c
	}
	if string(d[:len(embedCacheMagic)]) != embedCacheMagic {
		return c
	}
	p := len(embedCacheMagic)
	dim := int(binary.LittleEndian.Uint32(d[p:]))
	p += 4
	ml := int(binary.LittleEndian.Uint16(d[p:]))
	p += 2
	if dim <= 0 || dim > 1<<16 || p+ml > len(d) || string(d[p:p+ml]) != model {
		return c // different model or garbage: start over
	}
	p += ml
	rec := 32 + dim*4
	for p+rec <= len(d) {
		var k [32]byte
		copy(k[:], d[p:p+32])
		v := make([]float32, dim)
		for i := range v {
			v[i] = f32(binary.LittleEndian.Uint32(d[p+32+i*4:]))
		}
		c.m[k] = v
		p += rec
	}
	c.dim = dim
	return c
}

func (c *embedCache) setDim(dim int) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dim == 0 {
		c.dim = dim
	}
}

func (c *embedCache) key(text string) [32]byte {
	return sha256.Sum256([]byte(c.model + "\x00" + text))
}

func (c *embedCache) get(text string) []float32 {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m[c.key(text)]
}

func (c *embedCache) put(text string, v []float32) {
	if c == nil || len(v) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dim == 0 {
		c.dim = len(v)
	}
	if len(v) != c.dim {
		return
	}
	k := c.key(text)
	if _, ok := c.m[k]; ok {
		return
	}
	c.m[k] = v
	buf := make([]byte, 32+len(v)*4)
	copy(buf, k[:])
	for i, x := range v {
		binary.LittleEndian.PutUint32(buf[32+i*4:], u32(x))
	}
	c.pending = append(c.pending, buf...)
}

func (c *embedCache) flush() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.pending) == 0 || c.broken || c.dim == 0 {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		c.broken = true
		return
	}
	st, err := os.Stat(c.path)
	if err != nil || st.Size() == 0 {
		head := []byte(embedCacheMagic)
		head = binary.LittleEndian.AppendUint32(head, uint32(c.dim))
		head = binary.LittleEndian.AppendUint16(head, uint16(len(c.model)))
		head = append(head, c.model...)
		// rewrite the whole cache: header plus every vector held in memory
		body := make([]byte, 0, len(c.m)*(32+c.dim*4))
		for k, v := range c.m {
			body = append(body, k[:]...)
			for _, x := range v {
				body = binary.LittleEndian.AppendUint32(body, u32(x))
			}
		}
		if os.WriteFile(c.path, append(head, body...), 0o644) != nil {
			c.broken = true
		}
		c.pending = nil
		return
	}
	f, err := os.OpenFile(c.path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		c.broken = true
		return
	}
	defer f.Close()
	if _, err := f.Write(c.pending); err != nil {
		c.broken = true
		return
	}
	c.pending = nil
}

func f32(u uint32) float32 { return math.Float32frombits(u) }
func u32(f float32) uint32 { return math.Float32bits(f) }

// ApplyEmbedder swaps the vector engine and re-embeds the whole brain. A nil-provider option set selects the
// built-in hash embedder, so calling it with a zero value is always safe.
func (b *Brain) ApplyEmbedder(o EmbedderOptions) error {
	em, err := NewEmbedder(o, b.Dir)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.embedder, b.pvecs = em, nil
	b.embedAll()
	b.refresh()
	return nil
}
