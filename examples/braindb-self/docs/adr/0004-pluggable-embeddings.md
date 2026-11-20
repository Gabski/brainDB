# ADR 0004: the embedding engine is pluggable, hash-256 stays the default

- Status: accepted, 0.0.2
- Concepts: embeddings, retrieval

## Context

`hash-256` is offline, deterministic, free and lexical. It indexes 5-letter stems, bigrams and character trigrams,
so it matches shared word shapes, not meaning. For a brain whose queries reuse the vocabulary of its notes that is
enough. For an agent, where the question is phrased in the user's own words, it is not.

## Decision

`embedding.provider: openai` sends text to any OpenAI-compatible `/embeddings` endpoint: OpenRouter, OpenAI,
Ollama, LM Studio, vLLM, a local Text Embeddings Inference server. One config block, no provider SDK, no code
change. `hash-256` stays the default, so nothing changes unless it is configured.

Operational rules:

- requests batched (default 64) and retried on 429 and 5xx with capped exponential backoff;
- vectors cached on disk by `sha256(model + text)` next to the brain, so restarts and re-imports cost no API calls,
  and changing the model invalidates the cache by itself;
- `fallback: hash` degrades and reports the engine as degraded, `fallback: error` fails the operation instead;
- keys are referenced by environment-variable name and never written to `braindb.yaml`, `brain.xml` or the cache;
  a missing `apiKeyEnv` variable is an error at load time.

## Consequences

- Switching engines for a measurement is one line of environment, which is what made ADR 0005 possible.
- The default is now a documented trade-off rather than an implementation detail, and the measurement in ADR 0005
  reopens the question of whether it should stay the default for general agent use.
