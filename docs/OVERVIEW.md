<!--
Copyright 2026 Gabriel Koziestański
SPDX-License-Identifier: Apache-2.0
-->

# brainDB in plain language

**What this page is.** The first half explains brainDB without technical vocabulary — for anyone deciding whether this
is worth their team's time. The second half states the design decisions behind it, why it leans on existing standards
rather than inventing its own, and what is actually built today.

Status: **concept, version 0.0.2, released 2026-11-20.** Expect breaking changes at every `0.0.x` step. The honest split between what works
and what is written down is in [the status table](#what-works-and-what-is-on-paper) and in [README.md](../README.md).

---

## Part I — without the jargon

### The problem: the knowledge exists, nobody remembers it

A company knows a great deal. What was agreed last quarter, why that supplier was turned down, the special terms one
customer negotiated, a meeting note from a year ago. All of it exists — scattered across documents, threads, and the
heads of people who sometimes leave.

AI assistants have the same problem in a sharper form. Every conversation starts from nothing. You can paste in piles
of documents, but then the model gets everything at once: current next to obsolete, important next to incidental. It
has no way to know that the March decision was replaced in June, because both sentences sound equally confident.

### The idea: memory and attention are two different things

A library holds millions of sentences, but you read the one page your lamp is pointed at. brainDB works the same way.
All the knowledge sits in one place and none of it disappears. A question switches on a light at one point, and it
spreads along the connections: from that decision to the task it produced, and to the customer it concerned. Whatever
got lit becomes the context handed to the assistant. The rest waits.

```
            attention ─┐                  ┌─ the lit slice = context (XML for models, Markdown for people)
                       ▼                  │
   ┌──────── the whole memory graph ──────┴───────────────────────────┐
   │  notes · decisions · tasks · files · records · concepts ◆        │
   │  weights fade with time · replaced versions stay, dimmed         │
   └──────────────────────────────────────────────────────────────────┘
```

### What that changes

**Knowledge ages instead of vanishing.** A note loses weight within days, a decision over months. A newer version does
not delete the old one, it supersedes it — so you can always reconstruct what was known at the time, and why the answer
changed.

**You can see where an answer came from.** Every result carries its weight, the route the signal travelled, and the
source memory. Not "the model thinks so", but "this follows from these three records".

**Not everyone sees the same brain.** Knowledge carries access labels — finance, HR, sensitive IP. For someone without
clearance that part of memory is not hidden, it does not exist: not in their context, not in traversal, not in search
results, not in counts.

**The database knows what it is for.** Each brain has a written purpose. Anything that does not fit goes to a review
queue instead of polluting the memory, and assistants receive that purpose along with their context, so they know what
is worth storing.

### What it is not

Not a chatbot and not a language model — brainDB generates nothing. Not a replacement for the transactional database
that holds your invoices and orders. It is long-term memory, shared by people and assistants, that keeps track of what
is still true.

---

## Part II — the design decisions

Not a feature list. Twelve decisions, each of which closes off some roads — which is the point. A system with no closed
roads has no shape.

### Model: a weighted graph, not a bag of vectors

Knowledge is nodes (facts, decisions, tasks, questions, notes, files, CSV records, your own kinds) and weighted
synapses between them. Classic RAG chops documents into chunks and ranks them by similarity; here the relationship is
a first-class object, because most institutional knowledge *is* relationships.

### Context: spreading activation, not top-k

The context is not a list of the ten most similar paragraphs. It is a coherent slice of the graph around a focus,
shaped by depth, threshold and damping. That is why the context can include knowledge that shares no word with the
query but is connected to it.

### Time: decay with a floor, supersede instead of delete

Weight follows `w = base × (floor + (1 − floor) × 0.5^(age / half-life))`. Durable kinds carry a floor, so they rank
lower with age but never drop out of the graph. Only an explicit `SUPERSEDE` or `FORGET` removes knowledge. History
stays explainable.

### Structure: meaning and wiring are separate layers

A concept ◆ ("Pricing") carries knowledge and costs context tokens. A connector ⧉ (anchor, bridge, hub, alias, time
bucket) carries none — it joins the same customer across a SQL dump, a ticket queue and a pile of notes. Activation
routes through it, but it never appears as a result and never takes up room in the context.

### Data: entities recognised and normalised on write

Dates, money in any currency, percents, units, emails, phone numbers, domains, people, organisations, IBAN and VAT with
checksums, cards (always masked), IPs, versions, addresses. All reduced to a comparable form, which is what makes
`WHERE entities.money > 'EUR 10000'` meaningful and lets `RESOLVE email` turn an entity into an anchor that joins one
identity across sources.

### Hygiene: the purpose is an admission filter

The purpose written in `braindb.yaml` is scored deterministically, with reasons. An incoming item is admitted,
quarantined for review, or rejected. The default is advisory rather than blocking: a memory that silently drops data is
worse than a cluttered one.

### Access: permissions on knowledge, not on endpoints

Access labels sit on nodes. For a user without clearance those areas are switched off, and they do not leak through
vectors, scores, counts, connectors or error messages either. A property test guards that.

### Language: one query language, atomic scripts

BDQL covers reads, writes, analysis and administration. A script applies completely or not at all, so an agent reads
what it needs and writes what it learned in a single turn, with no half-applied states.

### Integration: a protocol instead of plugins

MCP over stdio and over HTTP, an HTTP API with an SSE stream, a CLI with a REPL, and a 3D web UI. An agent connects
with one command; nobody writes an adapter per tool.

### Format: one file, readable by a human and by git

`brain.xml` is written atomically. You can read it, diff it, keep it in a repository, copy it. 0.0.2 adds a SQLite
backend (`brain.db`) for larger brains, with lossless migration both ways, but XML stays the export format — the data
is meant to outlive the tool.

### Dependencies: the standard library, with one deliberate exception

The whole backend runs on the Go standard library with one dependency, `modernc.org/sqlite` (pure Go, so every
release target still cross-compiles with `CGO_ENABLED=0`). An earlier version of the plan promised a hand-rolled
write-ahead log, indexes and vector engine; SQLite replaced it, because re-creating a proven database is years of
work and the bugs would land in the one layer that must never lose data.

### Honesty: measured and unmeasured are kept apart

Retrieval quality is measured against a judged query set and published together with its weak results. Latency and
scale are not measured at all, so the documentation lists them as design targets, not results. The status table
separates "works" from "planned".

---

## Part III — why standards instead of our own

A custom protocol, a custom date format and a custom storage engine look like savings at first: you build exactly what
you need and nothing more. The bill arrives later, in four parts.

- **Integration.** A standard works with tools the author never saw. A custom protocol needs an adapter per tool — and
  then someone has to maintain every adapter.
- **Exit cost.** Data in an open format outlives the tool. Data in a private binary format lives exactly as long as its
  parser does.
- **Code you do not have to write.** IBAN validation, time zones, a database write-ahead log: in these areas any
  hand-written variant is worse than the existing one, because it has not spent a decade meeting reality.
- **Auditability.** "Conforms to ISO 8601" is a claim you can check. "Our own time format" is a claim you have to read
  the source for.

| Standard | Where brainDB uses it | What follows from it |
|---|---|---|
| MCP | connecting agents (stdio and HTTP) | Claude Code, Cursor, Codex and any other client work without a per-tool plugin |
| OpenAI-compatible `/embeddings` | the embedding engine | OpenRouter, OpenAI, Ollama, LM Studio, vLLM, TEI — switching provider is a config block, not a code change and not another SDK |
| ISO 8601 · ISO 4217 · E.164 | normalising dates, money, phone numbers | values from different sources compare and filter as values, not as strings |
| IBAN · VAT checksums | entity recognition | the number is verified, not guessed from its shape |
| HTTP + Bearer + SSE | API and event stream | familiar auth and a plain event stream instead of a bespoke socket protocol |
| XML · JSON · YAML · CSV · Markdown | context and query output | the result is read by a model, a spreadsheet, a script and a person, with no converter in between |
| SQLite | the storage backend | write-ahead log, indexes, a query planner, concurrent readers and crash safety, already built and already debugged |
| Apache-2.0 · SPDX · NOTICE | licence and file headers | predictable rights for commercial use and clear attribution |
| SemVer + changelog | versioning | you can see which change breaks existing brains before you apply it |

The same rule runs the other way. Where there is no standard, brainDB does not pretend to have one: the `bridge` and
`time` connectors are specified but not built, and the config **rejects** those keys with an explanation rather than
accepting them and quietly doing nothing.

---

## What works, and what is on paper

| Area | Status |
|---|---|
| Model: memories, concepts, connectors, decay, activation, context XML/MD | **works** |
| Four search modes, paths, link tests, suggestions | **works** |
| BDQL, MCP, HTTP API, SSE, CLI, REPL | **works** |
| Import: Markdown, CSV, JSON, YAML, PDF, DOCX, HTML, SQL dumps | **works** |
| Users, roles, access labels, purpose admission | **works** |
| Web UI, Storybook, Docker | **works** |
| Federation across brains (remote transport, federated search) | partial |
| SQLite backend (WAL, indexes, two-way migration, raw-query pushdown over the field indexes) | **works**; the brain is still loaded whole into memory and the stem index is written but not read |
| Latency measurements, publishing a brain as a binary | planned (`braindb bench` runs, the numbers are not published yet) |
| JWT / SSO | planned |

### Measured retrieval quality

32 judged queries over the sample brain (144 nodes), in two buckets: **lexical** (the query shares words with the
answer) and **paraphrase** (it shares only meaning). The built-in, free `hash-256` engine indexes word shapes, so
paraphrases are out of its reach, and the numbers say so plainly.

| Mode | Lexical recall@10 | Paraphrase recall@10 | Paraphrase queries with no hit |
|---|---:|---:|---:|
| `hybrid` | 0.86 | 0.22 | 12 / 16 |
| `vector` | 0.96 | 0.09 | 14 / 16 |
| `graph` | 0.92 | 0.15 | 13 / 16 |
| `tree` | 0.96 | 0.22 | 12 / 16 |

**Practical conclusion.** For a deployment where people ask in their own words, plug in a real embedding model — one
config block. The built-in engine is for offline use, tests, and lookups by literal phrasing. Full method and numbers:
[BENCHMARKS.md](BENCHMARKS.md). Latency and behaviour at a hundred thousand nodes have not been measured; the figures
in the documentation are targets, not results.

---

## Getting started

**1. Build a brain** — from an example or from your own folder of documents. Imports are incremental: unchanged files
are skipped, changed ones supersede the previous version.

```bash
make build
bin/braindb seed examples/sales-company/seed.json -o brain.xml
bin/braindb serve --brain brain.xml
```

**2. Connect an assistant** — one command on the client side. From then on the agent reads from and writes to the
shared memory.

```bash
claude mcp add braindb -- /abs/path/bin/braindb mcp \
  --brain /abs/path/brain.xml --actor claude
```

**3. Say what the brain is for** — the purpose in `braindb.yaml` decides what is admitted and what is quarantined, and
travels to agents with their context so they know what to store.

On authentication: the server **refuses** to listen on a non-loopback address until a token is set, and it reads token
values only from environment variables, never from the config file.

---

Further reading: [README.md](../README.md) · [SPEC.md](SPEC.md) · [AGENTS.md](AGENTS.md) ·
[INTEGRATIONS.md](INTEGRATIONS.md) · [BENCHMARKS.md](BENCHMARKS.md)
