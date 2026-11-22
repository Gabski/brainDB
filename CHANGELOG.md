# Changelog

## Unreleased

### Fixed

- **Two agents on one brain no longer lose each other's writes.** Every write loaded the file, applied a change and
  wrote the whole brain back, with a lock that only ordered goroutines inside one process. Two `braindb mcp`
  processes on the same file silently discarded each other's work: in a test, two agents saving 20 facts each left 22
  of 40 in the brain while the journal reported all 40. Writes now take an advisory file lock (`brain.xml.lock`,
  `flock` on Unix, `LockFileEx` on Windows) around reload, change and save, and the shutdown save (`Flush`) no longer
  writes an old in-memory copy over a newer file.
- **A directory holding only `brain.db` is found.** `--data-dir/--db`, `db ls`, `import`, `eval` and the server
  looked for `brain.xml` by name, so a SQLite-only database was invisible, and `braindb mcp --db NAME` on it created a
  new, empty `brain.xml` next to the real data: the agent saw an empty brain and no error. A database is now whichever
  of `brain.db` / `brain.xml` exists (the newer one when `migrate` left both), and an unknown `--db` is an error that
  lists the databases that do exist.

### Changed

- **`brain_view` is capped.** It returned the whole lit slice: 23,600 characters (about 5,900 tokens) on the
  `sales-company` example, while the status line said `~3061 tok`, because that figure counts only the text of the lit
  memories and not the tags, attributes and tables around them. `brain_view` now returns at most 3000 rendered tokens
  (`max_tokens` per call, `0` = no cap, or `budget.viewTokens` in `braindb.yaml`), drops the least active memories
  first, never the focus, and says how many it left out. The same example now costs about 2,700 tokens.
- **`budget.warnTokens` works.** The key was accepted and documented but the status warning was hard-coded to 4000.
- **Agents are told when search is lexical.** With the built-in `hash-256` engine the server instructions say that
  search matches words, not meaning, and tell the agent to search with the notes' vocabulary and give every
  `REMEMBER` 2-3 concepts. The "live view not started" message no longer blames "another process serves the UI" when
  that process serves a different database.

### Added

- **Switching databases from MCP.** A session can list and change its database without a restart. `brain_use` with no
  argument lists the databases with what each is for; `brain_use name` switches the session and answers with that
  database's purpose. `USE name;` as the first statement of a `brain_run` script now works over MCP too (reads or
  writes one script in another database), and `SHOW dbs` lists them. Over HTTP the token's `dbs` scope applies, and a
  database with access control is reachable only with the admin token.
- **Project databases.** `braindb init [NAME] [--example X] [--mcp]` creates `.braindb/NAME/` (with a `.gitignore` for
  the journal, cache and lock files) and, with `--mcp`, a `.mcp.json` entry that needs no paths. `braindb mcp` and
  `braindb run` find the nearest `.braindb/` from the working directory, add the shared databases from
  `$BRAINDB_HOME` or `~/.local/share/braindb`, and start in the database named like the project directory (or
  `default`, or `--db` / `BRAINDB_DB`). `--brain FILE` still means exactly that file.

## 0.0.2 — 2026-11-20

A storage backend, four changes to defaults and one new subsystem. Three of the default changes alter behaviour on
existing brains.

### Fixed

- **Clicking the user menu killed the top bar.** `/api/whoami` sends Go nil slices, which marshal to JSON `null`,
  while the UI typed `roles`, `labels` and `dbs` as lists and called `.join()` on them. Opening the menu threw
  `Cannot read properties of null (reading 'join')` and unmounted the whole top bar, so the click looked like it did
  nothing. Three fixes, because one was not enough: the server now sends `[]` instead of `null` (with a test that
  fails on any null list in `whoami`), the UI reads every one of those lists as nullable, and the top bar sits inside
  an error boundary so one bad field can no longer take the shell down.

- **The "new database" wizard failed on its own preview.** Step 2 asked the server to dry-run an import under the new
  name and got `no database "test" (have: brain)`: the database does not exist yet, which is the whole point of the
  wizard. A dry run addressed to an unknown database (`POST /api/import` with `dryRun`, `PUT /api/config?dryRun=1`) now
  runs on a throwaway in-memory brain and writes nothing; anything that would write still returns 404.
- **The wizard is no longer offered where it cannot work.** `CreateDB` needs `--data-dir`; a server started with
  `--brain` (which is what the Docker entrypoint does) serves one file and cannot create databases, yet the UI still
  showed "New database…" and only failed at the last step. `/api/health` now reports `manageDbs`, and the connection
  menu replaces the entry with "started with --brain: one database only". The command palette drops it too.

- **The UI index is no longer cached.** Asset file names carry a content hash, but `index.html` names them and was
  served with no cache policy at all, so a browser could keep the previous build for a long time after an upgrade —
  and 404 on its assets once they were gone. `index.html` and the SPA fallback are now `no-cache`, the hashed
  assets under `/assets/` are `immutable`.

- **Schema tab crashed on a plain file backend.** `/api/stats` reports `indexes`, `latency` and `cacheHitRate` only
  in the Redis build, but `ServerStats` declared them required and `StoragePanel` called `.map` on them, so the tab
  fell back to its error boundary with "Cannot read properties of undefined (reading 'map')" on every default,
  file-backed server. The three fields are optional now, their tables render only when present, and the panel says
  where those numbers come from. A `stats-file.json` fixture captured from a running server plus two stories keep it
  covered — every existing stats fixture carried Redis-shaped data, which is why the stories never caught it.

### Added

- **SQLite as a second storage backend** (`brain.db`, WAL). The F2 schema with node, field, edge, stem and vector
  indexes; a round trip verified by comparing the XML before and after; two-way migration through `braindb migrate`
  and `POST /api/migrate`; and serving straight from `brain.db`. The backend is chosen by the path extension
  (`.xml` → file, `.db` / `.sqlite` → SQLite) and validated against `storage.backend` in `braindb.yaml`.
  - One dependency, `modernc.org/sqlite`: pure Go, so all six release targets still cross-compile with
    `CGO_ENABLED=0`.
  - The brain is still loaded whole into memory, and the stem index is written but not read. Lazy loading and text
    pushdown stay on the roadmap.
- **Raw-query pushdown over the field index.** `kind`, `section`, `id` and field predicates are answered from the
  indexes as a sound pre-filter, with the in-memory predicate still re-checking every candidate. The probe is bounded
  and its cap doubles as the selectivity test: an unselective predicate falls back to the scan, which is measurably
  faster, so there is no regression.
- **`braindb bench --nodes N [--backend file|sqlite|both]`** — a synthetic brain measured on both backends:
  save, load, query and search timings, and whether a query was answered from the indexes.
- **Cross-site guards that run before authentication**, because no token is configured by default: an `Origin`
  check, a `Content-Type` requirement on writes, and a loopback `Host` check against DNS rebinding.
- **`server` block in `braindb.yaml` implemented** (`addr`, `tokenEnv`, `tokens` with scopes and `dbs`,
  `allowNoToken`) instead of being parsed and ignored. Token values come from environment variables only.
  `POST /api/config` now requires admin, like `PUT`.
- **`GET /api/entities` and `GET /api/sources/<name>`**; federation endpoints that are specified but not built
  answer an honest 501 instead of pretending.
- **`touched` column** exposing the absolute timestamp the XML already carried.
- **Contract tests pinning the API field names TypeScript cannot check**, plus tests for NULL semantics, the
  pushdown against the scan, config splicing, pattern persistence and the cross-site guards.
- **CI that actually gates.** `.github/workflows/ci.yml` runs on every push and pull request: `gofmt -l` (fails on
  drift), `go vet`, `go test -race`, `go build`, then the web job — `npm ci`, typecheck, unit tests, the emoji lint,
  the Vite build and the Storybook build. `make test` now mirrors the same checks locally.
- **Unit tests for the web UI** (Vitest, 157 tests over `src/model`): filters and facets of the Browse tab, the
  Search tab's narrowing / grouping, the graph index, the syntax tokenizers and statement splitter, grid export
  rendering, and the small helpers. 75% statement coverage of `src/model`. Components stay covered by Storybook.
  - The suite caught a real bug while being written: a typed `kind:` prefix and a kind chip were ORed instead of
    ANDed, so `kind:task` with the `fact` chip on showed both. Fixed in `browseFilter` — chips OR inside a group,
    AND against the prefixes.
- **Database settings as forms (web UI).** The Settings tab was one textarea holding `braindb.yaml`; it is now nine
  sections over the same file: Overview (identity and what the file configures), Context (depth, threshold, hop,
  superseded, budget), Kinds & decay (half-life, floor and base per kind, plus an override starting from the policy
  in force), Embeddings (provider, model, key env var, batching, graph enrichment), Import (chunking and the rule
  table), Storage (backend and path, what the server actually runs on, and the conversion between backends),
  Purpose (admission mode and thresholds), Access & MCP, and Raw YAML.
  - Storage says plainly when the file asks for one backend and the server is running on another, which is the
    normal state right after switching `storage.backend`, since that key takes effect on the next start.
  - Every key says whether it is **written in the file** or left to the **default**, with a one-click reset that
    removes the line. A form that hides that difference teaches the wrong model of the file.
  - `Diff` shows exactly which lines a form changed before saving, and the file is dry-run validated as you type.
  - **API and MCP tokens are added from the form.** `server.tokenEnv` for the admin token, a table of `server.tokens`
    (name, environment variable, scope `read`/`write`/`admin`, the `dbs` limit shown read-only), and `allowNoToken`
    with what it is for. The field takes the *name of an environment variable* and refuses anything that is not one,
    which keeps a pasted secret out of a file that gets committed. An agent on `/mcp` authenticates with the same
    tokens; a read token may call it, but its writes are refused.
  - The forms edit the YAML text, not a parsed model (`src/model/yamlEdit.ts`, 38 unit tests): comments survive, and
    a change rewrites one line. Anything the editor cannot do safely — a multi-line flow map, a key inside a list
    whose item is missing — raises `YamlEditUnsupported` and points at the raw editor instead of guessing.

- **Browse tab (web UI).** A faceted browser over the whole snapshot, for reading knowledge without writing BDQL.
  - Free-text filter with `kind:`, `section:` and `source:` prefixes and quoted phrases; matches are highlighted in
    the result rows and in the reading pane.
  - Facet sidebar with live counts (each group counted with the other groups applied): kinds, sections, sources and
    entities (`type:value`, loaded on demand), plus flags for pinned / concepts / superseded / external stubs.
  - Sort by weight, activation, age, kind, section, text or id, either direction; results page in at 120 rows.
  - Reading pane: the whole memory with entity highlighting, weight/base/age/activation, fields table, entity chips
    and the node's links — clicking a link walks to that memory, with a back button. Up/Down moves, Enter opens the
    memory in the Inspector and flies to it in Graph 3D.
- **Search tab rework (web UI).** The hits are now readable where they are found.
  - Every hit shows a snippet around the match with the query words highlighted; the caret expands the whole memory
    in place, with its fields. Kind, pin, superseded and the federated source are on the row.
  - Narrowing bar: free text over the hits plus kind / section / source facet chips with counts, all client-side —
    no second request. In tree mode a kept hit whose parent was narrowed away is re-parented to the nearest kept
    ancestor, so no branch disappears silently.
  - Grouping by kind, section or source, and a hit-count selector (top 8 / 15 / 30 / 50) passed to `/api/search`.
  - Recent searches (last 20, de-duplicated, in `localStorage`) come back as one-click chips.
- **Inspector navigation (web UI).** Reading a chain of linked memories no longer loses the trail.
  - Back / forward through the visited nodes, in the header and on Alt+Left / Alt+Right (50 steps, reset when the
    database or the viewed-as user changes).
  - Sections (Properties, Fields, Entities, Links, Vector, Nearest, Item XML) collapse, and what was left closed is
    remembered in `localStorage`; Vector and Item XML start closed.
  - Links are bucketed per edge type with counts, and the type chips narrow the list.
  - Copy id button in the header.
- **An embedding-provider registry, with the three wire formats that are not OpenAI's.** `embedding.provider` now
  takes a name — `openai`, `openrouter`, `azure`, `cohere`, `gemini`, `voyage`, `jina`, `mistral`, `together`,
  `deepinfra`, `huggingface`, `ollama`, `lmstudio`, `vllm`, `tei`, `compatible` — and that name decides the endpoint,
  the auth header, the batch ceiling and the wire format. `baseUrl` is now an override, needed only for the
  self-hosted ones.
  - **Cohere** (`/v2/embed`, `texts` in, `{embeddings:{float}}` out) and **Gemini**
    (`/models/{model}:batchEmbedContents`, one request per text, `x-goog-api-key`) are spoken natively instead of
    being pushed through a proxy. **Azure OpenAI** puts the deployment in the path, authenticates with `api-key` and
    appends `?api-version=` (`embedding.apiVersion`, default `2024-02-01`).
  - **Batch ceilings are enforced**, so `batch: 500` against Cohere sends 96 per request instead of collecting a 400.
  - **Asymmetric models** get the document-side hint by default (`search_document`, `RETRIEVAL_DOCUMENT`,
    `document`), overridable with `embedding.inputType`; `input_type` is sent only to providers that accept it,
    because strict OpenAI servers reject unknown fields.
  - Every provider's vectors are L2-normalised after decoding, so ranking does not depend on the engine's
    conventions, and an unknown provider name is rejected at load with the known names in the message.
  - `GET /api/embedding` returns the registry (readable without admin; writing still needs it), and the Settings
    form builds its provider list from it, so the UI cannot offer something the loader would reject.
  - Environment overrides gained `BRAINDB_EMBED_INPUT_TYPE` and `BRAINDB_EMBED_API_VERSION`.

- **Pluggable embedding engine.** `embedding.provider: openai` sends text to any OpenAI-compatible `/embeddings`
  endpoint — OpenRouter, OpenAI, Ollama, LM Studio, vLLM, Text Embeddings Inference. The built-in `hash-256` engine
  stays the default, so nothing changes unless configured.
  - Requests are batched (`batch`, default 64), retried on 429/5xx with capped exponential backoff, and cached on disk
    by `sha256(model + text)` in `embeddings.cache` next to the brain. Re-imports and restarts cost no API calls, and
    changing the model invalidates the cache by itself.
  - `fallback: hash` (default) degrades to hash vectors and reports the engine as degraded; `fallback: error` fails
    the operation instead. A missing `apiKeyEnv` variable is an error at load time.
  - Keys are referenced by environment-variable name and never written to `braindb.yaml`, `brain.xml` or the cache.
  - Environment overrides: `BRAINDB_EMBED_PROVIDER`, `BRAINDB_EMBED_MODEL`, `BRAINDB_EMBED_BASE_URL`,
    `BRAINDB_EMBED_KEY_ENV`, `BRAINDB_EMBED_DIM`, `BRAINDB_EMBED_FALLBACK`, `BRAINDB_EMBED_CACHE`.
- **Decay floors.** `Kind.Floor` and the `kinds[].floor` config key; `Brain.SetKindDecay` to override decay from Go,
  and `LegacyDecay()` to restore the 0.0.1 policy.
- `docs/BENCHMARKS.md`, which states plainly that nothing has been measured yet.

- **`examples/braindb-self`: brainDB's own project brain.** The repository documented inside the database it
  documents: 95 hand-written memories (decisions and the ones they reversed, the fifteen spec addenda, the measured
  retrieval table, the CLI surface), five ADRs, a roadmap, an agent playbook, two CSVs imported as `command` and
  `benchmark` records, three fill templates and a 24-query gold set.
  - The gold set is a **replication** of the sales-company measurement on a different domain: graph MRR 0.55
    against vector 0.46 over 24 queries and 158 nodes, graph again never missing a lexical query, and 8 to 9 of 12
    paraphrases returning nothing relevant in any mode.
  - The example also documents the threshold lesson it ran into: 72 hand-written links over 95 memories light 47%
    of the brain at threshold 0.2 and 23% at 0.3, for the same answers.

- **`braindb eval`: retrieval-quality measurement.** Scores a brain against a file of judged queries and reports
  recall@5, recall@10, MRR and nDCG@10 per search mode, broken down by query bucket.
  - A gold set whose relevant ids do not exist in the brain is rejected at load time, because a typo in a gold file
    otherwise shows up as a retrieval failure and quietly lowers every score.
  - `examples/sales-company/eval.json` ships 32 hand-judged queries in two buckets: queries that share words with
    the target memory, and queries that share only meaning.
  - First results are in `docs/BENCHMARKS.md`. Summary: with `hash-256`, graph activation beats flat vector top-k
    overall (MRR 0.57 vs 0.46), and both fail on paraphrases (12–14 of 16 queries return nothing relevant).

### Changed — behaviour

- **Decay no longer deletes knowledge.** The weight curve is now
  `w = base × (floor + (1 − floor) × 0.5^(age / half-life))`. Durable kinds carry a floor, so they rank lower with age
  but never drop out of the graph; only `SUPERSEDE` and `FORGET` remove knowledge.

  | kind | 0.0.1 half-life | 0.0.2 half-life | floor |
  |---|---|---|---|
  | decision | 60 d | 365 d | 0.80 |
  | fact | 30 d | 180 d | 0.70 |
  | record | 30 d | 180 d | 0.70 |
  | file | 14 d | 180 d | 0.60 |
  | chunk | 21 d | 120 d | 0.55 |
  | summary | 30 d | 90 d | 0.40 |
  | idea | 7 d | 60 d | 0.25 |
  | task | 14 d | 14 d | 0 |
  | question | 7 d | 7 d | 0 |
  | note | 2 d | 7 d | 0 |

  Existing brains keep their stored content; weights, and therefore context and ranking, change on the next load.
  `braindb.yaml` can restore the old policy per kind with `{halfLife: 30, floor: 0}`.
- **Purpose admission defaults to `advise`, not `enforce`.** Silently dropping knowledge because a similarity score
  fell below a threshold is an invisible, unrecoverable loss. `enforce` is now opt-in, and the shipped examples use
  `advise`.

### Changed — roadmap

- **SQLite replaced the hand-rolled storage engine, and shipped.** The planned custom write-ahead log, inverted and
  range indexes, query planner, pure-Go HNSW index and from-scratch Redis client are dropped in favour of SQLite,
  which provides all of it and is the one dependency the project takes. Redis is off the roadmap;
  `docker-compose.redis.yml` is removed. What landed is listed under **Added**; what is still on paper is lazy
  loading, text pushdown over the stem index, and the latency numbers.
- Performance figures are labelled as targets everywhere, because none of them has been measured.

### Fixed

- `TestNoEmoji` failed on `docs/index.html`: the navigation glyph `☰` is now an inline SVG.

- **An empty `purpose.outOfScope.entities` list forbade every entity.** `patterns.ExtractTypes` treats an empty
  type list as "every type", so a brain whose purpose did not list forbidden entities rejected any item carrying a
  date, an amount or a version — in `advise` mode too, because a forbidden entity bypasses the advise branch. Every
  shipped example happened to set `entities: [card]`, which hid it until `examples/braindb-self` left the key out.
  An empty list now forbids nothing.
- **Template fill cut tokens that contain a full stop.** The clause splitter used by `FILL` split on every `.`, so
  slots resolved to `A brain is stored today as one brain` and `MRR 0` instead of `brain.xml` and `0.57`. A full
  stop now ends a clause only when whitespace follows it.
- **A token scope the loader rejects now fails validation too.** `server.tokens[].scope` outside read/write/admin
  passed the dry run and then killed the server on its next start, because `configTokens` dies on it. The config
  check rejects it, requires `name` and `tokenEnv` on every entry, and refuses a `tokenEnv` that is not shaped like
  a variable name — a pasted token would otherwise be committed with the file.
- **A trash button in the settings tables deleted the whole list.** `removeYamlKey(y, 'kinds[name=fact]')` ignored the
  selector and removed the `kinds:` key with every entry under it, and the kinds, rules and token tables all call that
  path. A selector as the last path segment now removes that one item and keeps its neighbours; the list key goes only
  when the item removed was the last one, because an empty `kinds:` parses as null and the loader rejects it.
- **A missing field value satisfied every comparison.** `<` matched any node that lacked the field, so a date filter
  returned mostly rows with no date at all. A missing value now satisfies nothing.
- **An unknown column returned a silent NULL** instead of an error. It is decided against the table's schema, so the
  verdict no longer depends on the order of the conditions.
- **PII masking ran the whole pattern library over every rendered result** to find nothing. It is gated behind a
  cheap card pre-check: 7x faster on every query, on both backends, with a test that a card still never slips through.
- **`limit`/`offset` are applied before rows are materialised** when nothing is ordered.
- **Config writes spliced the whole document.** A write now replaces a single key, so comments and formatting survive.
  `PATTERN ADD`/`REMOVE` persist: the hook existed but deadlocked on the store lock when called inside a transaction.
- **`/api/quarantine`, `/api/patterns` and `/api/whoami` send the fields the UI reads.** The review queue crashed the
  whole page on the first quarantined item; the Access tab now reads the columns `SHOW roles`/`grants` return. Each
  tab sits in its own error boundary, so one bad payload cannot blank the app.

### Internal

- `HashEmbed` is unchanged and still the default, so the parity suite against the JavaScript prototype passes bit for
  bit. The parity fixture pins `LegacyDecay()` explicitly, because parity is about the algorithms, not about which
  half-life ships today.
