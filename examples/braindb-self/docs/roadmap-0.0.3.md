# Roadmap 0.0.3

The release is defined by one measurement, one migration and one promise not to break anything quietly.

## 1. Answer the paraphrase question with a number

`braindb eval` already runs against any engine through environment variables. Two rows of the engine table in
`docs/BENCHMARKS.md` are empty: a remote embedding model, and the same model with `vector` only as a flat-RAG
control. The control is the point. Without it, any improvement is split between the embeddings and the graph in
an unknown ratio.

Blocked on an API key, not on code.

## 2. SQLite behind the storage interface

Addendum F1 defines the interface, F2 the schema. Open: what moves first, the node store, the entity index or the
vectors. `brain.xml` stays as the readable export and stops being the hot path.

## 3. Federation that leaves the machine

Sources and external stubs are stored today; the transports are not written. The open decision is whether federated
search speaks the HTTP API or MCP over Streamable HTTP. Until then, `SOURCES lab` resolves against cached stubs,
which is honest but not federation.

## 4. Decide about the connectors that are only declared

`connectors.bridges` and `connectors.time` are rejected by the loader so that a configuration cannot silently do
nothing. Either implement them for 0.0.3 or remove the keys.

## 5. Documentation that matches the measurement

The README still presents `hash-256` as the default without saying, at the point of the quick start, that it does
not match paraphrases. `docs/BENCHMARKS.md` says it plainly; the quick start should too.

## Not in 0.0.3

`braindb publish`, snapshots, latency and scale numbers. The last one needs `braindb bench` on both backends, which
needs the SQLite backend to exist first.
