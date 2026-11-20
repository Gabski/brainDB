# ADR 0005: measure retrieval before claiming it works

- Status: accepted, 0.0.2
- Concepts: benchmark, retrieval, docs

## Context

The README claimed that graph activation beats flat vector top-k. The claim was plausible, repeated in the
specification, and unmeasured. `docs/BENCHMARKS.md` was created to say so in plain words.

## Decision

`braindb eval` scores a brain against a file of judged queries and reports recall@5, recall@10, MRR and nDCG@10 per
search mode, broken down by bucket. The gold set splits queries into **lexical** (shares words with the target) and
**paraphrase** (shares only meaning, with no 5-letter stem in common). A gold set whose relevant ids do not exist
in the brain is rejected at load time.

## Consequences

Measured on 20 Nov 2026, brainDB 0.0.2, `hash-256`, 144 nodes, 32 queries, k = 10:

- graph activation does beat flat vector top-k overall: MRR 0.57 against 0.46, nDCG@10 0.52 against 0.44;
- on lexical queries graph mode is perfect: MRR 1.00, no query without a hit;
- on paraphrases every mode collapses: 12 to 14 of 16 queries return nothing relevant in the top ten.

The core design claim survived its first measurement. The default engine did not. Performance figures elsewhere are
now labelled as targets, because latency and scale are still unmeasured.
