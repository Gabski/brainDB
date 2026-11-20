// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

// Embedding providers (Addendum F8).
//
// Most hosted embedding APIs speak the OpenAI /embeddings wire format, so they differ only in a base URL, an auth
// header and a batch ceiling. Three do not: Cohere posts texts and returns embeddings grouped by type, Gemini posts
// one request per text inside a batch envelope, and Azure puts the deployment in the path with an api-version query.
// This file holds that knowledge as data, so adding a provider is a table entry, not a new code path.

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// EmbedProvider describes one named provider of the `embedding.provider` key.
type EmbedProvider struct {
	Name string `json:"name"`
	// Label is what a UI shows; it names the service, not the model.
	Label string `json:"label"`
	// Wire is the request/response format: openai, cohere or gemini.
	Wire string `json:"wire"`
	// BaseURL is the default endpoint. Empty means the provider is self-hosted and `embedding.baseUrl` is required.
	BaseURL string `json:"baseUrl,omitempty"`
	// KeyEnv is the environment variable the service conventionally uses; it is a hint for `embedding.apiKeyEnv`,
	// never read directly, because a key is only ever taken from the variable the config names.
	KeyEnv string `json:"keyEnv,omitempty"`
	// NeedsKey is false for engines that run on localhost without auth.
	NeedsKey bool `json:"needsKey"`
	// MaxBatch is the provider's documented ceiling on texts per request; 0 means no published limit.
	MaxBatch int `json:"maxBatch,omitempty"`
	// InputType is the default value for the asymmetric-model hint (`input_type`, `task_type`), where one applies.
	InputType string `json:"inputType,omitempty"`
	// Auth is the header style: bearer (default), azure (`api-key`) or google (`x-goog-api-key`).
	Auth string `json:"auth,omitempty"`
	// Example model id, to put something real in front of a reader.
	Model string `json:"model,omitempty"`
	Note  string `json:"note,omitempty"`
}

var embedProviders = map[string]EmbedProvider{
	"openai":     {Name: "openai", Label: "OpenAI", Wire: "openai", BaseURL: "https://api.openai.com/v1", KeyEnv: "OPENAI_API_KEY", NeedsKey: true, MaxBatch: 2048, Model: "text-embedding-3-small"},
	"openrouter": {Name: "openrouter", Label: "OpenRouter", Wire: "openai", BaseURL: "https://openrouter.ai/api/v1", KeyEnv: "OPENROUTER_API_KEY", NeedsKey: true, MaxBatch: 512, Model: "openai/text-embedding-3-small"},
	"azure": {Name: "azure", Label: "Azure OpenAI", Wire: "openai", KeyEnv: "AZURE_OPENAI_API_KEY", NeedsKey: true, MaxBatch: 2048, Auth: "azure", Model: "text-embedding-3-small",
		Note: "baseUrl is the resource endpoint; the model is the deployment name and goes in the path, with embedding.apiVersion as the query"},
	"voyage":      {Name: "voyage", Label: "Voyage AI", Wire: "openai", BaseURL: "https://api.voyageai.com/v1", KeyEnv: "VOYAGE_API_KEY", NeedsKey: true, MaxBatch: 128, InputType: "document", Model: "voyage-3"},
	"jina":        {Name: "jina", Label: "Jina AI", Wire: "openai", BaseURL: "https://api.jina.ai/v1", KeyEnv: "JINA_API_KEY", NeedsKey: true, MaxBatch: 2048, Model: "jina-embeddings-v3"},
	"mistral":     {Name: "mistral", Label: "Mistral", Wire: "openai", BaseURL: "https://api.mistral.ai/v1", KeyEnv: "MISTRAL_API_KEY", NeedsKey: true, MaxBatch: 512, Model: "mistral-embed"},
	"together":    {Name: "together", Label: "Together AI", Wire: "openai", BaseURL: "https://api.together.xyz/v1", KeyEnv: "TOGETHER_API_KEY", NeedsKey: true, MaxBatch: 512, Model: "BAAI/bge-large-en-v1.5"},
	"deepinfra":   {Name: "deepinfra", Label: "DeepInfra", Wire: "openai", BaseURL: "https://api.deepinfra.com/v1/openai", KeyEnv: "DEEPINFRA_API_KEY", NeedsKey: true, MaxBatch: 512, Model: "BAAI/bge-m3"},
	"huggingface": {Name: "huggingface", Label: "Hugging Face router", Wire: "openai", BaseURL: "https://router.huggingface.co/v1", KeyEnv: "HF_TOKEN", NeedsKey: true, MaxBatch: 256, Model: "intfloat/multilingual-e5-large"},
	"cohere": {Name: "cohere", Label: "Cohere", Wire: "cohere", BaseURL: "https://api.cohere.com", KeyEnv: "COHERE_API_KEY", NeedsKey: true, MaxBatch: 96, InputType: "search_document", Model: "embed-v4.0",
		Note: "asymmetric: input_type search_document for what you store, search_query for what you ask"},
	"gemini":     {Name: "gemini", Label: "Google Gemini", Wire: "gemini", BaseURL: "https://generativelanguage.googleapis.com/v1beta", KeyEnv: "GEMINI_API_KEY", NeedsKey: true, MaxBatch: 100, InputType: "RETRIEVAL_DOCUMENT", Auth: "google", Model: "text-embedding-004"},
	"ollama":     {Name: "ollama", Label: "Ollama (local)", Wire: "openai", BaseURL: "http://127.0.0.1:11434/v1", MaxBatch: 64, Model: "nomic-embed-text"},
	"lmstudio":   {Name: "lmstudio", Label: "LM Studio (local)", Wire: "openai", BaseURL: "http://127.0.0.1:1234/v1", MaxBatch: 64, Model: "nomic-embed-text-v1.5"},
	"vllm":       {Name: "vllm", Label: "vLLM (self-hosted)", Wire: "openai", MaxBatch: 256, Note: "baseUrl is required: the OpenAI-compatible route of your server, usually http://host:8000/v1"},
	"tei":        {Name: "tei", Label: "Text Embeddings Inference (self-hosted)", Wire: "openai", MaxBatch: 256, Note: "baseUrl is required: TEI's OpenAI route, usually http://host:8080/v1"},
	"compatible": {Name: "compatible", Label: "Any OpenAI-compatible endpoint", Wire: "openai", Note: "baseUrl is required"},
}

// aliases keep older spellings and the obvious synonyms working.
var embedProviderAliases = map[string]string{
	"google":             "gemini",
	"googleai":           "gemini",
	"generativelanguage": "gemini",
	"azureopenai":        "azure",
	"hf":                 "huggingface",
	"voyageai":           "voyage",
	"openai-compatible":  "compatible",
	"custom":             "compatible",
	"lm-studio":          "lmstudio",
}

// lookupEmbedProvider resolves a provider name or alias, case-insensitively.
func lookupEmbedProvider(name string) (EmbedProvider, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	if a, ok := embedProviderAliases[n]; ok {
		n = a
	}
	p, ok := embedProviders[n]
	return p, ok
}

// EmbeddingProviders lists every configurable provider, hash first, then the rest by name. The UI reads this so
// that the list in the settings form cannot drift from the list the server accepts.
func EmbeddingProviders() []EmbedProvider {
	out := []EmbedProvider{{Name: "hash", Label: "Built-in hash-256 (offline, lexical)", Wire: "hash",
		Note: "offline, deterministic and free, but it matches word shapes, not meaning"}}
	names := make([]string, 0, len(embedProviders))
	for n := range embedProviders {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		out = append(out, embedProviders[n])
	}
	return out
}

func embedProviderNames() []string {
	out := []string{"hash"}
	for n := range embedProviders {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// endpoint builds the request URL for a provider, which is where azure and gemini stop looking like everyone else.
func (p EmbedProvider) endpoint(base, model, apiVersion string) string {
	base = strings.TrimRight(base, "/")
	switch p.Wire {
	case "cohere":
		return base + "/v2/embed"
	case "gemini":
		return base + "/models/" + url.PathEscape(strings.TrimPrefix(model, "models/")) + ":batchEmbedContents"
	}
	if p.Auth == "azure" {
		v := apiVersion
		if v == "" {
			v = defaultAzureAPIVersion
		}
		return base + "/openai/deployments/" + url.PathEscape(model) + "/embeddings?api-version=" + url.QueryEscape(v)
	}
	return base + "/embeddings"
}

const defaultAzureAPIVersion = "2024-02-01"

// authHeader returns the header name carrying the key for this provider.
func (p EmbedProvider) authHeader() (name, prefix string) {
	switch p.Auth {
	case "azure":
		return "api-key", ""
	case "google":
		return "x-goog-api-key", ""
	}
	return "Authorization", "Bearer "
}

// ---------------------------------------------------------------- wire formats

type cohereReq struct {
	Model          string   `json:"model"`
	Texts          []string `json:"texts"`
	InputType      string   `json:"input_type"`
	EmbeddingTypes []string `json:"embedding_types"`
}

type cohereResp struct {
	Embeddings struct {
		Float [][]float32 `json:"float"`
	} `json:"embeddings"`
	Message string `json:"message"`
}

type geminiReq struct {
	Requests []geminiOne `json:"requests"`
}

type geminiOne struct {
	Model   string `json:"model"`
	Content struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"content"`
	TaskType string `json:"taskType,omitempty"`
}

type geminiResp struct {
	Embeddings []struct {
		Values []float32 `json:"values"`
	} `json:"embeddings"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// encodeEmbedRequest builds the request body for the provider's wire format.
func encodeEmbedRequest(p EmbedProvider, model, inputType string, texts []string) ([]byte, error) {
	switch p.Wire {
	case "cohere":
		t := inputType
		if t == "" {
			t = p.InputType
		}
		return json.Marshal(cohereReq{Model: model, Texts: texts, InputType: t, EmbeddingTypes: []string{"float"}})
	case "gemini":
		req := geminiReq{Requests: make([]geminiOne, len(texts))}
		for i, t := range texts {
			one := geminiOne{Model: "models/" + strings.TrimPrefix(model, "models/"), TaskType: inputType}
			one.Content.Parts = []struct {
				Text string `json:"text"`
			}{{Text: t}}
			req.Requests[i] = one
		}
		return json.Marshal(req)
	}
	// OpenAI wire. input_type is sent only when the provider asks for one (Voyage, Jina v3), because strict
	// servers reject unknown fields.
	if inputType != "" {
		type withType struct {
			Model          string   `json:"model"`
			Input          []string `json:"input"`
			EncodingFormat string   `json:"encoding_format"`
			InputType      string   `json:"input_type"`
		}
		return json.Marshal(withType{Model: model, Input: texts, EncodingFormat: "float", InputType: inputType})
	}
	return json.Marshal(embedReq{Model: model, Input: texts, EncodingFormat: "float"})
}

// decodeEmbedResponse parses a provider response into n vectors, in input order.
func decodeEmbedResponse(p EmbedProvider, data []byte, n int) ([][]float32, error) {
	switch p.Wire {
	case "cohere":
		var r cohereResp
		if err := json.Unmarshal(data, &r); err != nil {
			return nil, fmt.Errorf("embedding response: %v", err)
		}
		if r.Message != "" && len(r.Embeddings.Float) == 0 {
			return nil, fmt.Errorf("embedding api: %s", r.Message)
		}
		if len(r.Embeddings.Float) != n {
			return nil, fmt.Errorf("embedding api returned %d vectors for %d inputs", len(r.Embeddings.Float), n)
		}
		return r.Embeddings.Float, nil
	case "gemini":
		var r geminiResp
		if err := json.Unmarshal(data, &r); err != nil {
			return nil, fmt.Errorf("embedding response: %v", err)
		}
		if r.Error != nil {
			return nil, fmt.Errorf("embedding api: %s", r.Error.Message)
		}
		if len(r.Embeddings) != n {
			return nil, fmt.Errorf("embedding api returned %d vectors for %d inputs", len(r.Embeddings), n)
		}
		out := make([][]float32, n)
		for i, e := range r.Embeddings {
			out[i] = e.Values
		}
		return out, nil
	}
	var r embedResp
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("embedding response: %v", err)
	}
	if r.Error != nil {
		return nil, fmt.Errorf("embedding api: %s", r.Error.Message)
	}
	if len(r.Data) != n {
		return nil, fmt.Errorf("embedding api returned %d vectors for %d inputs", len(r.Data), n)
	}
	out := make([][]float32, n)
	for _, d := range r.Data {
		if d.Index < 0 || d.Index >= n {
			return nil, fmt.Errorf("embedding api returned index %d out of range", d.Index)
		}
		out[d.Index] = d.Embedding
	}
	return out, nil
}
