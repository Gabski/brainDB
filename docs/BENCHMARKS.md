# Benchmarks

Two different questions. **Retrieval quality is measured** and the numbers are below. **Latency and scale are not
measured at all**, and the figures quoted in the README and [SPEC.md](SPEC.md) Addendum F6 are design targets, not
results.

---

## 1. Retrieval quality — measured

### What is being measured

32 hand-judged queries over the `sales-company` example (144 nodes, 64 memories), in two buckets:

- **lexical** (16 queries) — the query shares words with the target memory. Any retrieval method should pass these.
- **paraphrase** (16 queries) — the query shares only *meaning*. The wording was chosen so that no 5-letter stem of
  the query appears in the target text, which is exactly what the built-in `hash-256` engine indexes. For example
  *"smallest profit we are allowed to accept"* against *"the margin floor is 32%"*.

Relevance is binary and deliberately tight: only memories that actually answer the query are judged relevant. The
gold set is [`examples/sales-company/eval.json`](../examples/sales-company/eval.json), with the reasoning for each
paraphrase recorded in its `note` field.

```bash
braindb seed examples/sales-company/seed.json -o brain.xml
braindb eval --brain brain.xml --gold examples/sales-company/eval.json
```

Metrics: recall@k, mean reciprocal rank of the first relevant hit, and nDCG@10 with binary gains. "Queries with no
hit" counts queries where nothing relevant appeared anywhere in the top 10 — the number a user actually feels.

### Result: `hash-256`, the built-in offline engine

Measured 20 Nov 2026 · brainDB 0.0.2 · Go 1.27.4 · `hash-256` (dim 256) · 144 nodes · k = 10.

**lexical** (16 queries)

| mode | recall@5 | recall@10 | MRR | nDCG@10 | queries with no hit |
|---|---|---|---|---|---|
| `vector` | 0.71 | 0.96 | 0.85 | 0.82 | 0/16 |
| `hybrid` | 0.76 | 0.86 | 0.85 | 0.78 | 0/16 |
| `graph` | 0.83 | 0.92 | **1.00** | **0.91** | 0/16 |
| `tree` | 0.82 | 0.96 | 0.86 | 0.84 | 0/16 |

**paraphrase** (16 queries)

| mode | recall@5 | recall@10 | MRR | nDCG@10 | queries with no hit |
|---|---|---|---|---|---|
| `vector` | 0.02 | 0.09 | 0.07 | 0.06 | **14/16** |
| `hybrid` | 0.02 | 0.22 | 0.09 | 0.10 | **12/16** |
| `graph` | 0.15 | 0.15 | 0.14 | 0.14 | **13/16** |
| `tree` | 0.04 | 0.22 | 0.08 | 0.10 | **12/16** |

**all 32 queries**

| mode | recall@5 | recall@10 | MRR | nDCG@10 | queries with no hit |
|---|---|---|---|---|---|
| `vector` | 0.36 | 0.53 | 0.46 | 0.44 | 14/32 |
| `hybrid` | 0.39 | 0.54 | 0.47 | 0.44 | 12/32 |
| `graph` | 0.49 | 0.53 | **0.57** | **0.52** | 13/32 |
| `tree` | 0.43 | 0.59 | 0.47 | 0.47 | 12/32 |

### What these numbers say

**1. On lexical queries the offline engine is strong.** `graph` puts a relevant memory first on every single query
(MRR 1.00). If your queries reuse the vocabulary of your notes, `hash-256` needs no API key and costs nothing.

**2. On paraphrases it collapses.** Twelve to fourteen of sixteen queries return nothing relevant in the top ten.
This is not a tuning problem: `hash-256` hashes 5-letter stems, bigrams and character trigrams, so *"freezing
warehouses"* and *"−20 °C cold store"* have no shared signal to find. Calling this mode `vector` is accurate about
the mechanism and misleading about the capability — it is lexical retrieval in vector clothing.

Only four paraphrases found anything, and they found it through the graph rather than through similarity:

| query | best rank | target |
|---|---|---|
| "will the units survive hot conditions" | 1 | `q144` NaCore 2 at 45 °C |
| "yearly revenue from our most important client" | 6 | `f101` ARR €480,000, largest account |
| "smallest profit we are allowed to accept" | 8 | `f109` margin floor is 32% |
| "why we cannot meet the lower competing offer" | 10 | `q141` match Grayfield without dropping below the floor |

**3. Graph activation earns its place, but it cannot rescue a lexical seed.** `graph` beats flat `vector` top-k on
every aggregate metric (MRR 0.57 vs 0.46, nDCG 0.52 vs 0.44) and is the only mode that never misses a lexical query.
That is evidence for the core design claim. But on paraphrases every mode is within noise of every other, because
activation can only spread from seeds that similarity found, and with this engine similarity finds nothing.

**4. Therefore `hash-256` should not be the documented default for general use.** It is the right default for
*offline* and *zero-cost*, and it should stay available for exactly that. For anything where a user phrases a
question in their own words — which is every agent use case — a real embedding model is not an optimisation, it is a
prerequisite. See [Embeddings](../README.md#embeddings-hash-or-a-real-model).

### Still missing: the same table with a real embedding model

The experiment that settles the question is one command away, and has not been run because it needs an API key:

```bash
export OPENROUTER_API_KEY=...
BRAINDB_EMBED_PROVIDER=openai \
BRAINDB_EMBED_MODEL=openai/text-embedding-3-small \
BRAINDB_EMBED_BASE_URL=https://openrouter.ai/api/v1 \
BRAINDB_EMBED_KEY_ENV=OPENROUTER_API_KEY \
braindb eval --brain brain.xml --gold examples/sales-company/eval.json
```

It embeds 144 node texts and 32 queries once, then caches them, so repeat runs are free. Fully offline alternative,
same command with `BRAINDB_EMBED_BASE_URL=http://127.0.0.1:11434/v1` and an Ollama model such as `nomic-embed-text`.

| engine | lexical MRR | paraphrase MRR | paraphrase misses |
|---|---|---|---|
| `hash-256` (measured) | 0.85–1.00 | 0.07–0.14 | 12–14 of 16 |
| remote embedding model | — | — | — |
| remote, `vector` only (flat RAG baseline) | — | — | — |

The third row is the control: it isolates how much of any improvement comes from the embeddings and how much from
graph activation on top of them.

### Replication on a second brain

The same experiment on a different domain: [`examples/braindb-self`](../examples/braindb-self), brainDB's own
project knowledge, with its own 24-query gold set built the same way (12 lexical, 12 paraphrase).

```bash
braindb seed examples/braindb-self/seed.json -o self.xml
braindb eval --brain self.xml --gold examples/braindb-self/eval.json
```

Measured 20 Nov 2026 · brainDB 0.0.2 · Go 1.27.4 · `hash-256` (dim 256) · 158 nodes · k = 10.

| mode | lexical MRR | paraphrase MRR | all MRR | all nDCG@10 | paraphrase misses |
|---|---|---|---|---|---|
| `vector` | 0.86 | 0.06 | 0.46 | 0.47 | 9/12 |
| `hybrid` | 0.86 | 0.06 | 0.46 | 0.48 | 9/12 |
| `graph` | **0.96** | **0.14** | **0.55** | **0.53** | **8/12** |
| `tree` | 0.86 | 0.04 | 0.45 | 0.48 | 9/12 |

Both conclusions hold on the second brain. Graph activation ranks first overall (MRR 0.55 against 0.46 for flat
vector top-k, the sales set gave 0.57 against 0.46) and misses no lexical query; paraphrases collapse in every mode
(8 to 9 of 12, the sales set gave 12 to 14 of 16). Two independently written gold sets over two unrelated domains
now say the same thing, which is weak evidence made slightly less weak.

### Caveats, stated plainly

- 32 queries on a 64-memory fictional brain is a smoke test, not a benchmark. It is enough to show a collapse this
  large; it is not enough to rank two good engines against each other.
- The gold set was written by the same person who wrote the seed data. The paraphrase bucket is adversarial towards
  `hash-256` **by construction**, because that is the hypothesis being tested — but a reader should know it, and a
  public benchmark (LoCoMo, LongMemEval) is still owed.
- Relevance judgments are binary and tight. Looser judgments would raise every number.

---

## 2. Latency and scale — not measured

Nothing below has been run. These are the targets for a synthetic 100k-node brain (records + chunks + concepts,
average degree 8), which `braindb bench` will build and measure once it and the SQLite backend exist.

| operation | target | measured |
|---|---|---|
| import 100k CSV rows | < 30 s | — |
| activation, depth 2 | p95 < 50 ms | — |
| hybrid search | p95 < 100 ms | — |
| indexed raw query (2 field filters + sort + limit 50) | p95 < 30 ms | — |
| exec batch of 10 ops | p95 < 20 ms | — |
| cold start, full load | < 2 s | — |

Today the whole brain is held in memory and written to `brain.xml` atomically, which is comfortable into the tens of
thousands of nodes and has not been pushed further. Each future run will record CPU, RAM, disk, Go version, brainDB
version, storage backend and embedding engine, because none of these numbers mean anything without them.

---

## 3. Parity — verified

`go test ./internal/brain -run TestParity` replays a fixed brain through the Go implementation and pins the output to
the JavaScript prototype brainDB was ported from: activation order and values, float32 vectors bit for bit, all four
search modes, suggestions, ablation, paths, discovery and chunking. Two independent implementations agreeing to the
bit is a correctness result, not a performance or quality one.

*Measurement environment for section 1: Intel Core i5-1235U, 12 threads, 38 GB RAM, Linux, Go 1.27.4.*
