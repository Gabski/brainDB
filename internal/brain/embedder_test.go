// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// fakeEmbedAPI is a minimal OpenAI-compatible /embeddings endpoint. It returns a deterministic vector per text so
// tests can assert which texts were actually sent over the wire.
func fakeEmbedAPI(t *testing.T, dim int, calls, texts *int64, fail func(n int64) int) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(calls, 1)
		if fail != nil {
			if code := fail(n); code != 0 {
				w.WriteHeader(code)
				_, _ = w.Write([]byte(`{"error":{"message":"nope"}}`))
				return
			}
		}
		var req embedReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		atomic.AddInt64(texts, int64(len(req.Input)))
		out := embedResp{}
		for i, in := range req.Input {
			v := make([]float32, dim)
			for j := range v {
				v[j] = float32(math.Sin(float64(hash(in)%1000)*.01 + float64(j)))
			}
			out.Data = append(out.Data, struct {
				Index     int       `json:"index"`
				Embedding []float32 `json:"embedding"`
			}{Index: i, Embedding: v})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(s.Close)
	return s
}

func TestRemoteEmbedderBatchesAndNormalises(t *testing.T) {
	var calls, texts int64
	srv := fakeEmbedAPI(t, 8, &calls, &texts, nil)
	e, err := NewEmbedder(EmbedderOptions{Provider: "openai", Model: "m1", BaseURL: srv.URL, Batch: 2}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	in := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	v, err := e.Embed(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != len(in) {
		t.Fatalf("want %d vectors, got %d", len(in), len(v))
	}
	if calls != 3 { // 5 texts, batch 2 → 2+2+1
		t.Fatalf("want 3 batched requests, got %d", calls)
	}
	if e.Dim() != 8 {
		t.Fatalf("dimension should be learned from the response, got %d", e.Dim())
	}
	for i, x := range v {
		s := 0.0
		for _, f := range x {
			s += float64(f) * float64(f)
		}
		if math.Abs(s-1) > 1e-6 {
			t.Fatalf("vector %d is not L2-normalised: |v|² = %v", i, s)
		}
	}
	// identical texts must give identical vectors, different texts must not
	if vcos(v[0], v[1]) > .9999 {
		t.Fatal("different texts produced the same vector")
	}
}

func TestRemoteEmbedderCachePersists(t *testing.T) {
	var calls, texts int64
	srv := fakeEmbedAPI(t, 8, &calls, &texts, nil)
	dir := t.TempDir()
	opts := EmbedderOptions{Provider: "openai", Model: "m1", BaseURL: srv.URL}

	e1, err := NewEmbedder(opts, dir)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := e1.Embed([]string{"alpha", "beta"})
	if texts != 2 {
		t.Fatalf("want 2 texts embedded, got %d", texts)
	}
	if _, err := os.Stat(filepath.Join(dir, "embeddings.cache")); err != nil {
		t.Fatalf("cache file was not written: %v", err)
	}

	// a fresh embedder over the same directory must serve from disk and send only the new text
	e2, err := NewEmbedder(opts, dir)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := e2.Embed([]string{"alpha", "beta", "gamma"})
	if texts != 3 {
		t.Fatalf("want 1 more text over the wire (3 total), got %d", texts)
	}
	if vcos(first[0], second[0]) < .9999 {
		t.Fatal("cached vector differs from the original")
	}

	// a different model must not read the previous model's vectors
	e3, err := NewEmbedder(EmbedderOptions{Provider: "openai", Model: "m2", BaseURL: srv.URL}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e3.Embed([]string{"alpha"}); err != nil {
		t.Fatal(err)
	}
	if texts != 4 {
		t.Fatalf("a model change must invalidate the cache; texts = %d", texts)
	}
}

func TestRemoteEmbedderFallbackAndStrictMode(t *testing.T) {
	var calls, texts int64
	srv := fakeEmbedAPI(t, 8, &calls, &texts, func(int64) int { return http.StatusInternalServerError })

	// default: degrade to hash vectors of the right dimension rather than lose the import
	e, err := NewEmbedder(EmbedderOptions{Provider: "openai", Model: "m1", BaseURL: srv.URL, Dim: 8, Retries: -1}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e.(*remoteEmbedder).backoff = time.Millisecond
	v, err := e.Embed([]string{"alpha"})
	if err != nil {
		t.Fatalf("fallback mode must not return an error: %v", err)
	}
	if len(v[0]) != 8 {
		t.Fatalf("fallback vector has the wrong dimension: %d", len(v[0]))
	}
	if d := e.(*remoteEmbedder).Degraded(); d == "" {
		t.Fatal("a degraded embedder must report why")
	}

	// strict: fail loudly instead
	e2, err := NewEmbedder(EmbedderOptions{Provider: "openai", Model: "m1", BaseURL: srv.URL, Dim: 8, Retries: -1, Fallback: "error"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e2.(*remoteEmbedder).backoff = time.Millisecond
	if _, err := e2.Embed([]string{"alpha"}); err == nil {
		t.Fatal("fallback: error must surface the failure")
	}
}

func TestEmbedderConfigValidation(t *testing.T) {
	bad := []string{
		"embedding: {provider: wat}\n",
		"embedding: {provider: openai}\n",                        // no model
		"embedding: {baseUrl: 'http://x'}\n",                     // remote options without a remote provider
		"embedding: {provider: openai, model: m, fallback: x}\n", // bad fallback
	}
	for _, src := range bad {
		if _, errs := ParseConfig(src); len(errs) == 0 {
			t.Fatalf("expected a validation error for %q", src)
		}
	}
	cfg, errs := ParseConfig("embedding:\n  provider: openai\n  model: openai/text-embedding-3-small\n  baseUrl: https://openrouter.ai/api/v1\n  apiKeyEnv: OPENROUTER_API_KEY\n  dim: 1536\n")
	if len(errs) > 0 {
		t.Fatalf("valid config rejected: %v", errs)
	}
	o := cfg.embedderOptions()
	if o.Provider != "openai" || o.Model != "openai/text-embedding-3-small" || o.Dim != 1536 || o.APIKeyEnv != "OPENROUTER_API_KEY" {
		t.Fatalf("options not mapped: %+v", o)
	}
	// a missing key env var must fail at construction, not silently degrade the whole brain to hash vectors
	os.Unsetenv("OPENROUTER_API_KEY")
	if _, err := NewEmbedder(o, t.TempDir()); err == nil {
		t.Fatal("a missing apiKeyEnv must be an error")
	}
}

func TestHashEmbedderIsStillTheDefault(t *testing.T) {
	e, err := NewEmbedder(EmbedderOptions{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if e.Name() != "hash-256" || e.Dim() != embDim {
		t.Fatalf("default embedder changed: %s/%d", e.Name(), e.Dim())
	}
	v, _ := e.Embed([]string{"renewal discount"})
	if vcos(v[0], HashEmbed("renewal discount")) < .9999 {
		t.Fatal("default embedder must be bit-compatible with HashEmbed")
	}
}

func TestRetryBudget(t *testing.T) {
	// 0 = unset → the default of two retries (three attempts); negative = no retry at all
	for _, c := range []struct{ opt, attempts int }{{0, 3}, {-1, 1}, {1, 2}, {5, 6}} {
		var calls, texts int64
		srv := fakeEmbedAPI(t, 4, &calls, &texts, func(int64) int { return http.StatusTooManyRequests })
		e, err := NewEmbedder(EmbedderOptions{Provider: "openai", Model: "m", BaseURL: srv.URL, Dim: 4,
			Retries: c.opt, Fallback: "error"}, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		e.(*remoteEmbedder).backoff = time.Millisecond
		if _, err := e.Embed([]string{"x"}); err == nil {
			t.Fatal("want an error")
		}
		if calls != int64(c.attempts) {
			t.Fatalf("retries %d: want %d attempts, got %d", c.opt, c.attempts, calls)
		}
	}
}
