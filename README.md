# brainDB

**A graph database that works like a brain: long-term memory for AI agents and teams, queried by context.**

> **Status: concept, version 0.0.2, released 2026-11-20.** This repository and its documentation describe the system as designed.
> Commands, BDQL syntax, config keys, API endpoints and the performance numbers are a specification to react to,
> not a stable contract. Expect breaking changes at every 0.0.x step.
>
> **0.0.2 changes behaviour.** Durable kinds no longer decay to zero (every kind has a *floor*), purpose admission
> defaults to `advise` instead of `enforce`, the embedding engine is pluggable, and the SQLite backend ships —
> Redis is off the roadmap. See [CHANGELOG.md](CHANGELOG.md).

**Project page and documentation:** [https://braindb.eu](https://braindb.eu).
The full guide — installation, usage, BDQL, agent integration, API reference and examples — is at [https://braindb.eu/docs/](https://braindb.eu/docs/).

**New here?** The [overview](https://braindb.eu/docs/OVERVIEW.html) explains brainDB in plain language first, then the design
decisions behind it, why it leans on existing standards, and what is actually built today.

brainDB stores knowledge as a weighted graph:

- **memories**: facts, decisions, tasks, questions, notes, summaries, files, chunks, ideas, CSV records and your own kinds;
- **concepts**: abstract knowledge the memories connect to;
- **synapses**: weighted edges between nodes.

Memories decay with time. Outdated versions are superseded, not deleted.

The whole brain is memory. Only a **lit slice** of it is the working context. The slice comes from spreading activation that
starts at a **focus** and runs to a chosen **depth**. You can watch it in 3D, query it like a database, fill document
templates from it, and give it to any MCP-capable agent as its memory.

```
            focus ─┐                    ┌─ lit slice = context (XML / Markdown / JSON / YAML)
                   ▼                    │
   ┌──────── memory graph (brain.xml) ──┴──────────────────────────────────┐
   │  memories ── synapses ── concepts ◆      weights decay · superseded fade │
   │  files ⊂ chunks · CSV rows = records    hash or remote embeddings, graph-enriched │
   └──────┬───────────────┬──────────────────┬──────────────────┬───────────┘
          │               │                  │                  │
     MCP (stdio)     HTTP API + SSE     3D web UI         braindb publish
     7 tools         query · fill       (React, three.js,  → one binary per brain
                                         Storybook)          (instant MCP server)
```

## Status

brainDB is young. This table shows what works today and what is specified ([docs/SPEC.md](docs/SPEC.md)) but not built yet.

| area | status |
|---|---|
| Core model: memories, concepts, connectors, decay, activation, context XML/MD | **working** |
| Embeddings: built-in hash engine, or any OpenAI-compatible endpoint (OpenRouter, OpenAI, Ollama, vLLM, TEI) | **working** |
| Search (hybrid, vector, tree, graph), paths, link tests, suggestions, discover | **working** |
| BDQL, MCP (stdio and Streamable HTTP), HTTP API, SSE, CLI, REPL | **working** |
| Raw queries, template fill, built-in pattern library (entities), PII masking | **working** |
| Bulk import (Markdown, CSV, JSON/JSONL, YAML, SQL dumps, PDF/DOCX/HTML), multiple databases, `braindb.yaml` | **working** |
| Users, roles, grants, access labels (non-leakage property-tested), purpose admission filter | **working** |
| Web UI (DB-tool layout, 3D graph, BDQL editor), Storybook, Docker and compose | **working** |
| Federation | partial: sources and external stubs are stored, but remote transports and federated search are **planned** |
| `braindb publish` (brain as a binary), snapshots | **planned** |
| Automatic `bridge` and `time` connectors (`connectors.bridges`, `connectors.time`) | **planned**; both keys are rejected by the config so they cannot silently do nothing. Bridges can be created by hand with `CONNECTOR bridge "…"`; for time, query the `touched` column |
| Retrieval-quality evaluation (`braindb eval`, judged gold set, recall/MRR/nDCG) | **working**; measured, see [docs/BENCHMARKS.md](docs/BENCHMARKS.md) |
| SQLite backend: schema, WAL, lossless two-way migration, serving from `brain.db` | **working**; one dependency (`modernc.org/sqlite`, pure Go, every release target still cross-compiles with `CGO_ENABLED=0`) |
| Raw-query pushdown over the SQLite field/node indexes | **working** for `kind`, `section`, `id` and field predicates, as a sound pre-filter that the in-memory predicate still re-checks; it stands down when the database and memory can disagree, and falls back to the scan for predicates that match a large part of the brain (measured slower through SQL) |
| Text pushdown over the stem index, and lazy loading for 10^5–10^6 nodes | **planned**; the stem index is written but not read, and the brain is still loaded whole into memory (Addendum F3 "small databases") |
| Latency benchmarks (`braindb bench --nodes N [--backend file\|sqlite\|both]`) | **working**; synthetic brain, both backends, reports save/load/query/search timings and whether a query was answered from the indexes |
| `server` block in `braindb.yaml` (`addr`, `tokenEnv`, `tokens` with scopes and `dbs`, `allowNoToken`) | **working**; token values always come from environment variables, never from the file |
| JWT/SSO | **planned** |

## Features

- **Two node classes.**
  - **Knowledge nodes:** memories, records, chunks and concepts ◆. They hold content, appear in the context and cost tokens.
  - **Connector nodes ⧉:** `anchor`, `bridge`, `hub`, `alias`, `time`. They hold no knowledge. They organise connections coming from different places: the same customer in a SQL dump, in tickets and in notes; a link to another brain; a synonym. Activation and search route through them, but they never show up as results or cost tokens.
- **Built-in pattern library.**
  - Dates, money in any currency, percents, units, emails, phone numbers (E.164), websites and domains, people, organisations, IBAN/VAT (with checksums), cards (always masked), IPs, versions, id codes, addresses.
  - Entities are recognised in every text and record, normalised (dates to ISO 8601, money to an ISO 4217 amount, phones to E.164) and indexed, in several locales.
  - Entities can be queried (`WHERE entities.money > 'EUR 10000'`) and boost search on exact matches. Template slots use them. `RESOLVE email` turns them into anchors, so one person or company is joined across sources.
  - PII masking.
- **A purpose for every brain, used as an admission filter.**
  - `braindb.yaml` describes what the database is for, what belongs in it and what does not.
  - Every incoming item is scored against that description, deterministically and with reasons, and is then admitted, quarantined for review, or rejected.
  - Agents receive the purpose with their context, so they know what to store.
- **Users, roles and grants on knowledge.**
  - Nodes carry access labels (`finance`, `hr`, `ip-sensitive`). For a user without clearance, those areas of the brain are switched off.
  - What a user cannot see does not exist for them: no context, no traversal, no search hits.
  - It does not leak through vectors, scores, counts, connectors or error messages either; a property test guarantees this.
  - Tokens or JWT/SSO identify users. Admins can "view as" another user.
- **Context by activation.** Focus + depth + threshold + damping decide what the model sees.
  - The context ships with weights, activation, depth, the path the signal took (`via`), refs and concepts.
  - It renders as XML for models or as a Markdown digest for people (`text.md`).
- **Pluggable embedding engine.**
  - Built in: `hash-256`, offline, deterministic and free. It matches shared word shapes, so it is lexical, not semantic.
  - Or any OpenAI-compatible `/embeddings` endpoint: **OpenRouter**, OpenAI, Ollama, LM Studio, vLLM, a local TEI server.
    One config block, no provider SDK, no code change.
  - Every vector is cached on disk by `sha256(model + text)`, so a restart or a re-import costs nothing and changing
    the model invalidates the cache by itself.
  - Requests are batched, retried on 429/5xx, and degrade to hash vectors rather than losing an import — or fail
    loudly with `fallback: error`.
- **Four search modes.**

| mode | how it works |
|---|---|
| `hybrid` | vector seeds, then graph activation (GraphRAG style) |
| `vector` | kNN on graph-enriched embeddings |
| `tree` | beam search over the graph |
| `graph` | word stems + activation; also finds knowledge reachable only through connections |

- **Raw data queries.** Filter, sort and project memories and records like a table.
  Output: JSON, YAML, XML, Markdown or CSV.
  ```
  kind:deal stage=negotiation value>10000 sort:-value limit:10 select:id,fields.account,fields.value
  ```
- **Template fill.** Describe the data you want and brainDB fills it in, for example `name: "client name"` or `arr: "annual contract value:money"`.
  - Template formats: JSON, YAML, XML or Markdown.
  - Every value comes with its evidence: source memory, score and snippet.
  - Slots without enough evidence are listed in `missing`, so an agent can finish them.
- **Create a database from a pile of files.** Markdown, text, HTML, PDF, DOCX, CSV, JSON, JSONL, YAML and SQL dumps (PostgreSQL and MySQL: tables, rows, foreign keys).
  - Tables become kinds, rows become records, foreign keys become links, low-cardinality columns become concepts.
  - Imports are incremental: unchanged files are skipped, changed ones supersede the old version.
- **Two storage backends.**
  - `brain.xml`: the whole brain in one file, written atomically, with cached activation and search. Readable,
    diff-friendly, fine up to tens of thousands of nodes.
  - `brain.db`: SQLite (WAL) — write-ahead log, indexes, a query planner, concurrent readers and crash safety that
    would otherwise have to be written and debugged here from scratch. `braindb migrate` converts either way without
    loss. Raw queries push `kind`, `section`, `id` and field predicates down to the indexes; the stem index is
    written but not yet read, and the brain is still loaded whole into memory.
- **Many databases per server**, each configured by its own `braindb.yaml`: kinds, decay, context defaults, embedding, import rules, sources, MCP tools.
- **Analysis.**
  - link tests: propagation, plus ablation of every synapse;
  - strongest knowledge paths between two memories;
  - link prediction (Adamic-Adar + stems + cosine);
  - discovery of missing links and of bridges to faded memory.
- **Ingest.**
  - Markdown, text, code, PDF (via `pdftotext`) and images.
  - CSV, as one record per row.
  - ctxc workspaces (`state.json`, `<context>` XML).
  - Files are chunked, embedded and auto-linked to concepts and similar memories.
- **Federation: link other brains.**
  - Mount other brainDB servers, brain files, snapshots, published brain binaries or web pages as read-only sources.
  - Link your memories to their nodes (`lab:f101`).
  - Context activation, search, queries and template fills can then reach across brains, weighted per source, cached and with timeouts.
- **Professional DB-tool UI.**
  - Navigator tree, tabbed workspace (Browse with facet filters, Query editor with a results grid, Context, Template, Graph 3D, Schema, Sources, Settings, Activity), Inspector, status bar.
  - **Settings as forms, not only a text box.** `braindb.yaml` in sections — identity, context defaults, kinds and decay,
    embeddings, import, storage, purpose admission, access and MCP — each key showing whether it is written in the file or left to the
    default, with a reset per key, a diff before saving and dry-run validation as you type. The forms edit the text itself, so
    comments survive and only the line you touched changes; the raw editor is one click away for lists and rules.
  - **API and MCP tokens are added from the form**: a table of `server.tokens` with a name, an environment variable and
    a scope (`read` / `write` / `admin`), plus the admin `server.tokenEnv`. The field takes a *variable name* and refuses
    anything else, because the token value belongs in the environment and never in a file you commit.
  - Command palette and full keyboard navigation.
  - Tested by Vitest unit tests over the pure modules plus Storybook stories for every component; `make test` runs
    gofmt, `go vet`, the Go tests, the web typecheck and the web unit tests.
- **Live 3D UI.**
  - Six views: brain, layers, depth sphere, decay timeline, vector map (PCA) and spanning tree.
  - A live feed of what the agent does.
  - Every component has a Storybook story.
- **BDQL, a SQL-like language for everything**: `SELECT … WHERE … NEAR … ORDER BY`, `CONTEXT`, `SEARCH`, `REMEMBER`, `INSERT`, `UPDATE`, `LINK`, `SUPERSEDE`, `FORGET`, `FILL`, `IMPORT`, `SOURCE`, `EXPLAIN`, transactions. Available from MCP, HTTP, the CLI, a REPL and the UI editor.
- **MCP server** with two tools by default (`brain_view` + `brain_run`). Every reply ends with a status line (lit size, tokens, depth, focus).
- **Publish a brain as one executable.** The brain is embedded and its indices precomputed.
  - Run with no arguments, it is an MCP server.
  - Modes: read-only, overlay or writable.
- **Single static Go binary** with the web UI embedded, no runtime dependencies. Docker image and compose file included.
  The core is stdlib-only apart from one dependency, `modernc.org/sqlite` (pure Go, so every release target still
  cross-compiles with `CGO_ENABLED=0`), because reimplementing a storage engine is not where this project's value is.

## Quick start

### From source

```bash
make build                       # web UI (npm) + Go binary → bin/braindb
./bin/braindb seed examples/sales-company/seed.json -o brain.xml --ingest examples/sales-company/docs
./bin/braindb serve --brain brain.xml        # → http://127.0.0.1:7791
```

Requirements: Go ≥ 1.23. Node ≥ 20 is needed only to build the UI. `pdftotext` (poppler) is optional, for PDFs.

### Docker

```bash
docker compose up -d --build     # sales-company → http://127.0.0.1:7791, research-lab → http://127.0.0.1:7792
docker compose --profile test run --rm smoke     # end-to-end API checks against both services
docker compose logs -f sales
```

Single container:

```bash
docker build -t braindb .
docker run -d --name braindb -p 127.0.0.1:7791:7791 -v braindb-data:/data -e BRAINDB_EXAMPLE=research-lab braindb
```

| variable | default | meaning |
|---|---|---|
| `BRAINDB_DATA_DIR` | `/data` | databases directory (one subdirectory per database) |
| `BRAINDB_BRAIN` | `/data/brain.xml` | single-brain mode (backward compatible) |
| `BRAINDB_ADDR` | `0.0.0.0:7791` in the image, `127.0.0.1:7791` otherwise | listen address |
| `BRAINDB_EXAMPLE` | empty | on first start, seed from a bundled example (`sales-company`, `research-lab`, `braindb-self`), including docs and templates |
| `BRAINDB_BRAIN` extension | `.xml` | picks the backend: `brain.xml` is the file backend, `brain.db` / `brain.sqlite` the SQLite one |
| `BRAINDB_FORMAT` | `xml` | file format: `xml`, `bin` or `both` |
| `BRAINDB_EMBED_PROVIDER` | `hash` | `openai` to use a remote embedding endpoint (see [Embeddings](#embeddings-hash-or-a-real-model)) |
| `BRAINDB_EMBED_MODEL` | empty | model id, e.g. `openai/text-embedding-3-small` |
| `BRAINDB_EMBED_BASE_URL` | provider default | endpoint root, e.g. `https://openrouter.ai/api/v1` |
| `BRAINDB_EMBED_KEY_ENV` | empty | name of the variable holding the API key (never the key itself) |
| `BRAINDB_ALLOW_NO_TOKEN` | `1` in compose | allow listening off-loopback without tokens (container published on 127.0.0.1 only); ignored once a token is set |
| `BRAINDB_TOKEN_READ` | empty | optional read-only token (writes rejected), e.g. for cloud bots |
| `BRAINDB_TOKEN` | empty | when set, `/api/*` requires `Authorization: Bearer <token>` (or `?token=`); `/api/health` stays open |

The image runs as a non-root user with a read-only root filesystem (compose) and a healthcheck on `/api/health`.
The compose file publishes ports on `127.0.0.1` only. Set `BRAINDB_TOKEN` before exposing it any further.

## Use it from an agent (MCP)

> Install in Claude Code, Codex, Cursor, Gemini CLI, Hermes Agent, OpenClaw, Grok Build CLI and Grok Bot (cloud, over HTTPS): [docs/INTEGRATIONS.md](docs/INTEGRATIONS.md).
> Full guide: [docs/AGENTS.md](docs/AGENTS.md): setup for Claude Code, Cursor, CLI/HTTP agents, the working loop, rules to paste into CLAUDE.md / AGENTS.md, patterns, token budget, safety and troubleshooting.
> Skill for writing efficient BDQL: [skills/braindb-bdql](skills/braindb-bdql/SKILL.md). Install it with `cp -r skills/braindb-bdql .claude/skills/` or `make install-skill`.

```bash
# Claude Code
claude mcp add braindb -- /abs/path/bin/braindb mcp --brain /abs/path/brain.xml --actor claude

# Docker (stdio into the running service)
claude mcp add braindb -- docker compose -f /abs/path/docker-compose.yml exec -T sales braindb-entrypoint mcp
```

Cursor / generic `mcp.json`:

```json
{ "mcpServers": { "braindb": { "command": "/abs/path/bin/braindb",
  "args": ["mcp", "--brain", "/abs/path/brain.xml", "--actor", "cursor"] } } }
```

`braindb mcp` also serves the live UI on port 7791, so you can watch the agent work.
Several processes can share one `brain.xml`. They sync through the file: atomic writes, reload on change.

By default an agent gets **two tools**:

| tool | purpose |
|---|---|
| `brain_view` | input: the lit context as XML or Markdown |
| `brain_run` | one [BDQL](#bdql-the-braindb-query-language) script: reads, searches, writes, templates, imports and sources, atomic per script |

A typical turn takes one call:

```sql
CONTEXT FOR "renewal risk Halvorsen" DEPTH 2;
SEARCH "cycle life guarantee 4.5 V" LIMIT 5 SOURCES lab;
REMEMBER decision "Offer 18-month cycle-life guarantee backed by lab run R-114" CONCEPTS "Warranty" LINK TO lab:x204 AS D;
SUPERSEDE d131 WITH $D;
BOOST d121;
```

With `--tools all` (or `mcp: {tools: all}`) the per-operation tools are exposed as well:
`brain_search`, `brain_inspect`, `brain_exec`, `brain_ingest`, `brain_query`, `brain_fill` and `brain_sources`.

Typical loop: `brain_view` → `brain_run` (read what is missing, write what was learned, in one atomic script).

## Create a database from files

```bash
braindb create crm --from ./export/ --from ./dump.sql --config braindb.yaml --dry-run   # preview: importers, tables, FKs, counts
braindb create crm --from ./export/ --from ./dump.sql --config braindb.yaml             # → data/crm/
braindb import --db crm "./notes/**/*.md" --update                                      # incremental; changed items are superseded
braindb db ls
braindb serve --data-dir data                                                           # all databases; pick one in the UI
```

| input | becomes |
|---|---|
| `.md` | file + chunks per heading; front-matter → fields, `tags` → concepts; `[[wiki links]]` → edges |
| `.csv .tsv` | one record per row |
| `.json .jsonl .ndjson .yaml` | records (nested keys flattened, `json.path` selects the array); ctxc `state.json` and brainDB `seed.json` are detected |
| `.sql` | `CREATE TABLE` → section and kind; `INSERT` / `COPY` → records; `FOREIGN KEY` → links; low-cardinality columns → concepts |
| `.sqlite .db` | via the `sqlite3` CLI, when present |
| `.pdf .docx .html .txt` and code | file + chunks |

The UI has the same flow as a **New database** wizard: drop folders, review the dry-run report and the proposed config,
edit it, create.

## Configuration (`braindb.yaml`)

Each database directory holds a `braindb.yaml`. Precedence: flags > env > `braindb.yaml` > defaults.

```yaml
name: crm
goal: Prepare Q4 renewals
context: {depth: 2, threshold: 0.16, hop: 0.85}
kinds:
  - {name: customer, color: "#ff8787", halfLife: 90, base: 0.7, prefix: u, section: customers}
import:
  rules:
    - match: "*.sql"
      importer: sql
      tables:
        customers: {kind: customer, text: "{name} ({segment}) · owner {owner}", concepts: [segment]}
        audit_log: {skip: true}
    - match: "notes/**/*.md"
      importer: markdown
      kind: note
      headingsAsConcepts: true
embedding:
  provider: openai
  model: openai/text-embedding-3-small
  baseUrl: https://openrouter.ai/api/v1
  apiKeyEnv: OPENROUTER_API_KEY
sources:
  - {name: lab, uri: "http://lab:7791", tokenEnv: LAB_TOKEN, weight: 0.7}
mcp: {tools: [view, search, inspect, query, fill, exec]}
```

```bash
braindb config validate --db crm      # errors with line numbers and "did you mean"
braindb config show --db crm          # effective config, secrets masked
```

The full schema is in [docs/SPEC.md](docs/SPEC.md#e3-brain-configuration-braindbyaml). The UI has a **Settings** tab with live validation.

## Knowledge nodes and connector nodes

```
 SQL row r311 ─┐                         ┌─ ticket t140
 note n152 ────┼── ⧉ anchor customer:42 ─┼─ lab:x204 (other brain, via ⧉ bridge)
 deal g204 ────┘   (no knowledge, gain 0.95, transparent)
```

| type | what it joins |
|---|---|
| `anchor` | one real-world entity referenced in many places; created by `RESOLVE` or automatically on import from matching keys |
| `bridge` | a local area and an external brain; every federated link goes through one, so cross-brain edges stay auditable |
| `hub` | a workstream or import batch: one entry point instead of n×m edges |
| `alias` | vocabulary (ARR ↔ annual contract value ↔ ACV); expands query terms |
| `time` | what happened in the same day, week or month |

How connectors behave:
- Their gain is fixed and they never decay.
- Transparent connectors do not consume a depth level.
- A fan-out guard caps very large connectors.
- The context lists the connectors used as **routes**: `route="H:customer-42"` on items, and a `<routes>` section.

```sql
RESOLVE customer BY fields.customer_id, fields.account DRY RUN;
CONNECTOR alias "ARR" TERMS 'annual contract value', 'ACV';
CONNECTOR hub "Q4 renewals" CONNECT g204, g210, d121 AS H;
SELECT id, text FROM memory WHERE route = 'H:customer-42';
SUGGEST CONNECTORS LIMIT 10;
```

## Entities (built-in patterns)

```sql
EXTRACT FROM "Call Kim Aalto (+47 22 33 44 55) before 15 Dec 2026 about €48k, see halvorsen.example/pricing";
--  person Kim Aalto · phone +4722334455 · date 2026-12-15 · money EUR 48000.00 · url https://halvorsen.example/pricing · domain halvorsen.example

SELECT type, COUNT(*) FROM entities GROUP BY type;
SELECT id, text FROM memory WHERE HAS email AND entities.domain = 'halvorsen.example' LIMIT 10;
SELECT id, entities.money FROM memory WHERE entities.money > 'EUR 10000' AND entities.date BETWEEN '2026-12-01' AND '2026-12-31';
RESOLVE email DRY RUN;                       -- one ⧉ anchor per person/company seen in several places
PATTERN ADD sku '\bSKU-\d{5}\b' TYPE id_code NORMALIZE upper;
```

```yaml
patterns: {locales: [en, pl, nb], defaultRegion: NO, dateOrder: dmy}
privacy:  {mask: [phone, iban, card], redactOnIngest: [card]}
```

- Entities are derived data. They are recomputed from text and never stored, so brain.xml stays clean.
- The patterns are Go RE2, so there is no catastrophic backtracking.
- The UI highlights entities inline and includes a live pattern tester.

## Embeddings: hash or a real model

The built-in engine needs nothing and costs nothing, but it is lexical: it matches word shapes, so *"cycle life
guarantee"* and *"warranty duration"* look unrelated to it. For synonym-level recall, name a provider. Nothing else
in the system changes.

```yaml
embedding:
  provider: openai                     # hash (default), or any name from the table below
  model: openai/text-embedding-3-small
  baseUrl: https://openrouter.ai/api/v1
  apiKeyEnv: OPENROUTER_API_KEY        # the variable NAME; the key never enters braindb.yaml or brain.xml
  dim: 1536                            # optional; learned from the first response when omitted
  batch: 64                            # texts per request
  timeout: 30                          # seconds
  retries: 2                           # on 429 and 5xx; 0 = never retry
  fallback: hash                       # hash (default) = degrade on failure | error = fail loudly
  cache: embeddings.cache              # default: next to brain.xml
  inputType: search_document           # only where the model is asymmetric (Cohere, Gemini, Voyage)
  alpha: 0.5                           # graph enrichment (APPNP), unchanged
```

### Providers

Most services speak the OpenAI `/embeddings` format, so they differ only in an endpoint, a header and a batch
ceiling. Three do not, and brainDB speaks their wire format instead of asking you to proxy it: **Cohere** posts
`texts` and returns embeddings grouped by type, **Gemini** posts one request per text inside a batch envelope, and
**Azure OpenAI** puts the deployment in the path with an `api-version` query and an `api-key` header.

Naming the provider is enough; `baseUrl` is only needed for the self-hosted ones.

| `provider` | endpoint it uses | key variable | batch ceiling | wire |
|---|---|---|---|---|
| `openai` | `https://api.openai.com/v1` | `OPENAI_API_KEY` | 2048 | openai |
| `openrouter` | `https://openrouter.ai/api/v1` | `OPENROUTER_API_KEY` | 512 | openai |
| `azure` | your resource endpoint (`baseUrl`) | `AZURE_OPENAI_API_KEY` | 2048 | openai, `api-key` header, deployment in the path |
| `cohere` | `https://api.cohere.com` | `COHERE_API_KEY` | 96 | cohere |
| `gemini` (alias `google`) | `https://generativelanguage.googleapis.com/v1beta` | `GEMINI_API_KEY` | 100 | gemini, `x-goog-api-key` header |
| `voyage` | `https://api.voyageai.com/v1` | `VOYAGE_API_KEY` | 128 | openai + `input_type` |
| `jina` | `https://api.jina.ai/v1` | `JINA_API_KEY` | 2048 | openai |
| `mistral` | `https://api.mistral.ai/v1` | `MISTRAL_API_KEY` | 512 | openai |
| `together` | `https://api.together.xyz/v1` | `TOGETHER_API_KEY` | 512 | openai |
| `deepinfra` | `https://api.deepinfra.com/v1/openai` | `DEEPINFRA_API_KEY` | 512 | openai |
| `huggingface` (alias `hf`) | `https://router.huggingface.co/v1` | `HF_TOKEN` | 256 | openai |
| `ollama` | `http://127.0.0.1:11434/v1` | — | 64 | openai |
| `lmstudio` | `http://127.0.0.1:1234/v1` | — | 64 | openai |
| `vllm`, `tei`, `compatible` | your `baseUrl` | optional | 256 | openai |

- **The batch ceiling is enforced for you.** `batch: 500` against Cohere becomes 96 per request rather than a 400
  from the provider.
- **Asymmetric models.** Cohere, Gemini and Voyage distinguish stored documents from queries. A brain embeds what it
  stores, so the document setting is the default (`search_document`, `RETRIEVAL_DOCUMENT`, `document`); override it
  with `inputType`. One brain uses one setting for both sides today.
- **`GET /api/embedding`** returns this table, so the Settings form offers exactly the providers the server accepts.
- An unknown name is rejected when the config is saved, with the known names in the message.

Same settings from the environment, for containers:

```bash
BRAINDB_EMBED_PROVIDER=openai
BRAINDB_EMBED_MODEL=openai/text-embedding-3-small
BRAINDB_EMBED_BASE_URL=https://openrouter.ai/api/v1
BRAINDB_EMBED_KEY_ENV=OPENROUTER_API_KEY
BRAINDB_EMBED_INPUT_TYPE=search_document     # Cohere, Gemini, Voyage
BRAINDB_EMBED_API_VERSION=2024-02-01         # Azure only
```

How it behaves:
- **Cached on disk.** Vectors are keyed by `sha256(model + text)` in `embeddings.cache` next to the brain. Re-importing
  unchanged files, restarting the server or reloading the brain costs no API calls. Changing the model invalidates the
  cache on its own, because the model id is part of the key.
- **Batched.** One import is one round trip per `batch` texts, not one per node. Queries, template slots and purpose
  checks go through the same path.
- **Degrades, or fails loudly.** With `fallback: hash` an unreachable provider produces hash vectors of the right
  dimension and the status line says the engine is degraded; with `fallback: error` the operation fails instead.
- **Keys stay in the environment.** `apiKeyEnv` is a variable name. A missing variable is an error at load time, not a
  silent downgrade of the whole brain.
- The graph enrichment (`h = (1−α)·x + α·mean_w(neighbours)`) runs on top of whichever engine is configured.

> Switching engines changes every vector, so vector and hybrid search results change with it. Graph activation,
> weights and BDQL are unaffected.

## Purpose: what belongs in this brain

```yaml
purpose:
  name: Tessaline sales brain
  description: >
    Knowledge the sales team needs to sell and renew NaCore battery leases: accounts, deals, pricing,
    product claims backed by lab evidence, SLAs, warranties and customer commitments.
  inScope:  {topics: ["deals and pipeline", "pricing and discounts", "SLAs and warranties"], kinds: [deal, account, fact, decision]}
  outOfScope: {topics: ["private matters", "food, sports, entertainment"], entities: [card]}
  examples: {accept: ["Q4 commit is EUR 1.62M"], reject: ["Lunch menu for Friday"]}
  admission: {mode: advise, accept: 0.35, review: 0.20}      # advise (default) | off | enforce
```

```sql
SHOW PURPOSE;
CHECK "Lunch menu for Friday";                    -- score 0.08 → rejected (top conflict: "food, sports, entertainment")
SELECT id, text, purpose.score FROM quarantine;   -- items waiting for review
ADMIT r412 IN pipeline;   REJECT r414 REASON 'off-topic';
REMEMBER fact "…" OVERRIDE PURPOSE REASON 'needed for audit';
```

How admission works:
- Each incoming item is scored against the description, the in- and out-of-scope topics and the examples, using the brain's own embeddings and rules.
- **`advise` is the default and the mode to stay in.** Everything is stored; the score and the reasons are attached and
  reported. Nothing is ever lost because a cosine fell below a number.
- `enforce` is opt-in. Items above `accept` are stored, items in the review band are quarantined (invisible to everyone
  except reviewers until admitted), items below `review` are rejected with reasons. Turn it on only after running in
  `advise` for a while and reading what *would* have been dropped: a wrongly rejected memory is an invisible,
  unrecoverable loss, and it is the worst failure this database can have.
- `off` skips scoring entirely.
- Import dry-runs show the decision for every file.

## Users, roles and access-controlled knowledge

```yaml
access:
  enabled: true
  labels: [{name: finance}, {name: hr}, {name: ip-sensitive}]
  defaults: {sections: {pricing: [finance]}}
  roles:
    - {name: sales-rep, grants: [{actions: [read, write], labels: ["*"]}, {deny: true, actions: [read], labels: [hr]}]}
    - {name: analyst,   grants: [{actions: [read], labels: [finance]}]}
  users:
    - {name: kim,     roles: [sales-rep], tokenEnv: BRAINDB_TOKEN_KIM}
    - {name: grokbot, roles: [],          tokenEnv: BRAINDB_TOKEN_GROKBOT, dbs: [sales]}   # public knowledge only
```

- A node is visible only to users holding **all** its labels. Chunks and records inherit labels from their file or table.
- For everyone else the node, and the paths through it, are switched off.
  - Every read (context, all search modes and their scores, SELECT and counts, paths, suggestions, fill, entities, stats, the live event feed) behaves exactly as if the hidden nodes had been deleted.
  - Enriched vectors only aggregate neighbours the node's readers can also see. Similarity scores never depend on hidden text.
  - An invisible id gets the same `no such item` error as a nonexistent one.

```sql
WHOAMI;
GRANT read ON LABEL finance TO ROLE analyst;   DENY read ON LABEL hr TO ROLE sales-rep;
LABEL WHERE section = pricing ADD finance DRY RUN;
REMEMBER fact "Marrow Creek floor price is €41,000" LABELS (finance);
EXPLAIN ACCESS d140 FOR kim;                       -- admin: why kim can or cannot see it
AS USER grokbot; SEARCH "discount for Marrow Creek";  -- admin preview of another user's view
```

`braindb publish --labels public,sales` builds a binary that physically contains only what that label set may see.

## BDQL, the brainDB Query Language

One language for every operation. A script that writes is **atomic**: everything applies, or nothing does.

```sql
USE crm;
-- read
CONTEXT FOR "Q4 renewals" DEPTH 2 FORMAT md;
SELECT id, text, fields.account, fields.value FROM memory
  WHERE kind = deal AND fields.stage = 'negotiation' AND fields.value > 10000
  NEAR "renewal risk" ORDER BY score DESC LIMIT 10;
SELECT fields.stage, COUNT(*), SUM(fields.value) FROM memory WHERE kind = deal GROUP BY fields.stage;
SEARCH "cycle life guarantee" MODE hybrid LIMIT 5 SOURCES lab;
PATHS FROM q143 TO d127;
EXPLAIN SELECT * FROM memory WHERE fields.owner = 'Kim Aalto';

-- write
REMEMBER fact "Halvorsen asked for a 24-month warranty" CONCEPTS "Warranty", "Renewal" LINK TO g204 AS F;
INSERT INTO deal (text, account, stage, value) VALUES ('Upsell telematics', 'Ostrava Logistics', 'proposal', 12500);
UPDATE memory SET fields.stage = 'won' WHERE id = g204;
SUPERSEDE f107 WITH $F;
FORGET WHERE kind = note AND age > 90 AND NOT pinned DRY RUN;

-- documents, data, sources
FILL TEMPLATE 'account-brief.yaml' OUTPUT json WITH SOURCES;
IMPORT '/data/export/' UPDATE;
SOURCE ADD lab 'http://lab:7791' TOKEN ENV LAB_TOKEN WEIGHT 0.7;
```

```bash
braindb run -e "SELECT id, text FROM memory WHERE lit LIMIT 5"
braindb run examples/sales-company/queries.bdql --db sales
braindb shell --db sales                     # REPL: multi-line, history, \d, \f md, \q
curl -s localhost:7791/api/run -d '{"script":"SHOW stats;"}'
```

How a script behaves:
- **Errors** report the statement, line and column, with a caret excerpt and "did you mean".
- **Bulk guards:** `UPDATE` and `FORGET` without `WHERE` are errors. A bulk write over 100 nodes needs `CONFIRM`, and `DRY RUN` previews it.
- **Permissions:** read-only brains (published `readonly`, read-only tool sets) reject writes.
- **UI editor:** syntax highlighting, autocomplete for ids, kinds, fields and templates, lint, and one result tab per statement.

The full grammar is in [docs/SPEC.md](docs/SPEC.md#addendum-g-bdql-the-braindb-query-language).

## Querying

### Raw data

```bash
braindb query --brain brain.xml 'kind:deal stage=negotiation value>10000 sort:-value limit:10' --format md
braindb query --brain brain.xml '{"where":{"kind":["fact"],"concept":"pricing","minWeight":0.2},"near":"discount policy","limit":5}' --format yaml
```

| term | meaning |
|---|---|
| `kind:a,b` | restrict to kinds |
| `section:` | restrict to a section |
| `concept:` | restrict to a concept |
| `lit:true` | only nodes in the current context |
| `pinned:` | pinned or not |
| `superseded:` | superseded or not |
| `w>=0.2` | weight filter |
| `age<=30` | age filter, in days |
| `field<op>value` | field filter; ops `= != > >= < <= ~` (contains); numbers and money compare numerically |
| `"free text"` | text filter |
| `near:"text"` | adds a hybrid relevance score |
| `sort:-field` | sort; `-` means descending |
| `limit:` | row limit |
| `select:a,b,fields.x` | columns to return |

### Context

```bash
braindb context --brain brain.xml --format md -o text.md       # shared context (focus stored in the brain)
braindb query --brain brain.xml --context --query "renewal risk" --format json   # ad-hoc slice, nothing changes
```

### Templates

Every string leaf (JSON / YAML / XML) or `{{ … }}` placeholder (Markdown / text) is a slot. A slot has the form
`description[:type][|option…]`.

```yaml
client:
  name: "client name"
  owner: "account owner:name"
  renewal_date: "contract renewal date:date"
  arr: "annual contract value:money"
deals[]: "open deals|kind=deal|limit=5"
deals[].title: ".text"
deals[].stage: ".fields.stage"
deals[].risk: "main risk for this deal"
```

```bash
braindb fill --brain brain.xml examples/sales-company/templates/account-brief.yaml --output json --sources
```

| element | values |
|---|---|
| Types | `text` `name` `number` `money` `percent` `date` `email` `phone` `url` `bool` `list` `id` |
| Options | `kind=` `section=` `concept=` `scope=context\|memory` `limit=` `min=` `field=` |
| Item fields in list slots | `.id` `.kind` `.text` `.weight` `.fields.<name>` |

Filling is deterministic: retrieval plus typed extraction, no LLM inside. Low-confidence slots come back as `null` and are
listed in `missing`.

## External sources (federation)

> **Status: partial.** Sources and external stubs are stored, and `SOURCE ADD/REMOVE` works. Remote transports and federated activation, search, query and fill are planned.

```bash
braindb source add lab http://lab:7791 --token-env LAB_TOKEN --weight 0.7     # another brainDB server
braindb source add specs url+https://example.org/datasheet.html --ttl 86400  # a web page, chunked and cached
braindb source add archive file:///data/old/brain.xml                        # another brain file (read-only)
braindb source ls
```

| source URI | what it mounts |
|---|---|
| `http(s)://host:port` | another brainDB server |
| `file://…/brain.xml` | a brain file |
| `snap://…/brain.snap` | a snapshot |
| `exec://…/published-brain` | a published brain binary, over MCP |
| `url+https://…` | a web document |

- Link a memory to a remote node through the UI (Inspector → *Link external…*) or the API:
  `brain_exec {"ops":[{"op":"link","a":"d121","b":"lab:x204","w":0.7}]}`.
  Remote nodes are cached as stubs, so the brain still renders offline.
- When activation reaches an external link, brainDB asks the remote brain for its slice (`federate-depth`, threshold scaled by the source weight).
- `sources: ["*"]` fans searches and queries out to every source.
- Unreachable sources degrade to the cached stubs and are reported in the status line.
- Tokens are referenced by env var name, never stored in `brain.xml`.
- Remote content is treated as data, never as instructions.

`examples/federation/` links the sales brain to the research-lab brain. A sales question about a product guarantee is
answered with the lab's test evidence.

## Publish a brain as a binary

> **Status: planned.** This section describes the specified design.

```bash
braindb publish --brain brain.xml -o dist/acme-sales-brain --name acme-sales --version 1.0.0 --mode readonly
./dist/acme-sales-brain --version
claude mcp add acme-sales -- /abs/path/dist/acme-sales-brain       # no arguments = MCP server
```

The brain is compiled into a gzip/gob snapshot with precomputed vectors and indices. The snapshot is appended to a copy of
the `braindb` executable, checked with sha256 at start, and needs no Go toolchain at publish time.

| mode | behaviour |
|---|---|
| `readonly` | query, search, view and fill only |
| `overlay` | the agent's writes go to `$XDG_DATA_HOME/braindb/<name>/overlay.xml`; the published knowledge stays untouched |
| `writable` | extracts the brain and behaves like `braindb mcp` |

Other options:
- `--tools view,search,query,fill` limits the exposed tools.
- `--target linux/arm64,darwin/arm64,windows/amd64` builds for other platforms, using the runtimes from `make release`.

## HTTP API

The API binds to `127.0.0.1:7791` by default. All endpoints are JSON unless noted; the full contract is in [docs/SPEC.md](docs/SPEC.md).

| endpoint | purpose |
|---|---|
| `GET /api/patterns`, `POST /api/patterns/test`, `GET /api/entities` | pattern library, live extraction, entity index |
| `POST /api/run` | run a BDQL script (`{script, format, dryRun}`); `/api/run/explain` parses and plans only |
| `POST/GET/DELETE /mcp` | MCP over Streamable HTTP, for remote and cloud agents (token required off-loopback) |
| `GET /api/whoami` | the current user, roles, labels and databases |
| `GET /api/health` | liveness, version, node count, storage status and the embedding engine |
| `GET /api/stats` | counts, index sizes, cache hit rate, latencies |
| `GET/POST /api/dbs`, `DELETE /api/dbs/{name}` | databases (every endpoint takes `?db=` or `X-BrainDB-Database`) |
| `POST /api/import` | bulk import (multipart, folders keep relative paths, `dryRun`) |
| `GET/PUT /api/config` | `braindb.yaml`: read, validate, save |
| `GET /api/state` | full snapshot: nodes with weight and activation, edges, stats, params |
| `POST /api/context` | set focus, depth, threshold, hop, goal |
| `POST /api/context/query` | ad-hoc context slice |
| `GET /api/context.md` / `GET /api/context.xml` | current context as Markdown / XML |
| `POST /api/search` | `{query, mode, k, focus}` |
| `POST /api/query` | raw data query |
| `POST /api/fill` | template fill |
| `GET /api/templates` | available templates |
| `POST /api/exec` | atomic write batch (400 + nothing applied on error) |
| `GET /api/inspect` | `?id=&what=item,links,near,test,suggest,paths&to=` or `?what=discover` |
| `POST /api/ingest` | multipart file upload |
| `GET /api/layout` | `?view=embed\|tree` positions for the 3D views |
| `GET /api/vector` | raw and enriched vector of a node |
| `POST /api/embedding` | α, hops, raw/enriched, hybrid λ |
| `POST /api/kinds` | add a custom kind |
| `GET /api/brain.xml` / `PUT /api/brain.xml` | whole brain |
| `GET/POST /api/sources`, `DELETE /api/sources/{name}`, `POST /api/sources/{name}/sync` | external sources |
| `GET /api/events` | server-sent events (every change, by UI, agent or file) |
| `GET /api/journal` | last 100 events |

## Performance and storage

> **Status: partly built.** The SQLite backend, two-way migration, the raw-query pushdown over the field indexes and
> `braindb bench` ship in 0.0.2. The brain is still loaded whole into memory, the stem index is written but not read,
> and the latency targets below are still targets: nothing in section "Targets, not measurements" has been measured.

### SQLite, not a hand-rolled storage engine

An earlier version of this document promised a custom write-ahead log, inverted and range indexes, a query planner, an
HNSW vector index and a from-scratch Redis client, all on the standard library. That is years of work to re-create
what an embedded database already does correctly, and the bugs would be in the one layer that must never lose data.

0.0.2 ships SQLite instead: WAL, indexes, a planner, concurrent readers and crash safety come with it, and the brain
stays a single file you can copy. The deliberate cost is one dependency in the storage layer (`modernc.org/sqlite`,
pure Go); the rest of brainDB stays stdlib-apart-from-SQLite, and `brain.xml` remains the portable, diff-friendly
export format.

```yaml
storage:
  backend: file            # file (default, brain.xml) | sqlite — validated against path's extension
  path: brain.db           # overrides the brain path; .db / .sqlite select the SQLite backend
```

Switching is one command, and lossless both ways:

```bash
braindb migrate --brain brain.xml --to sqlite    # writes brain.db next to it, source untouched
braindb migrate --brain brain.db  --to file      # and back; the XML comes out byte-identical
braindb serve   --brain brain.db                 # serve straight from SQLite
```

| format | role |
|---|---|
| `brain.xml` | readable, diff-friendly, versionable in git; always available via `braindb export` |
| `brain.db` | SQLite (WAL): durable, crash-safe, concurrent readers. Raw queries push `kind`, `section`, `id` and field predicates down to the field/node indexes; the stem index is written but not yet read, and the brain itself is still held in memory |

```bash
braindb export --db crm -o brain.xml    # readable copy from any backend
braindb verify --db crm                 # integrity check
braindb bench  --nodes 100000           # synthetic benchmark → docs/BENCHMARKS.md
```

### Targets, not measurements

These are the numbers brainDB is being built to hit on a 100k-node brain. **Nothing here has been measured yet.**

| operation | p95 target |
|---|---|
| activation, depth 2 | < 50 ms |
| hybrid search | < 100 ms |
| indexed query | < 30 ms |
| import of 100k CSV rows | < 20–30 s |

Measured results, when there are any, go in [docs/BENCHMARKS.md](docs/BENCHMARKS.md).

## Data model

Decay tilts ranking toward the recent. It must not delete knowledge, so every kind has a **floor**: the share of its
base weight that age can never take away. Only `SUPERSEDE` and `FORGET` remove knowledge.

| kind | half-life | floor | base weight | behaviour |
|---|---|---|---|---|
| decision | 365 d | 0.80 | 0.70 | durable: a decision stands until it is superseded |
| fact | 180 d | 0.70 | 0.60 | durable |
| record | 180 d | 0.70 | 0.50 | durable: a row imported from a table does not expire |
| file | 180 d | 0.60 | 0.50 | durable |
| chunk | 120 d | 0.55 | 0.45 | durable, inherits its file |
| summary | 90 d | 0.40 | 0.60 | goes stale, does not vanish |
| idea | 60 d | 0.25 | 0.50 | fades, stays findable |
| task | 14 d | 0 | 0.55 | attention-shaped: really does go away |
| question | 7 d | 0 | 0.50 | attention-shaped |
| note | 7 d | 0 | 0.40 | attention-shaped |
| concept ◆ | no decay | — | 0.80 | never decays |

- **Weight:** `w = base × (floor + (1 − floor) × 0.5^(age / half-life))`, × 0.3 when superseded.
  With `floor: 0` this is the plain half-life curve. Pinned memories do not decay, and `boost` resets the clock.
- Override per kind in `braindb.yaml` (`kinds: [{name: fact, halfLife: 30, floor: 0}]`), or from Go with
  `Brain.SetKindDecay`. `LegacyDecay()` restores the pre-0.0.2 policy, where everything faded to zero.
- **Activation:** `a(v) = a(u) · w(u,v) · hop · (0.35 + 0.65 · w(v))`, level by level up to `depth`, kept while `≥ threshold`.
- **Embeddings:** local `hash-256` (stems, bigrams, char trigrams) by default, or any OpenAI-compatible endpoint
  (see [Embeddings](#embeddings-hash-or-a-real-model)). Either way they are enriched over the graph (APPNP):
  `h = (1−α)·x + α·mean_w(neighbours)`.
- **Storage:** `brain.xml`, human-readable and diff-friendly, with a lossless round-trip. Every change is appended to
  `brain.journal.jsonl`.

## Examples

| example | scenario |
|---|---|
| [`examples/sales-company`](examples/sales-company) | B2B sales team: accounts, deals, pricing, renewals; pipeline CSV; templates for an account brief, a quote and a deal review |
| [`examples/research-lab`](examples/research-lab) | research laboratory: hypotheses, experiments, samples, protocol versions; measurements CSV; templates for an experiment report, a sample sheet and a weekly status |
| [`examples/braindb-self`](examples/braindb-self) | brainDB's own project brain: decisions and the ones they reversed, spec addenda, the measured retrieval table, the CLI surface, the roadmap, and an agent playbook; templates for release notes, a feature-status sheet and an agent onboarding brief |

Each example has a `seed.json`, `docs/` to ingest, `templates/`, and a README with queries to try.
[`examples/federation`](examples/federation) runs both and mounts the lab into the sales brain.
[`examples/import-demo`](examples/import-demo) is a raw pile (Markdown notes, a PostgreSQL dump, a MySQL dump, JSONL tickets, nested JSON, CSV) plus a `braindb.yaml`, turned into a database with one `braindb create`.
[`examples/braindb-self`](examples/braindb-self) documents this repository inside brainDB, and measures retrieval on itself: a
second gold set, a different domain, the same two conclusions as [docs/BENCHMARKS.md](docs/BENCHMARKS.md).

## Development

```bash
make build            # web + Go → bin/braindb
make test             # go vet + go test + web typecheck
make web-dev          # Vite dev server with /api proxy to :7791
make storybook        # Storybook on :6006 (fixtures, mock API)
make release          # cross-compiled runtimes in dist/runtime/
make docker           # docker build -t braindb .
make smoke            # scripts/smoke-test.sh against a running server
make eval             # retrieval-quality evaluation on the sales example
docker build --target test .                       # tests inside Docker
sh scripts/mcp-smoke.sh ./bin/braindb mcp --brain /tmp/b.xml --no-http
```

```
cmd/braindb/            CLI: serve · mcp · seed · state · context · ingest · query · fill · publish · snapshot
internal/brain/         model, activation, embeddings, search, query, templates, exec, ingest, XML/MD/YAML I/O
internal/mcp/           MCP stdio server
internal/server/        HTTP API, SSE, embedded UI
web/                    React + TypeScript + three.js UI, Storybook
examples/               sales-company, research-lab, braindb-self, import-demo, federation
docs/index.html         guide source, published at https://braindb.eu/docs/
docs/SPEC.md            full specification
docs/AGENTS.md          how to use brainDB in AI agents
docs/INTEGRATIONS.md    install in Claude Code, Codex, Cursor, Gemini, Hermes, OpenClaw, Grok
skills/braindb-bdql/     agent skill for writing efficient BDQL (cheat-sheet, recipes)
docker/ scripts/        container entrypoint, smoke tests
```

## Benchmarks and parity

Two things are worth checking before trusting any of the claims above.

- **[docs/BENCHMARKS.md](docs/BENCHMARKS.md)** holds measured numbers. Where it says a number has not been measured,
  it has not been measured. Retrieval quality **is** measured; latency and scale are not.

  ```bash
  braindb eval --brain brain.xml --gold examples/sales-company/eval.json
  ```

  32 hand-judged queries over the sales example, split into queries that share *words* with the target memory and
  queries that share only *meaning*. The headline result, and the reason the embedding engine is pluggable:

  | bucket | `graph` MRR | `vector` MRR | queries with nothing relevant in the top 10 |
  |---|---|---|---|
  | lexical (16) | 1.00 | 0.85 | 0 of 16 |
  | paraphrase (16) | 0.14 | 0.07 | 12&ndash;14 of 16 |

  So: with the offline `hash-256` engine, graph activation beats flat vector top-k on every aggregate metric, and
  **both** fail on paraphrases, because a lexical engine has no signal to give either of them. If your users phrase
  questions in their own words, configure a real embedding model.
- **Parity.** brainDB is a Go port of an earlier JavaScript prototype of the same algorithms. `internal/brain/parity_test.go`
  replays a fixed brain through both and pins the Go output to the prototype's: activation order and values, float32
  vectors bit for bit, all four search modes, suggestions, ablation, paths, discovery and chunking. Two independent
  implementations agreeing to the bit is the strongest evidence in this repository that the core model is specified
  rather than improvised. Run it with `go test ./internal/brain -run TestParity`.

## About the author

brainDB is built by **Gabriel Koziestański** as part of a longer line of work on memory and context systems for
agents, multi-agent protocols, and tools that make AI usable inside real organisations.

If you want to help it go further: run it on a real pile of your own knowledge and open an issue describing what broke.
That is more useful than anything else right now. Research partnerships, pilots and sponsorship are welcome too —
open an issue titled "Research / sponsorship".

## License

brainDB is licensed under the [Apache License 2.0](LICENSE). Copyright © 2026 **Gabriel Koziestański**.

Anyone may use, modify and distribute brainDB, including commercially. When you do:
- **Keep the attribution.** Any copy or derivative work you distribute must include the [NOTICE](NOTICE) file, which credits
  Gabriel Koziestański as the original author. If your product shows third-party notices (an About, Credits or Licenses
  screen, or `--version` output), the attribution belongs there too. For example:
  *"Built on brainDB by Gabriel Koziestański (Apache License 2.0)."*
- **Mark your changes.** Modified files must carry a notice that you changed them.
- **Keep the license.** Include a copy of the Apache License 2.0.
