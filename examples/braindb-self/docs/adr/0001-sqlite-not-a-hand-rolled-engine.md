# ADR 0001: SQLite replaces the hand-rolled storage engine

- Status: accepted, 0.0.2
- Supersedes: the Addendum F plan to write the storage layer from scratch
- Concepts: storage, release

## Context

The original plan for scale was a custom write-ahead log, inverted and range indexes, a query planner, a pure-Go
HNSW index and a from-scratch Redis client. All of it inside a project whose value is the memory model, not the
bytes on disk. None of it had been measured, and none of it was close to crash-safe.

## Decision

SQLite becomes the storage backend and the single dependency the project takes. Redis leaves the roadmap and
`docker-compose.redis.yml` is removed. `brain.xml` stays as the readable, diff-friendly format and as the export;
it stops being the thing that has to scale.

## Consequences

- The WAL, the indexes, the planner, concurrent readers and crash safety arrive as a dependency instead of as
  six months of debugging.
- The "stdlib only" claim narrows to "the core is stdlib only".
- Open question: what moves first, the node store, the entity index or the vectors.
- Addendum F2 holds the schema; Addendum F1 holds the interface the backend must satisfy.
