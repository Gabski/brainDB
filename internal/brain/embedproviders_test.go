// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// capture records what a provider actually received, because the point of a provider table is the wire format.
type capture struct {
	path   string
	query  string
	header http.Header
	body   map[string]any
}

func fakeProvider(t *testing.T, dim int, respond func(n int) any, got *capture) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		got.path, got.query, got.header = r.URL.Path, r.URL.RawQuery, r.Header.Clone()
		got.body = map[string]any{}
		if err := json.Unmarshal(data, &got.body); err != nil {
			t.Errorf("request body is not JSON: %v", err)
		}
		n := 0
		switch {
		case got.body["input"] != nil:
			n = len(got.body["input"].([]any))
		case got.body["texts"] != nil:
			n = len(got.body["texts"].([]any))
		case got.body["requests"] != nil:
			n = len(got.body["requests"].([]any))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(respond(n))
	}))
	t.Cleanup(srv.Close)
	_ = dim
	return srv
}

func ramp(n, dim int) [][]float32 {
	out := make([][]float32, n)
	for i := range out {
		v := make([]float32, dim)
		for j := range v {
			v[j] = float32(i+1) * float32(j+1)
		}
		out[i] = v
	}
	return out
}

func TestCohereWireFormat(t *testing.T) {
	var got capture
	srv := fakeProvider(t, 4, func(n int) any {
		return map[string]any{"embeddings": map[string]any{"float": ramp(n, 4)}}
	}, &got)
	e, err := NewEmbedder(EmbedderOptions{Provider: "cohere", Model: "embed-v4.0", BaseURL: srv.URL}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.Embed([]string{"alpha", "beta"})
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 2 || len(v[0]) != 4 {
		t.Fatalf("want 2 vectors of 4, got %d", len(v))
	}
	if got.path != "/v2/embed" {
		t.Errorf("cohere posts to /v2/embed, got %q", got.path)
	}
	if got.body["texts"] == nil || got.body["input"] != nil {
		t.Errorf("cohere takes texts, not input: %v", got.body)
	}
	if got.body["input_type"] != "search_document" {
		t.Errorf("the document input_type must be sent by default, got %v", got.body["input_type"])
	}
	if ts, _ := got.body["embedding_types"].([]any); len(ts) != 1 || ts[0] != "float" {
		t.Errorf("embedding_types must ask for float, got %v", got.body["embedding_types"])
	}
	if got.header.Get("Authorization") != "" {
		t.Errorf("no apiKeyEnv was configured, so no Authorization header should be sent")
	}
	// vectors come back normalised like every other provider's
	var s float64
	for _, f := range v[1] {
		s += float64(f) * float64(f)
	}
	if math.Abs(s-1) > 1e-6 {
		t.Errorf("cohere vectors must be L2-normalised too: |v|² = %v", s)
	}
}

func TestGeminiWireFormat(t *testing.T) {
	var got capture
	srv := fakeProvider(t, 3, func(n int) any {
		out := make([]any, n)
		for i, v := range ramp(n, 3) {
			out[i] = map[string]any{"values": v}
		}
		return map[string]any{"embeddings": out}
	}, &got)
	e, err := NewEmbedder(EmbedderOptions{Provider: "google", Model: "text-embedding-004", BaseURL: srv.URL}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Embed([]string{"alpha", "beta", "gamma"}); err != nil {
		t.Fatal(err)
	}
	if got.path != "/models/text-embedding-004:batchEmbedContents" {
		t.Errorf("gemini puts the model in the path, got %q", got.path)
	}
	reqs, _ := got.body["requests"].([]any)
	if len(reqs) != 3 {
		t.Fatalf("gemini batches one request per text, got %v", got.body)
	}
	first, _ := reqs[0].(map[string]any)
	if first["model"] != "models/text-embedding-004" {
		t.Errorf("each request repeats the model with its models/ prefix, got %v", first["model"])
	}
	if first["taskType"] != "RETRIEVAL_DOCUMENT" {
		t.Errorf("the document taskType must be sent by default, got %v", first["taskType"])
	}
}

func TestAzureUrlAndAuthHeader(t *testing.T) {
	var got capture
	srv := fakeProvider(t, 2, func(n int) any {
		data := make([]any, n)
		for i, v := range ramp(n, 2) {
			data[i] = map[string]any{"index": i, "embedding": v}
		}
		return map[string]any{"data": data}
	}, &got)
	t.Setenv("AZ_KEY", "secret")
	e, err := NewEmbedder(EmbedderOptions{Provider: "azure", Model: "my-deployment", BaseURL: srv.URL, APIKeyEnv: "AZ_KEY"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Embed([]string{"alpha"}); err != nil {
		t.Fatal(err)
	}
	if got.path != "/openai/deployments/my-deployment/embeddings" {
		t.Errorf("azure puts the deployment in the path, got %q", got.path)
	}
	if !strings.Contains(got.query, "api-version=") {
		t.Errorf("azure needs an api-version query, got %q", got.query)
	}
	if got.header.Get("api-key") != "secret" {
		t.Errorf("azure authenticates with the api-key header, got %q", got.header.Get("api-key"))
	}
	if got.header.Get("Authorization") != "" {
		t.Errorf("azure must not also send a bearer token")
	}
}

func TestOpenAiWireSendsInputTypeOnlyWhenAsked(t *testing.T) {
	respond := func(n int) any {
		data := make([]any, n)
		for i, v := range ramp(n, 2) {
			data[i] = map[string]any{"index": i, "embedding": v}
		}
		return map[string]any{"data": data}
	}
	var plain, typed capture
	s1 := fakeProvider(t, 2, respond, &plain)
	s2 := fakeProvider(t, 2, respond, &typed)

	e1, err := NewEmbedder(EmbedderOptions{Provider: "openai", Model: "m", BaseURL: s1.URL}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e1.Embed([]string{"x"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := plain.body["input_type"]; ok {
		t.Error("plain OpenAI servers reject unknown fields, so input_type must be left out")
	}

	e2, err := NewEmbedder(EmbedderOptions{Provider: "voyage", Model: "voyage-3", BaseURL: s2.URL}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e2.Embed([]string{"x"}); err != nil {
		t.Fatal(err)
	}
	if typed.body["input_type"] != "document" {
		t.Errorf("voyage asks for an input_type, got %v", typed.body["input_type"])
	}
}

func TestProviderBatchCeiling(t *testing.T) {
	var got capture
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		data, _ := io.ReadAll(r.Body)
		got.body = map[string]any{}
		_ = json.Unmarshal(data, &got.body)
		n := len(got.body["texts"].([]any))
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": map[string]any{"float": ramp(n, 2)}})
	}))
	t.Cleanup(srv.Close)
	// Cohere caps a request at 96 texts; a config asking for 500 must be clamped, not rejected by the provider.
	e, err := NewEmbedder(EmbedderOptions{Provider: "cohere", Model: "embed-v4.0", BaseURL: srv.URL, Batch: 500}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	in := make([]string, 100)
	for i := range in {
		in[i] = string(rune('a'+i%26)) + strings.Repeat("x", i)
	}
	if _, err := e.Embed(in); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("100 texts at a ceiling of 96 is two requests, got %d", calls)
	}
}

func TestProviderRegistryAndErrors(t *testing.T) {
	if _, err := NewEmbedder(EmbedderOptions{Provider: "wat", Model: "m"}, t.TempDir()); err == nil || !strings.Contains(err.Error(), "known:") {
		t.Errorf("an unknown provider must list the known ones, got %v", err)
	}
	// self-hosted providers have no default endpoint, and saying so beats a connection error later
	if _, err := NewEmbedder(EmbedderOptions{Provider: "vllm", Model: "m"}, t.TempDir()); err == nil || !strings.Contains(err.Error(), "baseUrl") {
		t.Errorf("vllm must require a baseUrl, got %v", err)
	}
	if _, err := NewEmbedder(EmbedderOptions{Provider: "cohere"}, t.TempDir()); err == nil {
		t.Error("a remote provider without a model must fail")
	}
	ps := EmbeddingProviders()
	if len(ps) < 10 || ps[0].Name != "hash" {
		t.Fatalf("the provider list must start with hash and carry the remote ones, got %d entries", len(ps))
	}
	seen := map[string]bool{}
	for _, p := range ps {
		if seen[p.Name] {
			t.Errorf("duplicate provider %q", p.Name)
		}
		seen[p.Name] = true
		if p.Wire != "hash" && p.Label == "" {
			t.Errorf("provider %q has no label", p.Name)
		}
	}
	for _, want := range []string{"openai", "azure", "cohere", "gemini", "voyage", "ollama", "mistral", "jina"} {
		if !seen[want] {
			t.Errorf("provider %q is missing from the registry", want)
		}
	}
	// aliases resolve, so a config written against an older spelling keeps working
	for alias, target := range map[string]string{"google": "gemini", "hf": "huggingface", "custom": "compatible"} {
		if p, ok := lookupEmbedProvider(alias); !ok || p.Name != target {
			t.Errorf("alias %q should resolve to %q", alias, target)
		}
	}
}

func TestProviderConfigValidation(t *testing.T) {
	for _, src := range []string{
		"embedding: {provider: cohere}\n",                               // no model
		"embedding: {provider: vllm, model: m}\n",                       // no baseUrl
		"embedding: {provider: openai, model: m, apiVersion: '2024'}\n", // api-version is azure-only
	} {
		if _, errs := ParseConfig(src); len(errs) == 0 {
			t.Errorf("expected a validation error for %q", src)
		}
	}
	cfg, errs := ParseConfig("embedding:\n  provider: cohere\n  model: embed-v4.0\n  inputType: search_query\n")
	if len(errs) > 0 {
		t.Fatalf("valid cohere config rejected: %v", errs)
	}
	if o := cfg.embedderOptions(); o.InputType != "search_query" {
		t.Errorf("inputType must reach the embedder options, got %q", o.InputType)
	}
}
