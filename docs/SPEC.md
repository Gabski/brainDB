# brainDB: specification (shared contract)

> **Naming (authoritative, overrides any other name below):**
> - product name: **brainDB**
> - repo folder: `braindb/` (the lead renames `context-brain/` at the end)
> - Go module: `github.com/Gabski/brainDB`
> - binary / CLI: `braindb` (`cmd/braindb`), e.g. `braindb serve`, `braindb mcp`, `braindb query`, `braindb fill`
> - MCP server name: `braindb`; tool names stay `brain_*`
> - UI title: "brainDB"; Storybook title prefix "brainDB/…"
> - data file default: `brain.xml`
>
> Wherever this document says "Context Brain", `brain` (CLI), `cmd/brain` or `github.com/contextbrain/contextbrain`, read the names above.

Context Brain is a long-term memory for AI agents and people, modeled as a weighted graph:

- **Memories**: typed items (fact, decision, task, question, note, summary, file, chunk, idea, plus custom kinds) grouped into sections.
- **Concepts**: abstract knowledge, nodes with id prefix `A:`. Memories connect to them with weighted edges.
- **Synapses**: weighted edges between nodes.

The whole brain is memory. Only a **lit slice**, the current context, is handed to a model. The slice comes from
spreading activation that starts at the **focus** (seed ids) and runs to a configurable **depth**.

The algorithms were ported from a reference implementation that is not part of this repository. Parity is guaranteed by
the committed golden data in `internal/brain/testdata/` (`parity_oracle.json`).

## Layout

```
context-brain/
  go.mod                    module github.com/contextbrain/contextbrain (Go ≥ 1.23, stdlib only)
  cmd/brain/main.go         CLI: serve | mcp | seed | state | context | ingest
  internal/brain/           model, weights, activation, embedding, search, paths, suggestions, exec ops, ingest, XML/MD I/O, seed loader
  internal/mcp/             MCP stdio server (JSON-RPC 2.0, newline-delimited)
  internal/server/          HTTP API + SSE + embedded UI (//go:embed ui/dist)
  internal/server/ui/dist/  built web UI (Vite output; placeholder index.html committed so `go build` works)
  web/                      React 18 + TypeScript + Vite + three.js UI, Storybook
  examples/sales-company/   seed.json + docs/ (files to ingest)
  examples/research-lab/    seed.json + docs/
  examples/braindb-self/    seed.json + docs/ (brainDB's own project knowledge)
```

## Model

Kinds (`label`, `color`, `base`, `halfLife` in days, id `prefix`, default `section`):

| kind | color | base | halfLife | prefix | section |
|---|---|---|---|---|---|
| fact | #4ea8ff | 0.60 | 30 | f | facts |
| decision | #ffa94d | 0.70 | 60 | d | decisions |
| task | #69db7c | 0.55 | 14 | t | tasks |
| question | #da77f2 | 0.50 | 7 | q | questions |
| note | #adb5bd | 0.40 | 2 | n | notes |
| summary | #38d9a9 | 0.60 | 30 | s | procedures |
| file | #f783ac | 0.50 | 14 | x | files |
| idea | #ffe066 | 0.50 | 7 | i | ideas |
| chunk | #94d82d | 0.45 | 21 | c | files |
| concept | #e7f5ff | 0.80 | ∞ | A: | concepts |

Custom kinds can be added at runtime and via seed.

**Weight.** `w = base × 0.5^(age/halfLife) × (superseded ? 0.3 : 1)`.
- Pinned items do not decay.
- Concepts have a fixed weight (`base`).
- `age` is in days, derived from `touched` (ISO time of the last change or boost).

**Activation.** The rule:

```
seeds: act = 1, depth = 0
level by level, up to the depth:
  a(v) = act(u) · w(u,v) · hop · (0.35 + 0.65 · weight(v))
  superseded v, when showSuperseded is false:
    a ×= 0.15
    and "supersede" edges are skipped
  keep v if a ≥ threshold and a > its current act; record depth and via = u
```

Defaults: depth 2, threshold 0.16, hop 0.85.

**Edge types:** `concept` (memory↔concept, concept↔concept), `assoc`, `similar` (from embeddings), `supersede` (old→new),
`part` (file→chunk), `seq` (chunk→next chunk), `new` (added by user/agent from a suggestion).

**Embedding.**
- `hash-256`, (5-letter stems + bigrams + char trigrams, signed FNV-1a hashing, L2 norm).
- Concept vector = label vector + weighted mean of its members.
- **Enriched** vector (APPNP): `h = (1-α)·x + α·Σ w·h_neighbour / Σ w`, iterated `hops` times. `supersede` edges count ×0.3.
- Defaults: α 0.5, hops 2, use "enriched", hybrid λ 0.5.
- Coherence = mean cosine over non-supersede edges (raw vs enriched).

**Search modes:**

| mode | how it works |
|---|---|
| `hybrid` | up to 4 vector seeds (≥ 50% of the top score), activation from them; graph score of a seed = cos/topCos; score = λ·cos + (1−λ)·graph |
| `vector` | kNN, reports raw cosine, enriched cosine, and rank lift |
| `tree` | beam search: 2 roots, each node expands its 3 best children, each level keeps 6; score = 0.55·cos + 0.45·parentScore·w |
| `graph` | stem overlap, then activation; reports `via` hits |

**Analysis:**
- `testLinks`: propagation from one node plus ablation of each of its edges, with Δlit / Δsum and a `critical` flag.
- `suggest`: Adamic-Adar over shared neighbours + 0.12 × common stems + 1.2 × (cos − 0.2) when cos > 0.2. Excludes chunks of the same file.
- `paths`: the k = 3 strongest paths. Cost is `−log(w · (0.35 + 0.65·weight(v)) · (sup ? 0.3 : 1))`, with alternatives found by banning one edge of the best path at a time.
- `discover`: missing links inside the lit context, plus bridges from the lit context to faded memory.
- `spanTree(root)`: maximum spanning tree.

**Ingest:**
- **Text, markdown, code, CSV:** file node + chunks (`chunkText`, max 650 chars, at most 40 chunks). Edges `part` 0.9 and `seq` 0.5. Then auto-link:
  - ≤ 2 concepts with cos > 0.14, only when the node has no concept edge yet;
  - ≤ 3 similar memories with cos > 0.28, excluding siblings.
- **PDF:** text via `pdftotext` if present.
- **Images:** file node only.
- **ctxc workspace:** a `state.json` with `items[]` (or a directory containing one), or ctxc `<context>` XML, imports as items. Tags link to concepts whose label shares a stem.
- **Focus:** ingest focuses only files with ≤ 8 chunks.

## Brain XML (persistent format, `brain.xml`)

```xml
<brain version="1" step="4">
<goal>…</goal>
<embedding model="hash-256" dim="256" alpha="0.5" hops="2" use="enriched" lambda="0.5"/>
<kinds><kind name="deal" label="deal" color="#…" half-life="21" base="0.6" prefix="g" section="pipeline"/></kinds>
<context focus="d121,d123" goal-focus="d121,d123" depth="2" threshold="0.16" hop="0.85" show-superseded="0"/>
<concepts><concept id="A:pricing" base="0.8">Pricing</concept></concepts>
<memory>
<section name="facts">
  <item id="f101" kind="fact" base="0.6" age="30" touched="2026-10-21T09:00:00Z" pinned="1" superseded-by="…" name="…" mime="…" size="…" parent="x400" idx="0">text</item>
</section>
</memory>
<synapses><link a="f101" b="A:pricing" w="0.8" type="concept"/></synapses>
</brain>
```

Round-trip must be lossless: load → save gives identical output. Weights are rounded to 2 decimals.

## Context outputs

**Context XML**, what the model reads, modelled on ctxc `RenderXML`:

```xml
<context now="2026-11-20 09:30" step="4" tokens="~652" depth="2" threshold="0.16" hop="0.85" order="items sorted by activation">
<goal>…</goal>
<focus refs="d121,d123,n150"/>
<concepts note="concepts lit by the context"><concept id="A:client" act="0.66" depth="1">Client</concept></concepts>
<section name="facts">
  <item id="f103" kind="fact" w="0.38" act="0.43" depth="1" via="d121" age="20d" refs="q141" concepts="idp:0.8 api:0.6">text</item>
</section>
<memory note="not in context: faded or out of depth" total="113" lit="36" superseded-faded="5"/>
</context>
```

**Context Markdown** (`text.md`): Contents:
- header: goal, time, step, tokens, depth, threshold, hop, lit counts;
- Focus;
- Concepts line;
- one heading per section, each item with kind, act, depth ← via, w, pin and file info, then the text, then ◆ concepts and ↔ refs;
- an activation tree in a code block;
- a Statistics table;
- "Outdated versions (superseded, do not use)".

## HTTP API (127.0.0.1 only, JSON unless noted)

| method & path | body / query | response |
|---|---|---|
| GET `/api/state` | | `State` (below) |
| POST `/api/context` | `{focus?, addFocus?, removeFocus?, depth?, threshold?, hop?, showSuperseded?, goal?, goalFocus?:bool}` | `State` |
| POST `/api/embedding` | `{alpha?, hops?, use?: "raw"\|"enriched", lambda?}` | `State` |
| POST `/api/exec` | `{ops: Op[]}` | `{changes: string[], touched: string[], auto: AutoLink[], status: string}`; 400 `{error}` and nothing applied on failure |
| POST `/api/search` | `{query, mode?: "hybrid"\|"vector"\|"tree"\|"graph", k?: 8, focus?: bool}` | `SearchResult` |
| GET `/api/inspect` | `?id=&what=item,links,near,test,suggest,paths&to=` or `?what=discover` | `Inspect` |
| GET `/api/layout` | `?view=embed\|tree&root=` | `{positions: {[id]: [x,y,z]}, treeEdges?: [a,b][], root?}` |
| GET `/api/vector` | `?id=` | `{raw: number[64], enriched: number[64], dim, drift}` |
| POST `/api/ingest` | multipart `files[]`, `focus=true\|false` | `IngestResult` |
| POST `/api/kinds` | `{name, color, halfLife, base?}` | `State` |
| GET `/api/context.xml` `/api/context.md` | | text |
| GET `/api/brain.xml` / PUT `/api/brain.xml` | XML body | text / `State` |
| GET `/api/events` | | SSE: `data: Event` |
| GET `/api/journal` | | `Event[]` (last 100) |
| GET `/` | | embedded UI |

```ts
type Kind = {name: string; label: string; color: string; base: number; halfLife: number | null; prefix: string; section: string; builtin: boolean};
type Node = {id: string; kind: string; section: string; text: string; base: number; weight: number; age: number; pinned: boolean;
  supersededBy?: string; parent?: string; index?: number; file?: {name: string; mime: string; size: number};
  activation?: number; depth?: number; via?: string};            // activation/depth/via only when lit
type Edge = {a: string; b: string; w: number; type: 'concept'|'assoc'|'similar'|'supersede'|'part'|'seq'|'new'};
type Params = {depth: number; threshold: number; hop: number; showSuperseded: boolean};
type Embedding = {model: string; dim: number; alpha: number; hops: number; use: 'raw'|'enriched'; lambda: number; coherenceRaw: number; coherenceEnriched: number};
type Stats = {lit: number; memories: number; concepts: number; tokens: number; reach: number; avgWeight: number; superseded: number; newLinks: number; byDepth: number[]};
type State = {name: string; goal: string; step: number; params: Params; embedding: Embedding; kinds: Kind[]; sections: string[];
  focus: string[]; goalFocus: string[]; nodes: Node[]; edges: Edge[]; stats: Stats; status: string};
type SearchRow = {id: string; score: number; raw?: number; enriched?: number; lift?: number; vector?: number; graph?: number; depth?: number; via?: string; parent?: string | null};
type SearchResult = {mode: string; lambda?: number; rows: SearchRow[]; via?: SearchRow[]; roots?: string[]};
type Suggestion = {u: string; v: string; score: number; via: string[]; commonStems: number; cos: number};
type Inspect = {
  item?: {node: Node; decay: number; chain: string[]; xml: string};
  links?: {to: string; w: number; type: string}[];
  near?: {id: string; cos: number; linked: boolean}[];
  test?: {reached: {id: string; act: number; depth: number}[]; ablation: {to: string; w: number; type: string; dLit: number; dSum: number; critical: boolean}[]};
  suggest?: Suggestion[];
  paths?: {path: string[]; weights: number[]; strength: number}[];
  discover?: {inner: Suggestion[]; bridge: Suggestion[]}};
type AutoLink = {a: string; b: string; cos: number; type: 'concept'|'similar'};
type IngestResult = {report: string[]; fresh: string[]; links: AutoLink[]; status: string};
type Event = {type: 'changed'|'hello'; ts: number; by: string; tool: string; summary: string; ids: string[]};
```

**Exec ops**, an atomic batch: on error nothing changes and the message names the failing op (`op#2 link: no such item x`).

| op | fields |
|---|---|
| `remember` | `kind, text, section?, concepts?: string[]` (label or id; created if new), `links?: {to, w?}[], supersedes?, pin?, focus?, tmp?, autolink?: bool` |
| `concept` | `label, members?: string[], related?: string[], tmp?` |
| `link` | `a, b, w?, type?` |
| `unlink` | `a, b` |
| `boost` / `weaken` | `id` |
| `pin` / `unpin` | `id` |
| `supersede` | `old, new` |
| `edit` | `id, text?, section?, kind?` |
| `forget` | `id, force?`; cascades to chunks |
| `focus` | `ids? \| add? \| remove?` |
| `goal` | `text, focus?` |

`@name` references a `tmp` created earlier in the same batch.

Every mutation (API, MCP, UI) saves `brain.xml` atomically, appends to `brain.journal.jsonl` and broadcasts an `Event` over SSE.
`by` is `ui`, the MCP actor name (e.g. `claude`), or `file`. If another process changes `brain.xml` (mtime), reload it.

## MCP (stdio) tools

Five tools, with English descriptions and output:

| tool | what it does |
|---|---|
| `brain_view` | args `focus` (ids or "goal"), `add_focus`, `remove_focus`, `depth`, `threshold`, `hop`, `show_superseded`, `format` (`xml`\|`md`) |
| `brain_search` | `query, mode, k, focus` |
| `brain_inspect` | `id, what[], to`, or what=[discover] |
| `brain_exec` | `ops[]` |
| `brain_ingest` | `path, focus, max_files` |

Every result ends with a status line:

```
[ok] step 5 · lit 16/98 memories + 6 concepts · ~489 tok · depth 1/1 thr 0.16 · focus d121,f400
```

Over 4000 tokens it adds a warning. `initialize` returns `instructions`: the working loop view → search → inspect → exec → ingest.

`brain mcp` also starts the HTTP UI on `--port` (default 7791) unless `--no-http`. If the port is busy, it only syncs through the file.

## CLI

```
brain serve   --brain brain.xml [--seed examples/x/seed.json] [--port 7791]   # HTTP + UI; --seed creates brain.xml if missing
brain mcp     --brain brain.xml [--actor claude] [--port 7791] [--no-http]
brain seed    examples/x/seed.json -o brain.xml [--ingest examples/x/docs]
brain state   --brain brain.xml            # prints State JSON (used to make UI fixtures)
brain context --brain brain.xml [--format md|xml] [-o text.md]
brain ingest  --brain brain.xml PATH...
```

## Seed format (`examples/*/seed.json`)

```json
{
  "name": "Northwind Sales",
  "goal": "…",
  "sections": ["facts", "decisions", "tasks", "questions", "notes", "procedures", "ideas", "files"],
  "kinds": [{"name": "deal", "label": "deal", "color": "#ff8787", "halfLife": 21, "base": 0.6, "prefix": "g", "section": "pipeline"}],
  "concepts": [{"id": "pricing", "label": "Pricing"}],
  "conceptLinks": [["pricing", "margin", 0.8]],
  "memories": [{"id": "f101", "kind": "fact", "section": "facts", "text": "…", "concepts": ["pricing", "clients"],
                "ageDays": 30, "pinned": true, "base": 0.85, "supersededBy": "d121"}],
  "links": [["d121", "f103", 0.85]],
  "archive": {"pricing": ["older memory text", "…"]},
  "focus": ["d121", "d123"],
  "context": {"depth": 2, "threshold": 0.16, "hop": 0.85}
}
```

How the seed loads:
- concept ids get the `A:` prefix;
- a memory's first concept gets weight 0.8, the rest 0.6, with deterministic jitter ±0.1;
- `supersededBy` adds a `supersede` edge with weight 0.5;
- archive entries become older faded memories, generated deterministically: deterministic `mulberry32(7)` choices of kind and age 25–305 days, a possible second concept, and random `assoc` edges between memories that share a concept.

## UI (web/)

**Stack:** React 18 + TypeScript + Vite + three.js, no UI kit. Dark theme by default plus a light theme, using CSS variables.
Everything is in English.

**Layout:** a header with view tabs and search, a left panel, a 3D canvas in the middle, a right panel with tabs. It must match
every feature of `the reference UI (not in the repository)`, but all logic goes through the API. The UI computes only layouts.

**Views:**

| view | layout |
|---|---|
| Brain | point-cloud hemispheres, sections as lobes, concepts in the core |
| Layers | concept / decision / fact / task / note levels |
| Depth sphere | shells = activation depth |
| Decay | x = age (log), y = weight |
| Vectors | PCA positions from `/api/layout?view=embed` |
| Tree | positions from `/api/layout?view=tree` |

Lit nodes glow. Faded and superseded nodes are dim. Path and tree edges show pulses. Labels sit on the top lit nodes.

**Left panel:**
- goal; focus chips; sliders for depth, threshold and hop; superseded toggle; edges mode; colour by kind or depth; auto-rotate;
- stat cards; charts: reach by depth, weight × age scatter, top lit concepts;
- file drop zone; embedding controls;
- add-memory form; new-kind form; Discover button.

**Right tabs:**

| tab | contents |
|---|---|
| Memory | details, weight breakdown, path from focus, actions (focus, add to focus, test links, suggest links, path to…, link to…, tree root, find similar, boost, weaken, supersede by…, pin), links list, tool output, vector bars raw/enriched, nearest |
| Discoveries | search results per mode (snippet around the match, query highlighted, caret expands the whole memory), a narrowing bar (free text + kind/section/source facet chips with counts, grouping by kind/section/source), hit-count selector, recent searches, ingest report, discover inner/bridges with add-link buttons |
| Context | XML / Markdown toggle, copy, download `text.md` |
| Brain XML | edit, apply, download |
| Agent | live SSE feed; agent-touched ids glow cyan |

**Storybook** (`@storybook/react-vite`): every presentational component has stories driven by fixtures
`web/src/fixtures/*.json` (State/Search/Inspect from the examples), including a Brain3D story with each view.
The API client is a module that can be swapped for a mock in stories.

---

# Addendum A: Context Brain as a database (query + templates)

The service is also a new kind of database. Three query types run against the same brain:
1. **Context query**: an ad-hoc lit slice. It does not change the shared focus.
2. **Raw data query**: filter, sort and project memories and records, like a table.
3. **Template fill**: the caller gives a template whose slots describe the data wanted, for example `name="client name"`. The brain fills it from memory and renders JSON, XML, YAML or Markdown.

## A1. Records (structured raw data)

**CSV ingest** creates one **record** per row (max 500 rows) instead of text chunks.
- Built-in kind `record`: color #91a7ff, base 0.5, halfLife 30, prefix `r`, section = the file name stem, e.g. `pipeline`.
- Fields: `fields map[string]string` (header → cell).
- Text: `"col: val · col: val …"`.
- Edges: `part` 0.9 from the file node, `seq` 0.5 to the next record.
- Records are embedded and auto-linked like any memory.

Any memory may carry `fields`, set through exec `remember {fields: {...}}` / `edit {fields}`.

| format | how fields appear |
|---|---|
| Brain XML | attribute `data` holding a JSON object, e.g. `data="{&quot;stage&quot;:&quot;negotiation&quot;}"`; round-trip stays lossless |
| State `Node` | adds `fields?: Record<string,string>` |

## A2. Raw data query

The query is a JSON object, or a compact string that parses into the same object.

```json
{"from": "memory",
 "where": {"kind": ["deal"], "section": "pipeline", "concept": "renewal", "text": "discount",
           "lit": true, "pinned": false, "superseded": false, "minWeight": 0.2, "maxAgeDays": 30,
           "fields": [{"name": "stage", "op": "=", "value": "negotiation"}, {"name": "value", "op": ">", "value": "10000"}]},
 "near": "renewal risk",
 "orderBy": "-score",
 "limit": 20, "offset": 0,
 "select": ["id", "kind", "text", "weight", "activation", "fields.stage", "fields.value"]}
```

| part | meaning |
|---|---|
| `from` | `memory` (default; all non-concept nodes), `concepts`, `links` (edges: a, b, w, type) |
| `near` | adds `score` = hybrid score against that text; without it, score = weight |
| `orderBy` | any selectable field; `-` prefix = descending |
| `fields` ops | `= != > >= < <= ~ (contains, case-insensitive)`; numeric comparison when both sides parse as numbers (strip currency symbols, `,` thousands separators, `%`) |

**Compact form**: whitespace-separated terms, quotes allowed.

```
kind:deal,account section:pipeline concept:renewal lit:true pinned:false superseded:false
w>=0.2 age<=30 stage=negotiation value>10000 owner~kim "free text" near:"renewal risk"
sort:-value limit:10 select:id,text,fields.value
```

Unknown `key<op>value` terms are field filters. A bare word or quoted string is a text filter.

**Result**: `{columns: string[], rows: object[], count: number, total: number, rendered: string}`.
`format` is `json | yaml | xml | md | csv`:

| format | rendering |
|---|---|
| md | table |
| xml | `<rows><row id="…" …>` |
| csv | RFC 4180 |
| yaml | list of maps |

## A3. Context query

`{focus?: ids, query?: text, depth?, threshold?, hop?, format: xml|md|json|yaml}`
- With `query` and no focus, the seeds are the hybrid-search vector seeds.
- Returns the lit slice in the requested format. JSON/YAML is the same structure as the context XML: goal, focus, concepts, sections → items with act, depth, via, refs, concepts, fields.
- Shared state is not mutated.

## A4. Template fill

**Two template styles.** The style is detected from the content, or set with `templateFormat: json|yaml|xml|md`.

**1. Schema templates (JSON / YAML / XML).** Every string leaf is a slot: `"description[:type][|option…]"`.

```yaml
client:
  name: "client name"
  owner: "account owner:name"
  renewal_date: "contract renewal date:date"
  arr: "annual contract value:money"
deals[]: "open deals|kind=deal|limit=5"     # list slot: one item per matching memory
deals[].title: ".text"                      # item fields: .id .kind .text .weight .fields.<name>, or a description searched near the item
deals[].stage: ".fields.stage"
deals[].risk: "main risk for this deal"
summary: "current account status:text"
```

- JSON uses the same keys (`"deals[]"`, `"deals[].title"`), or an array holding one object as the item template: `"deals": [{"title": ".text", "risk": "main risk"}]` with `"deals": "open deals|kind=deal"`… Pick one rule, document it, support both if simple.
- XML: attribute values and text content are slots. `<deal repeat="open deals|kind=deal|limit=5" title=".text" stage=".fields.stage">main risk for this deal</deal>`. `repeat` marks a list element.

**2. Text templates (Markdown, or any text).** Placeholders:

```
{{ client name }}
{{ deal value:money }}
{{#each open deals|kind=deal|limit=5}} - {{ .text }} ({{ .fields.stage }}): {{ main risk for this deal }} {{/each}}
```

**Slot types:** `text` (default), `name`, `number`, `money`, `percent`, `date`, `email`, `phone`, `url`, `bool`, `list` (comma list), `id`.

**Options:**

| option | effect |
|---|---|
| `kind=a,b` | restrict to these kinds |
| `section=…` | restrict to this section |
| `concept=…` | restrict to this concept |
| `scope=context\|memory` | lit slice only, or all memory |
| `limit=N` | number of items in a list slot |
| `min=0.25` | minimum score |
| `field=name` | take the value from `fields[name]` of the best match |

**Fill algorithm.** Deterministic, no LLM.
1. Candidates:
   - top-k via hybrid search on the description, plus the parent path for context, e.g. `client > owner`;
   - inside a list item, restricted to the item's node and its neighbours within 2 hops;
   - if the description mentions a kind name or its plural, filter by that kind;
   - if the description contains a field name (exact or stem), it reads the value from `fields[name]` of the best record;
   - default scope: `memory`, with lit nodes boosted ×1.15.
2. Value:
   - typed slots: regex extraction over candidate texts, taking the first match in the best-scoring candidate. Money keeps its currency symbol; date is ISO when parseable;
   - `name`: a capitalised multi-word sequence;
   - `text`: the clause or sentence of the best candidate most similar to the description (split on `. ; · —`).
3. Every slot reports `{path, description, type, value, source, score, snippet}`.
   - Below `min` (default 0.2) the value is `null` and the path goes to `missing`.
   - `withSources: true` also adds `_sources` (JSON/YAML), `source="id"` attributes (XML) or footnotes (MD).

**Request**: `POST /api/fill {template, templateFormat?: "auto", output?: "same"|"json"|"yaml"|"xml"|"md", scope?, withSources?, min?}`.
**Response**: `{output: string, format: string, values: SlotValue[], missing: string[]}`.
A schema template can be rendered into any of the 4 formats. A text template renders as text.

## A5. Interfaces

**HTTP:**
- `POST /api/query` with `{query: object|string, format?}`;
- `POST /api/context/query` (A3);
- `POST /api/fill` (A4);
- `GET /api/templates` lists templates found in `<brain dir>/templates/*` and `examples/*/templates/*`, as `{name, path, format, content}`.

**MCP:** two more tools.
- `brain_query`: `{target: "memory"|"context", query (string or object), format}`.
- `brain_fill`: `{template, template_format, output, scope, with_sources, min}`.

The fill output lists `missing` slots so the agent can complete them, then store results with `brain_exec`. Tool descriptions are in English.

**CLI:**
```
brain query --brain b.xml 'kind:deal stage=negotiation sort:-value' --format yaml
brain query --brain b.xml --context --query "renewal risk" --format md
brain fill --brain b.xml templates/account-brief.yaml --output json [--sources]
```

**YAML:** a stdlib-only subset parser and emitter with block maps, block lists, plain/single/double-quoted scalars, `#` comments, and `|` block scalars for output.

**UI:** new right-panel tab **Query** with three modes:
- **Raw**: compact/JSON editor, format select, result table plus rendered output, row click selects the node;
- **Context**: query text, depth and format;
- **Template**: template editor with a picker from `/api/templates`, output format, filled preview, and an evidence table. Each slot shows value, source (clickable), score and snippet; missing slots are highlighted. Copy and download.

Storybook stories cover all of it with fixtures.

**Examples:**
- each example gets `templates/` with 3 templates covering yaml, json, xml and md;
- sales-company: e.g. `account-brief.yaml`, `quote.xml`, `deal-review.md`;
- research-lab: e.g. `experiment-report.md`, `sample-sheet.json`, `weekly-status.yaml`;
- the CSV in `docs/` must have columns that templates and queries use (`stage`, `value`, `owner`, `close_date` / `sample_id`, `temperature_c`, `conductivity_mS_cm`, `run_date`);
- the README lists example raw queries and fill commands.

---

# Addendum B: publish a brain as a compiled MCP binary

`braindb publish` turns one brain into a single executable. The executable is an MCP server for that brain and starts instantly.

```
braindb publish --brain brain.xml -o dist/acme-sales-brain \
  [--name acme-sales] [--version 1.2.0] \
  [--mode readonly|overlay|writable] \
  [--tools view,search,inspect,query,fill] \
  [--target linux/amd64,linux/arm64,darwin/arm64,windows/amd64] \
  [--with-ui] [--templates dir]
```

## How it works (no Go toolchain needed at publish time)

1. **Snapshot.** `braindb` compiles the brain into a binary snapshot with `encoding/gob` + gzip. The snapshot holds:
   - nodes, edges and kinds;
   - precomputed raw and enriched vectors, stem sets and adjacency;
   - templates and the manifest.

   Loading it skips XML parsing and embedding, so startup should be well under 50 ms for ~10k nodes.
2. **Payload appended to the executable.** `braindb` copies an executable and appends `payload | manifest JSON | 16-byte footer`. The footer is the magic `BRAINDB1` plus a uint64 payload length.
   - The executable copied is either its own (`os.Executable()`) or a prebuilt one for another platform, taken from `--runtime-dir` (from `make release`: `dist/runtime/braindb-<os>-<arch>[.exe]`).
   - At startup the binary checks its own file for the footer. If one is present, it runs in **published mode**.
3. **Published mode defaults.**
   - No arguments means `mcp` over stdio. The client config is just `{"command": "/path/acme-sales-brain"}`.
   - `serve` (only with `--with-ui`), `context`, `query` and `fill` still work as subcommands.
   - `--version` prints the manifest: name, version, brain step, node count, sha256 of the payload, build time.
   - MCP `serverInfo.name` = the manifest name, so several published brains can run side by side.
4. **Modes.**

| mode | behaviour |
|---|---|
| `readonly` (default) | `brain_exec`, `brain_ingest` and focus changes are not exposed. `brain_view` accepts focus/depth per call without persisting. |
| `overlay` | Writes go to an overlay file in `$XDG_DATA_HOME/braindb/<name>/overlay.xml` (or `--data`). The overlay holds new and changed nodes and edges plus tombstones, merged on top of the embedded snapshot at load. `braindb export --merge` produces a full brain.xml. |
| `writable` | At first run the binary extracts the snapshot to the data dir, then behaves like normal `braindb mcp --brain <that file>`. |

5. **`--tools`** limits the exposed MCP tools. The MCP instructions are generated to match.
6. **Integrity.** The payload sha256 is checked at start. Optionally `--sign key` adds an ed25519 signature, verified with `--pubkey` / an embedded public key.

## Also

- `braindb snapshot --brain brain.xml -o brain.snap`: the snapshot alone. `braindb mcp --snapshot brain.snap` gives fast startup without publishing.
- `make release` cross-compiles the runtime binaries (`CGO_ENABLED=0`, `-trimpath -ldflags "-s -w"`) for linux/darwin/windows × amd64/arm64.
- Tests:
  - publish → run the produced binary → MCP handshake → `brain_search` returns the same rows as the source brain;
  - readonly mode rejects writes;
  - overlay persists across restarts;
  - a corrupted payload fails the checksum.

---

# Addendum C: external sources and federation (linking other brains)

A brain can **mount** external sources as read-only namespaces and link its own memories to theirs.
Context, search, query and fill can then reach across brains.

## C1. Sources

| scheme | example | transport |
|---|---|---|
| `http(s)://` | `http://lab:7791` | another brainDB server (HTTP API); auth token from env var |
| `file://` | `file:///data/lab/brain.xml` | another brain file, loaded read-only and reloaded on mtime change |
| `snap://` | `snap:///opt/brains/lab.snap` | a snapshot (Addendum B) |
| `exec://` | `exec:///opt/brains/acme-sales-brain` | a published brain binary or `braindb mcp`, spoken to over MCP stdio |
| `url+http(s)://` | `url+https://example.org/spec.html` | a plain web page or document: fetched, chunked and embedded as a cached file node; refreshed after `ttl` |

Stored in brain.xml:

```xml
<sources>
  <source name="lab" uri="http://lab:7791" token-env="LAB_TOKEN" weight="0.7" ttl="300" mode="live" federate-depth="1" enabled="1"/>
  <source name="spec" uri="url+https://example.org/spec.html" weight="0.5" ttl="86400" mode="cached"/>
</sources>
```

- Tokens are **never** stored in XML, only the name of the env var that holds them.
- External ids are `name:id`, e.g. `lab:f101`. Local ids never contain `:`, except concepts, which use `A:`.
  A source name must not be `A`.

## C2. External links

`link {a: "d121", b: "lab:x204", w: 0.7}` creates an edge of type `external` to a remote node.
- The remote node is cached as a **stub**: kind, text, weight and fetchedAt. It is stored in `<externals>`
  (`<ref id="lab:x204" kind="fact" w="0.42" fetched="…">text</ref>`) so the brain still renders offline.
- Stubs show in State as nodes with `source: "lab"` and `external: true`.

## C3. Federated operations

**Activation.** When activation reaches an external ref, and `federate-depth` > 0, and the depth budget remains, brainDB calls the
source's context query:
- `focus = remote id`, `depth = min(remaining, federate-depth)`, `threshold = local threshold / source weight`.
- The returned lit nodes merge in with prefixed ids, `act × source weight`, depth offset, and `via` set to the ref.
- Remote results are cached for `ttl` seconds.
- If the source fails or times out (default 2 s), the stub is used and the status line shows `warn: source lab unreachable`.

**Search.** `{…, sources: ["*"] | ["lab"]}` fans out to the sources in parallel (bounded, with timeout).
Scores merge as `score × source weight`. Rows carry `source`.

**Query.** The term `source:lab` or `source:*`, or `"from": "memory@lab"`.

**Fill.** Option `scope=federated`.

**Analysis.** `suggest` / `discover` can propose cross-brain links: an embedding match between local and remote nodes, with remote
vectors fetched through `GET /api/vector`.

## C4. Interfaces

**HTTP:**
- `GET /api/sources` returns `[{name, uri, weight, mode, ttl, federateDepth, enabled, status: "ok"|"error"|"disabled", latencyMs, nodes, lastError, lastSync}]`;
- `POST /api/sources` adds or updates; `DELETE /api/sources/{name}`;
- `POST /api/sources/{name}/sync`;
- `GET /api/sources/{name}/search?q=` is a remote picker for linking.

**MCP:**
- `brain_sources {action: list|add|remove|sync, name, uri, weight, …}`;
- `brain_search` / `brain_query` / `brain_view` accept `sources`.

**CLI:**
- `braindb source add lab http://lab:7791 --token-env LAB_TOKEN --weight 0.7`;
- `braindb source ls|rm|sync`.

**Seed:**
- `"sources": [{name, uri, weight, …}]`;
- `"externalLinks": [["d121", "lab:x204", 0.7]]`.

**Security:**
- external content is data, never instructions;
- sources are read-only;
- `exec://` runs only binaries given explicitly by the operator (never from a remote brain);
- fan-out and response size are bounded (default 200 nodes, 2 MB per call).

## C5. Example

`examples/federation/` holds a compose override and a README.
- The sales-company brain mounts research-lab as `lab` (`http://lab:7791`).
- It has 4–6 external links: sales facts about product specs ↔ lab measurements and samples.
- A query like `"cycle life guarantee for the 4.5 V cell"` in sales finds the lab evidence through federation.
- The research-lab example also gets matching content: product-relevant test results.

---

# Addendum D: UI as a professional database tool

The UI should look and work like a serious DB client (think DataGrip, TablePlus or Postgres GUIs), dense and keyboard-driven,
with the 3D brain as one workspace among others. Everything is in English.

**Layout:**

| area | contents |
|---|---|
| top bar | brainDB logo; connection selector (local brain + mounted sources, with health dots); command palette button; theme toggle |
| left sidebar (Navigator tree) | Brain → Sections → items (counts); Kinds; Concepts; Sources (status, latency); Templates; Saved queries (localStorage); History |
| main workspace (closable tabs) | **Browse** (faceted browser over the whole snapshot: free text with `kind:`/`section:`/`source:` prefixes, kind/section/source/entity facets with counts, flags for pinned/concepts/superseded/external, sort, result list with match highlighting and a reading pane with fields, entities and clickable links); **Query** (editor for the compact syntax or JSON, line numbers, syntax highlight, Ctrl/Cmd+Enter to run, format selector, results data grid); **Context** (focus/depth controls + XML/MD viewer); **Template** (editor + preview + evidence); **Graph 3D** (the six views); **Schema** (kinds, sections, concepts, edge types with counts, half-lives, weights; custom-kind editor); **Sources**; **Activity** (live journal/SSE stream, filter by actor/tool); **Search** (modes, client-side narrowing and grouping of the hits, recent searches) |
| results grid | sortable and resizable columns, sticky header, row selection, copy cell/row, export (json/yaml/xml/md/csv), row count + elapsed ms |
| right Inspector | back / forward through visited nodes (Alt+Left / Alt+Right) and copy id; collapsible sections whose open/closed state survives a reload; links bucketed per edge type with chips to narrow them; the selected node: properties table (id, kind, section, weight breakdown, activation, depth, via, source, fields); links; nearest; actions (focus, test links, suggest, path to…, link to…, link external…, boost/weaken/pin/supersede/forget) |
| bottom status bar | connection, step, lit/total, tokens, depth/threshold, focus ids, last op + latency, SSE live indicator |

**Command palette (Ctrl/Cmd+K):** run query, open tab, jump to node by id or text, set focus, switch view, add source.

**Visual style:**
- Neutral palette with CSS variables, dark and light; colour is used only for the kind badges.
- Monospace (system mono stack) for ids, queries and code; system UI font elsewhere.
- 13px base, compact 28px rows, subtle 1px borders, no gradients except the 3D canvas, focus rings, full keyboard navigation.

**Storybook:** a story for every component, plus full-page stories (Workspace with Query tab, Graph tab, Schema tab) driven by fixtures.

---

# Addendum E: creating databases from bulk imports, and brain configuration

## E1. Multiple databases

A server hosts many databases, like a DB server.
- `--data-dir DIR` (env `BRAINDB_DATA_DIR`, default `./data`): each subdirectory is one database.
- Layout of one database:

```
data/<db>/
  braindb.yaml        # configuration (E3)
  brain.xml           # the brain
  brain.journal.jsonl
  templates/          # fill templates
  imports/            # optional copies of imported raw files (config: import.keepRaw)
```

- `--brain FILE` still works and serves a single database named after the file stem (backward compatible).
- Every API endpoint accepts `?db=<name>` or header `X-BrainDB-Database: <name>`. Without them it uses the default
  database (`--db`, or the first one).
- SSE events carry `db`.

| interface | operations |
|---|---|
| HTTP | `GET /api/dbs` → `[{name, nodes, edges, step, sizeBytes, updated, goal}]`; `POST /api/dbs {name, config?: yaml}` creates an empty db; `DELETE /api/dbs/{name}?confirm=<name>` |
| MCP | the server is bound to one database per process (`--db`); tool `brain_dbs {action: list}` is read-only |
| CLI | `braindb db ls|create|rm` |

## E2. Bulk import ("create a database from a pile of files")

```
braindb create crm --from ./export/ --from ./dump.sql --config braindb.yaml [--dry-run] [--data-dir data]
braindb import --db crm ./more-files/ "./notes/**/*.md" [--dry-run] [--update]
```

Over HTTP: `POST /api/import?db=crm` with multipart `files[]` (directory upload keeps relative paths) plus optional fields
`config` (yaml) and `dryRun`.

The response is an `ImportReport`:
- per file: `{path, importer, status: created|updated|skipped|error, nodes, edges, records, message}`;
- totals;
- detected mapping, as a YAML snippet the user can paste into the config.

Import is **incremental**:
- every imported node stores `src` (relative path + locator, e.g. `dump.sql#customers/42`) and `hash`;
- re-importing skips unchanged items;
- `--update` replaces changed items: the old version is superseded, not deleted;
- import never deletes.

Importers are picked by extension or sniffing, and can be overridden in config:

| importer | input | result |
|---|---|---|
| markdown | `.md .markdown .mdx` | file node + chunks split by headings. YAML front-matter → `fields` (`tags` → concepts, `kind`, `section`, `title`). Headings (h1/h2) → concepts (configurable). `[[wiki links]]` and relative `[text](other.md)` links → `assoc` edges between files once all are imported. Code blocks stay inside chunks. |
| text / code / html / xml / pdf / docx | text extraction | file + chunks. HTML: strip tags, keep headings. DOCX: unzip `word/document.xml`, stdlib. PDF: `pdftotext` if present. |
| csv / tsv | header row | one `record` per row (Addendum A1), section = file stem |
| json / jsonl / ndjson | arrays of objects | one record per object; nested objects flattened to `a.b` field names (depth ≤ 3); arrays of scalars joined with `, `. Config `json.path` picks the array (`$.data.items`). ctxc `state.json` is detected and imported as items. A brainDB seed.json is detected and loaded as a seed. |
| yaml | | same as JSON |
| sql | `.sql` dumps (PostgreSQL / MySQL / SQLite dialects; stdlib tokenizer, no DB driver) | each `CREATE TABLE` → a table: kind `record` (or a mapped kind), section = table name. `INSERT INTO … VALUES (…), (…)` (incl. multi-row) and PostgreSQL `COPY … FROM stdin` blocks → records with `fields`. PRIMARY KEY → record locator. `FOREIGN KEY … REFERENCES` (inline or table constraint) → edges between the records (`assoc`, w 0.7, configurable). Low-cardinality text columns (≤ 30 distinct values, ≥ 2 rows each; configurable) → concepts. Comments and unknown statements are skipped. |
| sqlite | `.sqlite .db` files | only if the `sqlite3` CLI is present: `sqlite3 -json` per table, then as JSON; otherwise reported as skipped |
| images | | file node with metadata |

Records get text from a template, configurable per table or file: default `"<col>: <val> · …"` (first 8 columns).
They are embedded and auto-linked like other memories.

Limits are configurable:

| limit | default |
|---|---|
| file size | 50 MB |
| records per file | 100k |
| chunks per file | 200 |
| total nodes per import | 500k |

Embedding and linking run after parsing, in batches.

## E3. Brain configuration: `braindb.yaml`

Precedence: CLI flags > env > `braindb.yaml` > built-in defaults. `braindb config show|validate [--db x]` prints the effective
config (secrets masked) or errors with line numbers. `GET/PUT /api/config?db=` returns or validates-and-saves the YAML text and returns
`{ok, errors[], effective}`.

```yaml
name: crm
description: Customer knowledge base
goal: Prepare Q4 renewals
server: {addr: 127.0.0.1:7791, tokenEnv: BRAINDB_TOKEN}
context: {depth: 2, threshold: 0.16, hop: 0.85, showSuperseded: false, focus: [d121]}
embedding: {model: hash-256, alpha: 0.5, hops: 2, use: enriched, lambda: 0.5}
budget: {warnTokens: 4000}
kinds:                                  # add or override kinds
  - {name: customer, label: customer, color: "#ff8787", halfLife: 90, base: 0.7, prefix: u, section: customers}
  - {name: fact, halfLife: 45}          # override a built-in half-life
sections: [facts, decisions, tasks, customers, orders]
concepts:                               # predefined concepts and relations
  - {id: churn, label: Churn risk, related: [renewal, support]}
import:
  keepRaw: false
  chunkSize: 650
  autolink: {conceptMin: 0.14, similarMin: 0.28, maxSimilar: 3}
  rules:                                # first match wins; glob on relative path
    - match: "notes/**/*.md"
      importer: markdown
      kind: note
      section: notes
      concepts: [meetings]
      headingsAsConcepts: true
    - match: "*.sql"
      importer: sql
      tables:
        customers: {kind: customer, text: "{name} ({segment}) · owner {owner}", concepts: [segment], key: id}
        orders:    {kind: record, section: orders, text: "Order {id}: {total} {currency} on {created_at}", foreignKeys: true}
        audit_log: {skip: true}
    - match: "exports/*.jsonl"
      importer: json
      kind: record
      section: tickets
      json: {path: "$"}
      text: "{subject}: {status}"
      concepts: [priority, product]
sources:                                # federation (Addendum C)
  - {name: lab, uri: "http://lab:7791", tokenEnv: LAB_TOKEN, weight: 0.7, federateDepth: 1}
templates: {dir: templates}
mcp: {actor: agent, tools: [view, search, inspect, query, fill, exec, ingest, sources]}
publish: {mode: readonly, tools: [view, search, query, fill]}
```

- Text templates `{column}` refer to record fields.
- Unknown keys produce a validation error with a "did you mean" hint.
- The YAML parser is the stdlib subset from Addendum A, extended with flow maps/lists (`{a: 1}`, `[a, b]`) and quoted strings.
  These are required for this config.

## E4. UI

**"New database" wizard**, from the connection selector → New…:
1. name and description;
2. drop files or folders (directory upload, many files), or pick an example;
3. dry-run preview: files grouped by importer, detected tables, columns and FKs, record counts, proposed concepts, plus the generated `braindb.yaml` snippet;
4. edit the config in a YAML editor with live validation from `PUT /api/config` dry-run;
5. create, with a progress bar through the SSE events `import.progress {db, done, total, file}`, then open the new database.

**Import tab** on an existing database: the same flow without creating.

**Settings tab:** `braindb.yaml` editor, validation errors inline, effective config viewer, save.

The connection selector lists databases (`/api/dbs`) and mounted sources.
Storybook: wizard steps, ImportReport (success, partial errors), the config editor with errors.

## E5. Tests

- markdown front-matter, headings and wiki links;
- CSV, JSON (nested, path), JSONL;
- SQL dump (Postgres sample with COPY, MySQL sample with multi-row INSERT and backticks, FKs → edges);
- incremental re-import (skip unchanged; `--update` supersedes);
- config precedence and validation errors with line numbers;
- multi-db API isolation (`?db=`);
- `braindb create` end to end on `examples/import-demo/`.

## E6. Example `examples/import-demo/`

A raw pile of files plus a `braindb.yaml`, showing `braindb create demo --from examples/import-demo/raw --config examples/import-demo/braindb.yaml`. Contents of `raw/`:
- `notes/*.md`: 5–8 notes with front-matter and wiki links;
- `crm.sql`: PostgreSQL dump with customers, orders and contacts, FKs, COPY + INSERT;
- `tickets.jsonl`: ~30 support tickets;
- `products.json`: nested;
- `pricing.csv`.

Its README lists the expected import report and queries that join across sources, e.g. tickets of customers with open orders.
Each existing example also gets its own `braindb.yaml`.

---

# Addendum F: performance at scale, storage and embedding providers

Goal: databases with **10^5–10^6 nodes** (bulk imports, Addendum E) stay fast, and several brainDB processes (HTTP servers,
MCP processes, containers) can work on the same database at once.

## F1. Storage engine interface

`internal/store` defines a `Store` interface:

```
Load(db) (*Brain, error)        LoadLazy(db) (Reader, error)
Apply(db, Mutation) error       Snapshot(db) error
Subscribe(db) (<-chan Event)    Lock(db, ttl) (Unlock, error)
```

Two implementations:

| backend | behaviour |
|---|---|
| `file` (default, as today) | The whole brain is held in memory and written to `brain.xml` atomically (temp file + rename). Readable, diff-friendly, crash-safe, and adequate into the tens of thousands of nodes. Every change is also appended to `brain.journal.jsonl`. |
| `sqlite` | One SQLite file per database, in WAL mode. SQLite provides durability, indexes, the query planner, concurrent readers and crash recovery. `brain.xml` remains available at any time through `braindb export`. |

### Why SQLite, and not a hand-written engine

Earlier revisions of this document specified a custom write-ahead log, inverted and range indexes, a query planner, a
pure-Go HNSW index and a from-scratch RESP client for Redis, all under a stdlib-only rule. That is a multi-year effort
to rebuild what an embedded database already does, concentrated in the one layer where a bug silently destroys the
user's data. It is the wrong place to spend this project's time.

SQLite is therefore the backend, shipped in 0.0.2, and the one accepted dependency. The rest of brainDB stays stdlib-only. Redis
is dropped from the roadmap: the multi-process sharing it was meant to provide is covered by SQLite's WAL mode and its
reader/writer locking, and a shared-nothing deployment can still federate (Addendum C).

## F2. SQLite schema

One file per database, `brain.db`, `PRAGMA journal_mode=WAL`, `synchronous=NORMAL`, `foreign_keys=ON`.

| table | columns | purpose |
|---|---|---|
| `meta` | `key`, `value` | goal, step, params, embedding settings, config hash, schema version |
| `node` | `id` PK, `kind`, `section`, `text`, `base`, `touched`, `pinned`, `sup`, `parent`, `idx`, `src`, `hash`, `source`, `labels` | one row per node |
| `field` | `node_id`, `name`, `value`, `num` | record fields; `num` holds the normalised numeric/money value |
| `edge` | `a`, `b`, `type`, `w` | adjacency, indexed both ways |
| `stem` | `stem`, `node_id` | inverted index for graph search and text filters |
| `vec` | `node_id`, `kind` (`raw`/`enr`), `dim`, `data` BLOB | float32 little-endian |
| `ext` | `source`, `id`, … | external stubs (Addendum C) |
| `srcfile` | `path` PK, `node_id`, `hash` | incremental import |
| `journal` | `seq`, `ts`, `event` | events, trimmed to the last 10k |

Indexes: `node(kind)`, `node(section)`, `edge(a,type)`, `edge(b,type)`, `stem(stem)`, `field(name,value)`,
`field(name,num)`. The query planner is SQLite's; brainDB only has to translate BDQL `WHERE` clauses into SQL.

## F3. In-process working set

**Implemented today:** the first mode only. Whatever the backend, the brain is loaded whole into memory and
SQLite is the durable store. Raw queries push `kind`, `section`, `id` and field predicates down to the `node` and
`field` indexes as a sound pre-filter, re-checked in memory, with a fallback to the scan for unselective predicates.
Lazy loading, the LRU and text pushdown over the `stem` index are specified below but not built; the `stem` index is
written on every save so it can be added without a migration.

- **Small databases** (≤ `memory.fullLoad`, default 200k nodes): fully loaded in memory, SQLite is the durable store.
- **Large databases:** lazy. Nodes and adjacency are fetched on demand into an LRU (`memory.maxNodes`).
  Activation walks level by level, fetching each frontier in one query; raw queries are answered by SQL over the
  indexes, and only the result page is materialised.

## F4. Indexes and algorithms (all backends)

**Implemented: the raw-query pre-filter.** On the SQLite backend a raw query's field predicates are
answered from the `field` index and the ids they return replace the walk over every node. Three rules
keep it safe, and each one came out of a measurement rather than a guess:

- it is a **pre-filter, never a decision**. Whatever SQL returns still goes through the same in-memory
  predicate, so the SQL only has to be sound — it may return rows that do not match, but it must never
  drop one that does. A numeric comparison therefore keeps rows whose value is not a number (in memory
  those fall back to a string comparison, which can match), and case-insensitive equality is pushed
  down only for ASCII literals, because SQLite's `NOCASE` folds ASCII while Go folds Unicode;
- it **stands down when the database could disagree with memory**. Every structural change bumps a
  generation counter that the writer records; if the two differ, the query scans;
- it is **bounded, and the bound is the selectivity test**. One predicate drives the lookup: the first
  whose index probe returns fewer than `selectivityCap` (2000) rows. Hitting the cap means the
  predicate matches a large part of the brain, which measured *slower* through SQL than in memory, so
  the query scans instead. The remaining predicates are left to the in-memory filter, which re-checks
  every candidate anyway.

The probe is a bounded index range scan with no `ORDER BY` and no `DISTINCT`, and brain order is
restored in Go over the few rows that come back. Both of those clauses, and an index whose collation
did not match the query's, each made an earlier version materialise the whole result before the bound
could apply — and measured slower than the scan it was meant to replace.

- **Inverted stem index** for graph search and text filters: written on every save, not yet read. Text
  matching is a substring test, which stem membership does not imply, so pushing it down would need the
  stems of the query term — specified, not built.
- **Field indexes** for records: equality and range, both in SQL on the `field` table.
- **Vector KNN:** brute force. It is linear, but a 100k × 1536 float32 scan is tens of milliseconds, which meets the
  targets below; an approximate index is deferred until a benchmark shows brute force is actually the bottleneck.
- **Enrichment (APPNP)** runs incrementally: only nodes within `hops` of the changed nodes are recomputed.
- **Activation** is cached by `(focus, params, step)`.
- **Embeddings** are cached on disk by `sha256(model + text)` (Addendum F8), so remote providers are paid once per
  distinct text, ever.
- **Bulk import:** parse → batch insert in one transaction → embeddings in batched provider calls → auto-link → commit.
- A **write lock** (SQLite's own, or a flock on `brain.lock` for the file backend) makes exec batches atomic across
  processes.

## F5. Configuration and interfaces

```yaml
storage:
  backend: file                  # file (default) | sqlite; validated against path's extension
  path: brain.db                 # overrides the brain path; .db/.sqlite select the SQLite backend
memory: {fullLoad: 200000, maxNodes: 200000}
cache: {activationTtl: 60, searchTtl: 60, federationTtl: 300}
```

- Env: `BRAINDB_STORAGE`.
- CLI:
  - `braindb migrate --brain brain.xml --to sqlite|file` copies everything both ways, losslessly (the XML of the migrated brain equals the original's), and leaves the source file untouched;
  - `braindb bench --nodes 100000` runs a synthetic benchmark;
  - `braindb stats --db crm` reports storage, node and edge counts, index sizes, cache hit rate, p50/p95 latencies.
- HTTP: `GET /api/stats` gives the same as `braindb stats`; `/api/health` includes `storage` and the embedding engine.
- UI: the status bar shows the storage backend and the latency of the last op; the Schema tab gets a
  **Storage & performance** panel, with Storybook stories.

## F6. Performance targets

Measured by `braindb bench` and Go benchmarks on a 100k-node synthetic brain (records + chunks + concepts, avg degree 8).
**These are targets. None of them has been measured yet, and `docs/BENCHMARKS.md` says so explicitly until it holds
real numbers.**

| operation | target |
|---|---|
| import 100k CSV rows | < 30 s |
| activation, depth 2 | p95 < 50 ms |
| hybrid search | p95 < 100 ms |
| indexed raw query (2 field filters + sort + limit 50) | p95 < 30 ms |
| exec batch of 10 ops | p95 < 20 ms |
| cold start, full load | < 2 s |

When a target is missed, the report says so, with numbers.

## F7. Deployment

- The default image needs no extra services: one process, one file, one volume.
- `make bench` produces `docs/BENCHMARKS.md`.

## F8. Embedding providers (implemented)

The vector engine is pluggable. `internal/brain.Embedder` has one method, `Embed([]string) ([][]float32, error)`, and
two implementations.

| provider | behaviour |
|---|---|
| `hash` (default) | `hash-256`: 5-letter stems, bigrams and char trigrams, signed FNV-1a hashing, L2-normalised. Offline, deterministic, free. **Lexical, not semantic**: it matches shared word shapes, so synonyms do not match. |
| remote | A named provider from the registry in `internal/brain/embedproviders.go`. The name decides the endpoint, the auth header, the batch ceiling and the wire format; `baseUrl` only overrides the endpoint. `GET /api/embedding` returns the registry. |

Three wire formats cover every provider in the registry:

| wire | request | response | providers |
|---|---|---|---|
| `openai` | `POST {baseUrl}/embeddings` `{model, input[], encoding_format:"float"}`, plus `input_type` where the provider asks for one | `{data:[{index, embedding}]}` | openai, openrouter, azure, voyage, jina, mistral, together, deepinfra, huggingface, ollama, lmstudio, vllm, tei, compatible |
| `cohere` | `POST {baseUrl}/v2/embed` `{model, texts[], input_type, embedding_types:["float"]}` | `{embeddings:{float:[[…]]}}` | cohere |
| `gemini` | `POST {baseUrl}/models/{model}:batchEmbedContents` `{requests:[{model, content:{parts:[{text}]}, taskType}]}` | `{embeddings:[{values:[…]}]}` | gemini (alias google) |

Auth is per provider: `Authorization: Bearer` by default, `api-key` for Azure, `x-goog-api-key` for Gemini. Azure
also puts the deployment in the path and appends `?api-version=` (`embedding.apiVersion`, default `2024-02-01`).
Vectors are L2-normalised after decoding whatever the provider returned, so ranking does not depend on the engine's
conventions. `batch` is clamped to the provider's ceiling instead of being passed through to a 400.

```yaml
embedding:
  provider: openai
  model: openai/text-embedding-3-small
  baseUrl: https://openrouter.ai/api/v1
  apiKeyEnv: OPENROUTER_API_KEY     # variable NAME; the key is never written to config, brain.xml or the cache
  dim: 1536                         # optional; learned from the first response
  batch: 64
  timeout: 30
  retries: 2                        # on 429/5xx, exponential backoff capped at 8 s; 0 = never retry
  fallback: hash                    # hash (default) = degrade | error = fail the operation
  cache: embeddings.cache
```

Env overrides (precedence flags > env > `braindb.yaml` > defaults): `BRAINDB_EMBED_PROVIDER`, `BRAINDB_EMBED_MODEL`,
`BRAINDB_EMBED_BASE_URL`, `BRAINDB_EMBED_KEY_ENV`, `BRAINDB_EMBED_DIM`, `BRAINDB_EMBED_FALLBACK`,
`BRAINDB_EMBED_CACHE`.

Required behaviour:
- **Batching.** All node texts of an import go through the embedder in one call per `batch`; the same path serves
  queries, template slots and purpose checks. One import is O(nodes/batch) requests, not O(nodes).
- **Caching.** `embeddings.cache` next to the brain: an append-only file of `sha256(model + text) → float32[dim]`,
  with a header carrying the magic, dimension and model id. A different model or dimension invalidates the file,
  which is then rewritten. The cache is never authoritative: a missing or corrupt file only costs API calls.
- **Dimension.** Learned from the first response when `dim` is unset. A response of the wrong size is an error, never
  silently truncated or padded.
- **Failure.** `fallback: hash` emits hash vectors of the configured dimension, records the reason, and reports the
  engine as degraded in status lines; `fallback: error` fails the operation. A missing `apiKeyEnv` variable is an
  error at load time, never a silent downgrade.
- **Normalisation.** Every returned vector is L2-normalised, so `vcos` stays a cosine.
- Graph enrichment (APPNP) runs on top of whichever engine is configured.

> Changing the engine changes every vector, so vector and hybrid search results change with it. Graph activation,
> weights and the rest of BDQL are unaffected.

---

# Addendum G: BDQL, the brainDB Query Language

One scripting language for **every** operation. Agents write one script instead of calling many tools, so MCP shrinks to two
tools. The language is SQL-like, case-insensitive for keywords, statements end with `;`, and `--` starts a comment.
BDQL is a thin layer over the existing operations (search, query, exec ops, fill, ingest, sources). It adds no new semantics.

## G1. Statements

```sql
USE crm;                                                   -- switch database (multi-db)
SHOW kinds | sections | concepts | sources | dbs | stats | config | templates;

-- read
CONTEXT [FOCUS d121, d123 | FOR "renewal risk"] [DEPTH 2] [THRESHOLD 0.16] [HOP 0.85] [SOURCES lab | *] [FORMAT md] [SET];
        -- without SET: ad-hoc slice (A3); with SET: persists focus/params (like brain_view args)
SELECT id, kind, text, weight, activation, fields.value
  FROM memory | concepts | links | memory@lab | memory@*
  WHERE kind IN (deal, account) AND fields.stage = 'negotiation' AND fields.value > 10000
        AND text ~ 'discount' AND concept = 'renewal' AND lit AND NOT superseded AND age <= 30 AND weight >= 0.2
  NEAR "renewal risk" [MODE hybrid | vector | tree | graph]
  ORDER BY score DESC, fields.value DESC
  LIMIT 10 OFFSET 0
  FORMAT md;                                               -- json | yaml | xml | md | csv
SELECT COUNT(*), fields.stage FROM memory WHERE kind = deal GROUP BY fields.stage;   -- COUNT/SUM/AVG/MIN/MAX on fields
SEARCH "cycle life guarantee" [MODE hybrid] [LIMIT 5] [SOURCES lab] [FOCUS];
INSPECT d121 [WITH item, links, near];   TEST d121;   SUGGEST d121;   DISCOVER;
PATHS FROM q143 TO d127 [LIMIT 3];
EXPLAIN SELECT …;                                          -- plan: indexes used, estimated rows, cache hits

-- write (atomic per script, see G3)
REMEMBER fact "Graph API limit suffices for nightly sync"
  [IN facts] [CONCEPTS "Integrations", "Performance"] [LINK TO i171 WEIGHT 0.8, $F]
  [FIELDS (stage = 'won', value = 12000)] [SUPERSEDES f107] [PIN] [FOCUS] [AS F];
INSERT INTO deal (text, account, stage, value, close_date)                         -- records; columns other than
  VALUES ('Q4 renewal', 'Halvorsen Freight', 'negotiation', 48000, '2026-12-15'),  -- text/section/concepts → fields
         ('Upsell telematics', 'Ostrava Logistics', 'proposal', 12500, '2027-01-20');
UPDATE memory SET text = 'Won at 46k', fields.stage = 'won' WHERE id = g204;       -- or any WHERE (bulk)
CONCEPT "Churn risk" [MEMBERS g204, g210] [RELATED "Renewal"] [AS C];
LINK d121 TO lab:x204 [WEIGHT 0.7] [TYPE assoc];
UNLINK d121 FROM f103;
BOOST d121, f400;   WEAKEN n152;   PIN d127;   UNPIN d127;
SUPERSEDE f107 WITH f400;
FORGET n152 [FORCE];
FORGET WHERE kind = note AND age > 90 AND NOT pinned [DRY RUN];                   -- bulk delete needs WHERE
FOCUS d121, d123;   FOCUS ADD $F;   FOCUS REMOVE n150;   FOCUS GOAL;
GOAL "Prepare Q4 renewals" [FOCUS d121];

-- documents, data, sources
FILL TEMPLATE 'account-brief.yaml' OUTPUT json [SCOPE context | memory | federated] [WITH SOURCES] [MIN 0.2];
FILL $$
client: "client name"
arr: "annual contract value:money"
$$ AS yaml OUTPUT md;
INGEST '/data/inbox/' [FOCUS];
IMPORT '/data/export/' [UPDATE] [DRY RUN];
SOURCE ADD lab 'http://lab:7791' [TOKEN ENV LAB_TOKEN] [WEIGHT 0.7] [DEPTH 1];
SOURCE SYNC lab;   SOURCE REMOVE lab;
KIND ADD customer [COLOR '#ff8787'] [HALF LIFE 90] [BASE 0.7] [PREFIX u] [SECTION customers];

BEGIN; … COMMIT;   ROLLBACK;
LET $n = (SELECT COUNT(*) FROM memory WHERE kind = deal);
```

**Lexical rules:**

| element | form |
|---|---|
| ids | bare: `d121`, `lab:x204`, `A:pricing` |
| strings | `'…'` or `"…"` (with `\` escapes), or dollar-quoted `$$…$$` for multi-line |
| numbers | including money/percent in comparisons |
| booleans | `true` / `false` |
| field paths | `fields.x` |
| variables | `$F`, set by `AS F` or `LET`; `$F` used where an id is expected; `$n` used where a value is expected |
| operators | `= != <> > >= < <= ~` (contains), `IN (…)`, `NOT`, `AND`, `OR`, parentheses |
| pseudo-columns | `lit`, `pinned`, `superseded`, `external` |
| `score` | present only with `NEAR` |
| `concept = 'x'` | matches the concept label or id |
| `age` / `touched` | time: `age` in days (relative), `touched` as an ISO-8601 instant (absolute), the same value the XML carries. There is no `AS OF` / time travel |

**Missing values.** A node that does not carry the field has no value for it, and no value satisfies any
comparison: `<`, `<=`, `>`, `>=`, `=`, `!=`, `IN` and `NOT IN` are all false for it, and `BETWEEN` with it
is false. `fields.x != ''` is therefore the idiom for "has a value". Without this rule a missing value would
read as the empty string and `WHERE fields.close_date < '2026-12-01'` would match every node that has no
close date at all.

**Editing `braindb.yaml` from the running server.** Writes that the UI or BDQL make to the config
(`PATTERN ADD`/`REMOVE`, enabling a pattern, `SetConfigKey`) replace only the lines that hold the value
they change. Comments, blank lines, key order and inline flow style elsewhere in the file survive byte
for byte, because a parse-and-re-emit round trip cannot keep them: the parser drops comments. Pattern
changes are written once the transaction commits, so a failed script leaves the file alone.

**Unknown columns.** A column name that no row of the table defines is an error, not a NULL:
`unknown column "banana" in memory (has: …)`. A name counts as known when it is a built-in column of the
table or a field carried by at least one of its rows; `entities.<type>` and `has:<type>` are always accepted,
since custom patterns define the types. The verdict is drawn from the table, never from the rows a particular
condition order happened to visit, so it does not depend on how the `WHERE` clause is written. A statement
that scans no rows at all is not checked.

The compact query syntax from A2 stays as a shorthand: `QUERY 'kind:deal stage=negotiation sort:-value limit:10' [FORMAT md];`.

## G2. Results

A script returns one result per statement: `{index, statement, kind: rows|text|changes|plan, columns?, rows?, text?, changes?, ids?, elapsed_ms}`.
The script result is `{results, status, ok, error?}`.

Rendering for MCP and CLI, compact and token-frugal:
- rows as a Markdown table, or in the requested `FORMAT`;
- writes as `changes: +f400 ⇄f107 ▲d121`;
- text (context, fill output) verbatim;
- then one status line, as before.

## G3. Transactions and safety

- **Atomicity.** A script that contains writes runs as one transaction: all statements succeed, or nothing is applied (atomic snapshot, or one SQLite transaction).
  Explicit `BEGIN … COMMIT` splits a script into several transactions.
  Reads inside a transaction see its own uncommitted writes.
- **Errors** report `statement N, line L, col C: message`, with a caret excerpt and "did you mean" for keywords, ids, kinds and fields.
- **Bulk guard.** `UPDATE` and `FORGET` without `WHERE` are errors. Bulk writes affecting > 100 nodes need `CONFIRM` at the statement's end.
  `DRY RUN` shows what would change.
- **Permissions.** Read-only contexts (published `readonly`, `mcp.tools` without write, token scopes) reject write statements with a clear error.
- **Limits.** Max statements per script 200, max rows returned 1000 (`LIMIT` cap), timeout 30 s; all configurable.

## G4. Interfaces

- **MCP** default tools: `brain_view` (input: the lit context, `format`) and `brain_run` (`{script, format?, dry_run?}`).
  - Its description carries a compact BDQL cheat-sheet generated from the grammar, as ctxc does.
  - The per-operation tools (`brain_search`, `brain_exec`, …) stay available with `mcp.tools: all` (config) or `--tools all`.
  - The `initialize` instructions show a typical one-call turn:
    ```sql
    CONTEXT FOR "…";
    SEARCH "…" LIMIT 5;
    REMEMBER … AS F;
    LINK $F TO …;
    BOOST …;
    ```
- **HTTP:** `POST /api/run {script, format?, dryRun?}`. `POST /api/run/explain` parses only and returns the AST + plan, used by the UI linting.
- **CLI:**
  - `braindb run -e "SELECT …"`, `braindb run script.bdql`, `cat x.bdql | braindb run -`;
  - `braindb shell`: REPL with multi-line input until `;`, history file `~/.braindb_history`, `\d` (=SHOW), `\f md` (default format), `\q`.
- **UI:** the Query tab editor is BDQL-first.
  - Syntax highlighting, autocomplete (keywords, ids from State, kinds, sections, concepts, field names from records, template names), lint markers from `/api/run/explain`.
  - Run the statement at the cursor (Ctrl+Enter) or all (Ctrl+Shift+Enter); one result tab per statement.
  - EXPLAIN view; a snippet library of examples.
- **Publish:** a published binary in readonly mode exposes `brain_view` + `brain_run` (reads only).

## G5. Implementation notes and tests

- Hand-written lexer + recursive-descent parser in `internal/bdql`, producing an AST.
- A planner maps the AST onto the existing brain operations and the indexes (F4).
- A golden-file test suite in `internal/bdql/testdata/*.bdql` with expected outputs. It covers every statement, error messages
  with positions, transactions and rollback, variables, bulk guards, readonly rejection, and GROUP BY aggregates.
- Fuzz test for the parser (`go test -fuzz`) for at least 30 s in CI.
- Each example ships a `queries.bdql` that runs green with `braindb run`.

---

# Addendum H: connector nodes (structural, knowledge-free)

brainDB has two node classes:

| class | examples | carries knowledge | in context output and tokens | search hit | decays |
|---|---|---|---|---|---|
| **knowledge** | memories (all kinds), records, chunks, concepts ◆ | yes | yes | yes | yes (concepts: no) |
| **connector** ⧉ | anchor, bridge, hub, alias, time | **no** | no (only as routes) | no | no |

**Connectors** organise connections that come from different places: files, tables, imports, other brains, vocabularies.
They add no facts of their own, only a label and an optional key.
- A connector turns n×m pairwise links into n+m links through one junction.
- It lets activation and search reach related knowledge without inventing an edge between every pair.
- **How they differ from concepts:** a concept is abstract knowledge with meaning (◆ "Pricing"). A connector is plumbing
  (⧉ "entity customer:42", "bridge sales↔lab", "alias ARR = annual contract value").

## H1. Connector types

| type | purpose | typical source | default gain | transparent |
|---|---|---|---|---|
| `anchor` | one real-world entity referenced in many places (customer 42 as a SQL row, in tickets, in meeting notes, in another brain) | import key matching, `RESOLVE` | 0.95 | yes |
| `bridge` | the junction between a local area and an external source / other brain | federation links | 0.85 | yes |
| `hub` | groups a workstream or batch, so many items share one entry point ("Q4 renewals", "import 2026-11-20 crm.sql") | user, import batches | 0.8 | no |
| `alias` | vocabulary mapping that routes query terms (ARR ↔ annual contract value ↔ ACV) | user, config, co-occurrence | 0.9 | yes |
| `time` | time bucket (day/week/month) connecting what happened together | **planned**; the type exists, nothing builds them yet | 0.7 | no |

## H2. Semantics

- Id prefix `H:` (e.g. `H:customer-42`).
  Fields: `type`, `label`, `key?` (e.g. `customer:42`), `gain`, `transparent`, `terms?` (for alias), `source?`.
- **Weight:** fixed. No decay, no boost or weaken. The gain replaces `(0.35 + 0.65·weight)` in activation.
- **Activation through a connector:**
  - entering it: `a = act(u) · w(u,H) · gain`. It does not apply `hop`, and when `transparent` it does **not** consume a depth level;
  - leaving it: the normal rule (with `hop`);
  - a connector is never "lit" as an item. A lit node reached through one records `via` = the knowledge node before it plus `route` = the connector id(s).
  - Fan-out guard: a connector with > `connectors.maxFanout` (default 200) neighbours passes activation only to its top-N neighbours, ranked by edge weight × node weight.
- **Context output:**
  - connectors are not items and cost no tokens;
  - XML: items reached through one get `route="H:customer-42"`, and a compact `<routes>` element lists the connectors used: `<route id type label key used="5"/>`;
  - Markdown: a "Routes" line under the item, and an "Organising connectors" list at the end;
  - JSON/YAML: a `routes` array.
- **Search:**
  - never returned as rows, but used for expansion. Hybrid and graph activation pass through them; tree beam search expands through them without counting a level when transparent;
  - `alias` connectors rewrite and expand query terms before vector/stem matching (the query gets the alias terms appended).
- **Embedding:** a connector vector is the weighted mean of its neighbours. It is used only for routing and suggestions, never as a hit.
- **Suggestions and discover:** they propose connectors. Each proposal comes with evidence and can be created in one click or statement:
  - `anchor`: nodes from ≥ 2 places sharing a key value (same `customer_id` / account name / email) or high cosine plus a shared proper name;
  - `hub`: a clique-like cluster with many pairwise edges;
  - `alias`: terms that co-occur or map through concepts.
- **Import (Addendum E):**
  - configurable automatic anchors: `connectors.anchors: [{key: customer, match: ["customers.id", "orders.customer_id", "tickets.customer_id", "fields.account"]}]`;
  - SQL foreign keys can produce anchors instead of direct edges (`foreignKeys: anchor`);
  - each import run may create a `hub` for its batch (`connectors.importHub: true`).
- **Federation (Addendum C), planned:** an external link is meant to go through a `bridge` per source and area (`link a → H:bridge-lab → lab:x204`), keeping cross-brain edges organised and easy to audit or remove. Today bridges are only created by hand (`CONNECTOR bridge "…"`); nothing builds them automatically, so `connectors.bridges` is rejected rather than silently ignored.
- **Stats:** `connectors`, `routesUsed`, and `connectorEdges` are separate from knowledge counts. `lit` and `tokens` exclude connectors.

## H3. Storage

- Brain XML: `<connectors><connector id="H:customer-42" type="anchor" label="Halvorsen Freight" key="customer:42" gain="0.95" transparent="1" terms="" source=""/></connectors>`.
- Edges to connectors have type `connector`.
- SQLite (F2): a `node` row with `kind = 'connector'`, plus indexes on connector type and key.
- The round-trip stays lossless.

## H4. Interfaces

**BDQL:**

```sql
CONNECTOR anchor "Halvorsen Freight" KEY 'customer:42' CONNECT g204, r311, t140, lab:x204 [GAIN 0.95] [TRANSPARENT] AS H;
CONNECTOR alias "ARR" TERMS 'annual contract value', 'ACV';
CONNECTOR hub "Q4 renewals" CONNECT g204, g210, d121;
CONNECT n152 TO $H;   DISCONNECT n152 FROM $H;
RESOLVE customer BY fields.customer_id, fields.account [MIN 2] [DRY RUN];  -- create anchors from shared key values
SELECT id, type, label, key, degree FROM connectors WHERE type = anchor ORDER BY degree DESC;
SHOW connectors;
SUGGEST CONNECTORS [LIMIT 10];
SELECT … WHERE route = 'H:customer-42';  -- knowledge reached through a connector
```

Pseudo-columns: `connector` (bool) and `route`.

**HTTP:**
- State gets `connectors: Connector[]`, with `Connector {id, type, label, key?, gain, transparent, terms?, source?, degree}`;
- edges of type `connector`;
- `Node.route?: string[]`;
- `GET /api/inspect?what=connectors` returns suggestions.

**MCP:** through `brain_run`. The cheat-sheet includes the connector statements.

**UI:**
- connectors render as small hollow rings ⧉ with thin dashed edges, in a neutral colour. A **Show connectors** toggle defaults to on in Graph and off in the Brain view.
- On hover, a connector highlights everything it joins.
- The Inspector shows type, key, gain, transparent, degree, the sources it spans and the routes used in the current context, with actions connect, disconnect, convert hub ↔ anchor and dissolve.
- The Navigator gets a Connectors group by type. The Schema tab shows counts. A "Suggested connectors" list in Discoveries has create buttons.
- Storybook stories for all of these.

**Config:**

```yaml
connectors:
  maxFanout: 200
  importHub: false
  # bridges: and time: are specified above but not built yet; the config rejects them
  # rather than accept a setting that does nothing.
  anchors:
    - {key: customer, match: ["customers.id", "orders.customer_id", "tickets.customer_id", "fields.account"]}
  aliases:
    - {label: ARR, terms: ["annual contract value", "ACV"]}
```

## H5. Tests

- activation passes through transparent connectors without consuming depth, and through opaque ones with depth;
- gain is applied; the fan-out guard works;
- connectors are excluded from rows and tokens but appear in routes;
- alias expansion changes search results;
- `RESOLVE` builds anchors across SQL + JSONL + Markdown in import-demo;
- XML round-trip;
- the SQLite backend stores and loads connectors.

## G6. Grammar completeness (added with the agent guide)

The grammar must accept everything in `skills/braindb-bdql/reference/cheatsheet.md`, including:
- `SELECT *` and `fields.*` (all fields of the matched records);
- `FROM connectors` and `memory@*`;
- `SHOW connectors | templates`;
- `SEARCH … FOCUS`;
- `QUERY '<compact>'`;
- `INSPECT … WITH …`;
- `SUGGEST CONNECTORS`;
- `UPDATE … DRY RUN CONFIRM`.

A test extracts every fenced ```sql block from `docs/AGENTS.md`, `README.md` and `skills/**/*.md`, skips blocks that contain
`<…>` placeholders, and parses (not executes) each one. Any parse error fails the build, so the docs cannot drift from the
language.

---

# Addendum I: built-in pattern library (entities)

brainDB recognises, normalises and indexes typed **entities** inside every text it stores: memories, chunks, records'
fields and file names. Entities are **derived data**: they are recomputed deterministically from text, so they are not
stored in brain.xml. Only custom pattern definitions live in `braindb.yaml`. They are kept in an index (in memory, or in SQLite
`ent:{type}:{value}` sets) and power queries, search boosts, template slots and automatic anchors.

## I1. Built-in patterns (`internal/patterns`, Go RE2 regexps, so there is no catastrophic backtracking)

| type | recognises (examples) | normalised value | validator |
|---|---|---|---|
| `date` | `2026-12-15`, `15.12.2026`, `12/15/2026` (locale), `15 Dec 2026`, `December 15, 2026`, `15 grudnia 2026` (pl), `Q4 2026`, `2026-W47` | ISO `2026-12-15`, `2026-Q4`, `2026-W47` | real calendar date |
| `datetime` | `2026-11-20 09:30`, `2026-11-20T09:30:00Z`, `9:30 am` (with a date nearby) | RFC 3339 | |
| `duration` | `18 months`, `90 days`, `2h 30m`, `P18M` | ISO 8601 duration `P18M` | |
| `money` | `€48,000`, `48 000 EUR`, `$12.5k`, `12,50 zł`, `PLN 1 200,00`, `USD 3.2M` | `{amount: 48000.00, currency: "EUR"}` → `EUR 48000.00` | ISO 4217 currency list |
| `percent` | `12%`, `12.5 %`, `12 percent` | `0.125` | |
| `number_unit` | `4.5 V`, `3.2 mS/cm`, `25 °C`, `500 rps`, `12 kg` | `{value, unit}` (SI-normalised where trivial) | unit list |
| `email` | `kim.aalto@halvorsen.example` | lower-cased | RFC-lite + TLD |
| `phone` | `+47 22 33 44 55`, `(555) 010-2030`, `+48 600 100 200`, `600-100-200` | E.164 `+4722334455` (default region from config) | length per country code |
| `url` | `https://halvorsen.example/pricing`, `www.example.org`, `example.org/docs` | canonical URL; `host` also indexed as `domain` | public suffix-lite |
| `domain` | `halvorsen.example`, from emails and urls | lower-cased host | |
| `person` | `Kim Aalto`, `Dr. Ana Petrov`, `Ms. Lee`, `Jan Kowalski` (capitalised 2–3 token names, titles, an optional first-name gazetteer) | `Kim Aalto` | not at sentence start alone; not a known org or place |
| `org` | `Halvorsen Freight AS`, `Brightline Supply Co.`, `Acme GmbH`, `Nordic Labs Ltd`, `sp. z o.o.`, `S.A.`, `Inc.` | name without the legal suffix + `suffix` | legal-suffix list |
| `iban` | `NO93 8601 1117 947` | compact upper-case | mod-97 |
| `vat` | `NO 123456789 MVA`, `PL5260250274`, `DE123456789` | country + number | per-country checksum where known |
| `card` | 13–19 digits | masked `**** 1111` (never stored in clear in outputs) | Luhn |
| `ip` | IPv4 / IPv6 | canonical | net/netip |
| `version` | `v1.2.0`, `3.11.174`, `protocol v3` | semver-ish | |
| `id_code` | `SKU-10422`, `INV-2026-0042`, `R-114`, ticket ids `#4821` | upper-cased | |
| `hashtag` · `mention` | `#renewal`, `@kim` | lower-cased | |
| `geo` | `59.91, 10.75` | `lat,lon` | range |
| `address` | street + number + postal code + city (EU/US forms) | single line | postal code pattern per country |

Locales (`patterns.locales`, default `[en]`; built-in `en, pl, de, fr, es, nb`) add month names, date orders, decimal and
thousands separators, and currency words.

- **Precision over recall:**
  - ambiguous matches need context (e.g. `12/11/2026` uses `patterns.dateOrder: dmy|mdy`);
  - bare numbers are never money without a currency token.
- Each entity records `{type, value (normalised), raw, start, end, confidence, pii}`.

## I2. Uses

- **Index and query.** Pseudo-table `entities (id, node, type, value, raw, confidence)` and per-node columns.

  ```sql
  SHOW patterns;
  SELECT type, COUNT(*) FROM entities GROUP BY type;
  SELECT value, COUNT(*) FROM entities WHERE type = email GROUP BY value ORDER BY COUNT(*) DESC LIMIT 20;
  SELECT id, text FROM memory WHERE HAS email AND entities.domain = 'halvorsen.example' LIMIT 10;
  SELECT id, text, entities.money FROM memory WHERE entities.money > 'EUR 10000' AND entities.date BETWEEN '2026-12-01' AND '2026-12-31';
  SELECT id, text FROM memory WHERE entities.person = 'Kim Aalto' ORDER BY weight DESC LIMIT 10;
  EXTRACT FROM "Call Kim Aalto (+47 22 33 44 55) before 15 Dec 2026 about €48k";   -- test the library on any text
  EXTRACT g204;                                                                     -- entities of one node
  PATTERN ADD sku '\bSKU-\d{5}\b' [TYPE id_code] [NORMALIZE upper] [PII false];      -- custom pattern (written to braindb.yaml after the transaction commits)
  PATTERN TEST sku 'Order SKU-10422 and SKU-1'; PATTERN REMOVE sku;
  ```

  - `money` compares only within the same currency, unless `patterns.fx` gives conversion rates.
  - `date` compares as ISO.
  - `HAS <type>` tests for presence.
- **Search.** Entities found in a query (an email, a phone, an id code, a date) add an exact-match boost to nodes that
  carry the same normalised value. "who is +47 22 33 44 55" finds the contact even when written as `22334455`.
- **Template fill (A4).** The slot types `date`, `money`, `percent`, `email`, `phone`, `url`, `name` (= person),
  `number` and `id` use the library, so templates and queries share one definition. New slot types: `org`, `domain`,
  `duration`, `iban`, `vat`, `address`, `version`.
- **Connectors (H).** `RESOLVE email | phone | domain | org | person | vat | id_code [MIN 2]` creates `anchor` connectors
  by normalised entity value. "The same person or company across SQL rows, tickets, notes and other brains" becomes
  one statement. Config `connectors.entityAnchors: [email, phone, vat]` does it automatically on import.
- **Import (E).** Entities are extracted from records' fields per column. A column whose values are ≥ 80% one entity type
  is typed accordingly in the ImportReport and used for numeric and date comparisons.
- **Privacy.** Types with `pii: true` (email, phone, person, address, iban, vat, card, ip):
  - `privacy.mask: [phone, iban, card]` masks them in context output, MCP replies and exports (`+47 •• •• 44 55`);
  - `privacy.redactOnIngest: [card]` replaces them before storing;
  - `card` is always masked in outputs.
  - `SHOW pii` reports counts per type, for audits.

## I3. Config

```yaml
patterns:
  locales: [en, pl, nb]
  defaultRegion: NO          # for phone numbers without a country code
  dateOrder: dmy             # dmy | mdy | ymd for ambiguous numeric dates
  disabled: [hashtag]
  custom:
    - {name: sku, type: id_code, regex: '\bSKU-\d{5}\b', normalize: upper}
    - {name: run_id, type: id_code, regex: '\bR-\d{3,4}\b'}
  fx: {base: EUR, rates: {USD: 0.92, NOK: 0.085, PLN: 0.23}}   # optional, for cross-currency comparison
privacy:
  mask: [phone, iban, card]
  redactOnIngest: [card]
```

Custom regexes are validated at load. Go RE2 rules out catastrophic backtracking.
Each pattern is limited to 1000 matches per text.

## I4. Interfaces

- **HTTP:**
  - `GET /api/patterns` lists the built-in and custom patterns with their types;
  - `POST /api/patterns/test {text, types?}` returns entities;
  - `GET /api/entities?type=&value=&limit=` returns entity rows;
  - `Node.entities?: Entity[]` in State (only when `?entities=1`, to keep State small) and in Inspect `item`.
- **MCP:** through `brain_run` (EXTRACT, PATTERN, entity queries). The cheat-sheet includes them.
- **UI:**
  - entities are highlighted inline in item text (chips coloured by entity type, with the normalised value on hover);
  - Inspector "Entities" table;
  - Navigator group "Entities" (types → top values → nodes);
  - Schema "Patterns" panel: list, enable/disable, add a custom pattern, and a live tester box that highlights matches as you type;
  - masked PII shows a lock icon;
  - Storybook stories for all of these.

## I5. Tests

- a fixture corpus per type and locale (≥ 20 positives and ≥ 10 hard negatives each: version numbers vs dates, phone vs order numbers, money vs plain numbers);
- precision ≥ 0.95 and recall ≥ 0.85 per type on the corpus, reported by a test;
- normalisation round-trips;
- validators (Luhn, mod-97, calendar);
- masking in every output path (context XML/MD/JSON, MCP, exports, fill);
- RESOLVE by email/phone across SQL + JSONL + MD in import-demo;
- custom pattern add/test/remove;
- a benchmark: extraction over 100k records within the import targets of F6.

---

# Addendum J: remote MCP over HTTP and scoped tokens

Cloud-hosted agents (Grok Bot, remote Hermes/OpenClaw gateways, hosted IDE agents) cannot spawn a local stdio process or
reach `localhost`. brainDB therefore also speaks MCP over HTTP.

- **Endpoint `/mcp`:** MCP Streamable HTTP transport on the same server as the API. The same tools, instructions and status lines as stdio.
  - `POST /mcp` takes a JSON-RPC request (or batch) and replies with `application/json`, or with `text/event-stream` when the client accepts it.
  - `GET /mcp` opens an SSE stream for server messages.
  - Sessions use the `Mcp-Session-Id` header. `MCP-Protocol-Version` is honoured. `DELETE /mcp` ends a session.
  - Database selection: `?db=` or `X-BrainDB-Database`. Actor name: `X-BrainDB-Actor`, or the token's name.
- **Scoped tokens** (env or config `server.tokens`):

  ```yaml
  server:
    tokens:
      - {name: grokbot, tokenEnv: BRAINDB_TOKEN_GROKBOT, scope: read,  dbs: [sales]}
      - {name: hermes,  tokenEnv: BRAINDB_TOKEN_HERMES,  scope: write, dbs: ["*"]}
      - {name: admin,   tokenEnv: BRAINDB_TOKEN,         scope: admin}
  ```

  | scope | allows |
  |---|---|
  | `read` | read statements, `brain_view`, read BDQL; writes rejected with a clear error |
  | `write` | adds writes and ingest |
  | `admin` | adds db create/delete, config, sources and patterns |

- **Cross-site guards** (they run before authentication, because by default no token is configured):
  - a request carrying an `Origin` that is not the server's own is refused, unless the origin is listed
    in `BRAINDB_ALLOWED_ORIGINS` (comma-separated, `*` allows any);
  - a write must send `application/json` (or XML/YAML where the endpoint takes a document, or a multipart
    form to `/api/ingest` and `/api/import`). `text/plain`, a form encoding and a missing `Content-Type`
    are exactly what a cross-origin page can send without a CORS preflight, so they are rejected with 415;
  - when the listener is bound to loopback, the `Host` header must name a loopback address too. This is
    what stops DNS rebinding: a page on an attacker domain resolved to 127.0.0.1 reaches the socket, but
    arrives with the attacker's `Host`.

  - The token name is used as the journal actor.
  - `BRAINDB_TOKEN` alone keeps today's meaning: one admin token.
- `/mcp` requires a token whenever the server listens on a non-loopback address. The server refuses to start in that
  configuration without at least one token.
- `/api/health` reports `mcpHttp: true`.
- Tests:
  - an MCP handshake over HTTP: initialize, tools/list, brain_run;
  - session header handling;
  - a read scope rejects writes;
  - a db restriction is enforced;
  - SSE response mode.

The exception is explicit: `--allow-no-token`, or env `BRAINDB_ALLOW_NO_TOKEN=1`, lets the server listen on a non-loopback
address without tokens. It logs a warning at start and shows `auth: none` in `/api/health`. This is meant for containers
whose port is published only on the host's loopback (the default compose file sets it). The moment any token is configured,
the flag is ignored.


---

# Addendum K: license and attribution

- License: **Apache License 2.0** (`LICENSE`). `NOTICE` credits Gabriel Koziestański as the original author. Section 4(d) of the
  license requires redistributions and derivative works to carry it.
- Source files start with:
  ```
  // Copyright 2026 Gabriel Koziestański
  // SPDX-License-Identifier: Apache-2.0
  ```
  TS/JS use the same, shell uses `#`.
- The attribution is visible in the product:
  - `braindb --version` and `braindb help` print `brainDB <version> · © 2026 Gabriel Koziestański · Apache-2.0`;
  - MCP `serverInfo` has `"title": "brainDB by Gabriel Koziestański"`;
  - `/api/health` includes `"author": "Gabriel Koziestański", "license": "Apache-2.0"`;
  - published binaries embed NOTICE and print it with `--version --notice`;
  - the UI has an About dialog (top bar menu) with the attribution, links to LICENSE and NOTICE, and the version, plus a discreet footer line in the status bar: "brainDB · © Gabriel Koziestański";
  - Storybook has the same in its manager title or about page;
  - the Docker image carries OCI labels (authors, licenses) and `/opt/braindb/LICENSE` and `/opt/braindb/NOTICE`.

---

# Addendum L: storage formats: readable XML and fast binary

The file backend (F1, `storage.backend: file`; the old name `xml` stays as an alias) supports two on-disk formats. The choice is
explicit:

```yaml
storage:
  backend: file
  format: xml        # xml | bin | both
  export: {xml: true, every: 1000, onShutdown: true}   # for format: both, how often brain.xml is refreshed
```

| format | primary file | for |
|---|---|---|
| `xml` (default) | `brain.xml` + `brain.wal.jsonl` | human-readable, diff-friendly, easy to edit and version in git |
| `bin` | `brain.bdb` + `brain.wal.bin` | speed: no parsing; vectors and indexes are loaded ready to use, memory-mapped where the OS allows |
| `both` | `brain.bdb` is the source of truth; `brain.xml` is a readable export refreshed every N ops and on shutdown | speed plus a readable copy for review, git and backups |

## L1. Binary format `.bdb` (versioned, little-endian, stdlib only)

```
header   magic "BRAINDB\x00" · format version u16 · flags u16 · created i64 · brain step u64 · section count u32
toc      per section: id u16 · offset u64 · length u64 · crc32c u32 · compression u8 (0 none, 1 flate)
sections
  STRS   string table (all texts, ids, labels, field names/values), offsets + UTF-8 blob
  META   goal, params, embedding settings, config hash, kinds, sections, sources, connectors (compact JSON or fixed records)
  NODE   columnar node table: id ref, kind u16, section u16, base f32, touched i64, flags (pinned, sup, connector, external) u16,
         parent/sup/route refs u32, fields offset u32
  FLDS   record fields: (node u32, name ref, value ref) sorted by node
  ADJ    CSR adjacency: row offsets u32[n+1], neighbour u32[m], weight f32[m] (or u8 quantised with flag), type u8[m]
  VRAW   raw vectors float32[n×dim]   (or int8 + per-vector scale when quantize: int8)
  VENR   enriched vectors, same layout
  STEM   inverted stem index: stem ref → posting list (delta-encoded varints)
  FIDX   field indexes: equality (value ref → postings) and numeric ranges (sorted (value f64, node u32))
  ENT    entity index (Addendum I): (type, normalised value ref) → postings
  HNSW   vector index graph (when built)
  EXTS   external stubs
```

- **Loading.** The file is memory-mapped where supported (`syscall.Mmap` on unix; read-into-memory on Windows). Strings,
  CSR and vectors are used in place, without copying. Indexes and vectors are trusted only when their section crc matches
  and their config hash equals the current embedding/pattern config. Otherwise they are rebuilt in the background.
- **Writing.** Mutations go to `brain.wal.bin`: length-prefixed binary records with a crc, fsync per batch.
  - Compaction writes a new `.bdb` to a temp file, then fsync, then an atomic rename. It runs every `snapshotEvery` ops, or 30 s, or on shutdown.
  - Crash safety: last valid `.bdb` + WAL replay, with a torn final record ignored.
- **Size options:** `storage.compress: none|flate` (flate is for STRS and postings only; ADJ and vectors stay mappable);
  `storage.quantize: none|int8` (vectors ×4 smaller, recall checked ≥ 0.95).
- **Compatibility:** the format version is checked. Newer files are refused with a clear message. Older versions are
  upgraded on the next compaction. The same `.bdb` is the snapshot used by `braindb publish` (B), so there is one binary format.

## L2. Conversion and tools

```
braindb convert --db crm --to bin|xml|both       # switch format in place (keeps a backup of the old file)
braindb export  --db crm -o brain.xml            # readable export from any format/backend
braindb inspect-file brain.bdb                   # header, sections, sizes, crc status, counts
braindb verify  --db crm                         # crc of every section + WAL integrity + xml/bin equivalence for format: both
```

- **Equivalence guarantee:** `xml → bin → xml` round-trip produces an identical brain.xml. A test checks it on every example.
- **The UI Settings / Storage panel** shows the format and file sizes, and offers a Convert button and an Export XML button.
- **Health and stats** report `format`.

## L3. Targets (added to F6, measured by `braindb bench --format bin|xml`)

| operation | target |
|---|---|
| cold load of a 100k-node brain, `bin` | < 300 ms |
| cold load, `xml` | < 2 s |
| `.bdb` size vs brain.xml with vectors, flate | ≤ 60% |
| `.bdb` size with int8 vectors | ≤ 35% |
| write batch latency | same as WAL xml or better |

## L4. Tests

- round-trip equivalence `xml ↔ bin` on all examples;
- crc corruption detected (single flipped byte per section);
- torn WAL record recovery;
- mmap vs read path give identical results;
- version refusal and upgrade;
- quantised recall ≥ 0.95;
- `format: both` keeps brain.xml within N ops of the binary.

---

# Addendum M: users, roles, grants and access-controlled knowledge

Knowledge can be restricted. Each request runs as a **principal** (user or service). Areas of the brain the principal may
not see are **switched off for them**: those nodes do not exist in their view. Nothing they can observe depends on hidden
knowledge.

## M1. Model

- **Labels.** A node carries 0..n access labels, e.g. `finance`, `hr`, `ip-sensitive`, `customer:marrow`.
  - Unlabelled = visible to everyone with read access to the database.
  - A node is visible to a principal iff the principal holds **every** label on it (clearance semantics).
  - Labels are inherited at creation:
    - chunks and records inherit from their file or table;
    - import rules, sections and kinds can set default labels;
    - `REMEMBER` uses the writer's default labels unless given.
- **Users:** `{name, display, roles[], labels[] (direct clearances), defaultWriteLabels[], dbs[], disabled}`.
  - Authentication by token: each scoped token from Addendum J becomes a user.
  - Optional: JWT bearer verification (HS256/RS256/ES256 via stdlib crypto, issuer/audience/claims mapping to user, roles and labels) for SSO gateways.
- **Roles:** `{name, grants[]}`.
- **Grants:** `{actions: [read|write|admin], labels: [...] | "*", dbs: [...] | "*", sections?: [...], kinds?: [...], sources?: [...], deny?: bool}`.
  - `read` on a label = clearance for that label.
  - `write` = may create or modify nodes carrying those labels (and must hold read for them).
  - `admin` = users, grants and config.
  - `deny` grants override allows.
  - Sections and kinds narrow a grant further, e.g. read `finance` only in the `invoices` section.
- **Effective view.** Every operation evaluates against the principal's visible set. It is computed as a bitmask per node,
  with up to 64 labels per database in a uint64; above that, compressed bitsets. The check is one AND in the hot loop.
- **Admins** see everything. With `AS USER <name>` they can preview another user's view.

## M2. Non-leakage guarantees (the core requirement)

For a principal P, every read operation must return exactly what it would return on a copy of the brain with all nodes
invisible to P (and their edges) **deleted**. Covered operations:
- context;
- search in all modes, with scores;
- SELECT, aggregates and counts;
- paths;
- suggest, discover and connector suggestions;
- fill;
- entities;
- stats and status line;
- layout;
- vectors;
- journal and SSE events;
- error messages.

Consequences:
- **Activation** skips invisible nodes entirely. It does not traverse through them.
- **Connectors** route only between visible nodes. Their degree counts only visible neighbours. A connector whose only purpose is joining invisible nodes is itself invisible.
- **Enriched vectors** are leak-free by construction: a node's enriched vector aggregates only neighbours whose labels ⊆ the node's labels. Anyone who can see the node can see every contributor.
  - Concept prototypes follow the same rule: only members whose labels ⊆ the concept's labels contribute.
  - Hybrid and vector scores therefore never depend on hidden text.
- **Indexes** (stems, fields, entities, HNSW) filter by the visibility bitmask before scoring and counting.
  - HNSW search over-fetches and filters. When a principal's visible fraction is below 20%, it falls back to an exact scan of the visible set.
- **Unknown or invisible ids** produce the same error: `no such item d777`. "Access denied" is used only for writes to a visible node the principal may not modify, or for creating with labels they lack.
- **Status line, stats, health counts and `SHOW`** report visible totals only. `tokens` is computed on the visible slice.
- **Journal and SSE.** Events are filtered per subscriber. Ids the subscriber cannot see are removed, and a summary that mentions hidden content is replaced by `[restricted change]`.
- **Fill.** Slots only use visible evidence. A value that exists only in hidden memory is reported as `missing`.
- **Federation.** Each source call carries the principal (a per-user token mapping `sources[].tokens: {user|role: tokenEnv}`, or a shared token plus the label filter applied locally to stubs). Remote results are filtered by the stub labels.
- **Caches** are keyed by the principal's label set.
- **Writes.**
  - A principal cannot link to, supersede, update or forget nodes it cannot see; they do not exist for it.
  - New nodes get the writer's `defaultWriteLabels` plus explicit `LABELS (...)`, which must be a subset of the writer's write grants.
  - Edges between nodes with different labels are allowed; each side stays visible only to those cleared for it.

## M3. Configuration (`braindb.yaml`, or a separate `access.yaml` referenced by `access.file`, mode 0600)

```yaml
access:
  enabled: true
  labels:                                   # documented labels (unknown labels on nodes are an error at load)
    - {name: finance, description: "Prices, margins, invoices"}
    - {name: hr, description: "People data"}
    - {name: ip-sensitive, description: "Unpublished research"}
  defaults:
    sections: {pricing: [finance], salaries: [hr]}
    kinds: {experiment: []}
    import: {"raw/crm.sql#orders": [finance]}
  roles:
    - {name: sales-rep,  grants: [{actions: [read, write], labels: ["*"], deny: false}, {actions: [read], labels: [finance], sections: [pricing]}, {deny: true, actions: [read], labels: [hr]}]}
    - {name: analyst,    grants: [{actions: [read], labels: [finance]}]}
    - {name: researcher, grants: [{actions: [read, write], labels: [ip-sensitive]}]}
  users:
    - {name: kim,     roles: [sales-rep], defaultWriteLabels: [], tokenEnv: BRAINDB_TOKEN_KIM}
    - {name: grokbot, roles: [],          labels: [], tokenEnv: BRAINDB_TOKEN_GROKBOT, dbs: [sales]}   # public knowledge only
    - {name: admin,   roles: [admin],     tokenEnv: BRAINDB_TOKEN}
  jwt: {issuer: "https://sso.example.org", audience: braindb, jwksUrl: "https://sso.example.org/.well-known/jwks.json",
        claims: {user: sub, roles: groups, labels: braindb_labels}}
```

- `access.enabled: false` (default for new databases) means a single-user brain, exactly as before.
- **Enabling access on an existing brain:** all nodes are unlabelled, so everyone with read access sees everything. Run `LABEL … WHERE …` to restrict.

## M4. Storage

| store | representation |
|---|---|
| brain.xml | `labels="finance,customer:marrow"` on items, connectors and concepts |
| `.bdb` | label table in META, `labels u64` column in NODE (or a bitset section) |
| SQLite | `node.labels`, with an index on it |

Users, roles and grants live in config, never in brain.xml. Secrets are token env names or JWKS, never stored values.

## M5. BDQL

```sql
WHOAMI;                                                     -- principal, roles, labels, dbs
SHOW users | roles | labels | grants [FOR kim];
CREATE USER lee ROLES (analyst) TOKEN ENV BRAINDB_TOKEN_LEE;   ALTER USER lee ADD ROLE sales-rep;   DROP USER lee;
CREATE ROLE auditor;   GRANT read ON LABEL finance TO ROLE auditor [IN SECTION invoices];   REVOKE read ON LABEL finance FROM ROLE auditor;
GRANT read, write ON LABEL ip-sensitive TO USER ana;        DENY read ON LABEL hr TO ROLE sales-rep;
LABEL d140, d141 ADD finance;   LABEL d140 REMOVE finance;
LABEL WHERE section = pricing ADD finance [DRY RUN] [CONFIRM];
REMEMBER fact "…" LABELS (finance);   INSERT INTO deal (…) VALUES (…) LABELS (finance);
SELECT id, labels FROM memory WHERE labels ~ 'finance' LIMIT 20;      -- labels column visible to admins and to holders
EXPLAIN ACCESS d140 FOR kim;                                -- admin: why kim can/cannot see d140 (labels, grants, denies)
AS USER kim; CONTEXT FOR "Q4 renewals";                     -- admin preview of another user's view (journaled)
```

User and role statements need `admin`. They are written to the access config file with an atomic write plus a backup, and they are journaled.

## M6. Interfaces

- **HTTP/MCP:** every request resolves a principal from its token or JWT. stdio MCP uses `--user` together with the token in `BRAINDB_TOKEN_<USER>` or `--token-env`, or runs as admin when access is disabled.
  - `GET /api/whoami`.
  - `/api/state` and everything else returns the filtered view.
- **Publish:** `braindb publish --labels public,sales` **physically excludes** every node the label set cannot see, including vectors recomputed without them. Shipping a binary to partners cannot leak hidden data even by reverse engineering.
- **UI:**
  - top bar user menu (WHOAMI, switch token);
  - admins get an **Access** tab: users, roles, grants editor with validation, labels list with node counts, `EXPLAIN ACCESS` tool, and an "Access overlay" view mode in Graph 3D that hatches and colours restricted areas by label;
  - an admin "View as…" selector renders the brain exactly as another user sees it, with inaccessible areas switched off;
  - non-admins never see hidden nodes, labels they do not hold, or counts that include them;
  - Storybook stories: access tab, overlay, view-as, write denied.

## M7. Tests

- **Property test:** for random brains, random label assignments and random principals, every read API on the full brain as P equals the same API run by an admin on a copy with P-invisible nodes deleted. This covers context, all search modes (rows and scores), SELECT and aggregates, paths, suggest, discover, fill, entities, stats, layout, vectors, the status line and SSE events.
- **Write rules:** cannot touch invisible nodes; LABELS ⊆ write grants; deny overrides allow; section and kind narrowing.
- Other cases:
  - error messages identical for invisible and nonexistent ids;
  - publish with `--labels` contains no hidden strings (grep of the binary payload);
  - JWT verification with valid, expired, wrong-audience and tampered tokens;
  - performance: activation and search overhead with access enabled ≤ 10% vs disabled on the 100k bench.


---

# Addendum N: icons, no emoji

- **UI:** no emoji anywhere. All pictograms are SVG icons from one icon set, wrapped in a single `<Icon name=… size=… />` component with an accessible label or `aria-hidden`:
  - set: [Lucide](https://lucide.dev) via `lucide-react`, ISC license (compatible with Apache-2.0), exact version pinned and tree-shaken;
  - icons inherit `currentColor` and sizes 14/16/20 px, aligned to the 28 px row grid;
  - the app logo and favicon are an SVG mark, not an emoji;
  - the Storybook "Icons" story lists every icon used, with its name;
  - the kind, connector and entity-type glyphs in the 3D view and legend use the same SVG shapes.
- **Text outputs** (MCP, CLI, status lines, context XML/MD, logs, journal): no emoji. Use plain words or ASCII tags (`warn:`, `error:`, `pinned`, `masked`).
  - The compact symbols defined in this spec stay: `◆` concept, `⧉` connector, `⇄` supersede, `▲` boost, `▼` weaken, `~` assoc, `≈` similar, `→`.
  - They are plain Unicode symbols, not emoji, and each one is documented.
- **Docs and examples:** no emoji. Use the same documented symbols and words.
- A test fails the build if emoji code points (U+1F300–U+1FAFF, U+2600–U+27BF except the symbols listed above, U+2B50, U+2705, U+274C) appear in `internal/`, `cmd/`, `web/src/`, `docs/`, `skills/` or `examples/`.

---

# Addendum O: brain purpose and the admission pre-filter

Every database can declare its **purpose**: what the brain is for, what belongs in it, and what does not. The purpose is:
1. **an admission pre-filter**: every incoming item (`REMEMBER`, `INSERT`, `INGEST`, `IMPORT`, UI upload, MCP writes) is scored against it before it enters memory;
2. **guidance**: agents receive the purpose in the MCP instructions and in the context header, so they know what to store;
3. **documentation**: `SHOW PURPOSE`, the UI and `braindb config show` describe the database to people.

The filter is deterministic (embeddings + rules, no LLM inside) and explainable: each decision lists its reasons.

## O1. Configuration

```yaml
purpose:
  name: Tessaline sales brain
  description: >
    Knowledge the sales team needs to sell and renew NaCore battery leases: accounts and contacts, deals and pipeline,
    pricing and discount rules, product claims backed by lab evidence, SLAs, warranties and customer commitments.
  inScope:
    topics: ["accounts and contacts", "deals and pipeline", "pricing and discounts", "product claims and specs",
             "SLAs and warranties", "customer meetings and commitments"]
    concepts: [pricing, renewal, warranty]       # existing concept ids or labels
    kinds: [deal, account, fact, decision, task, question, note, file, chunk, record]
    sections: [facts, decisions, tasks, pricing, pipeline]
  outOfScope:
    topics: ["private matters unrelated to work", "lab internal procedures", "food, sports, entertainment"]
    entities: [card]                             # entity types (Addendum I) never admitted
  examples:
    accept: ["Marrow Creek asked for a 24-month warranty on NaCore 2", "Q4 commit is EUR 1.62M"]
    reject: ["Lunch menu for Friday", "Photo of the office cat"]
  admission:
    mode: enforce          # off | advise | enforce
    accept: 0.35           # score ≥ accept → admitted
    review: 0.20           # review ≤ score < accept → quarantine; below review → rejected
    apply: [remember, insert, ingest, import]
    exempt: [admin]        # roles that bypass the filter (their writes are still scored and journaled)
    chunks: file           # file: a file's chunks inherit the file-level decision; each: score every chunk
    records: table         # table: a CSV/JSON/SQL table is scored once (name + columns + a sample of rows) and its records inherit the decision; each: score every record
```

- Without a `purpose` block, the filter is off and the brain behaves as before.
- `purpose.description` alone (no topics or examples) is enough. It then becomes the only positive anchor.
- **Structured data is scored per table, not per row (`records: table`, the default).** Short rows such as an order line or a
  ticket carry too little text for a reliable per-item score. The table is scored once on its name, column names and up to
  20 sample rows, and every record inherits that decision. `each` is available for tables with long free-text rows.
- **Exempt roles** bypass the decision, but dry-runs and `CHECK` always show the score and the decision the filter *would*
  make. Previews are therefore meaningful for admins too.

## O2. Scoring

For an incoming item with text t (plus fields, kind, section and entities):

```
pos  = max( cos(v(t), centroid(description + inScope.topics + examples.accept)),
            max cos(v(t), v(topic_i)) over inScope.topics,
            max cos(v(t), concept vectors of inScope.concepts) )
neg  = max cos(v(t), v(x)) over outOfScope.topics ∪ examples.reject
base = clamp( pos − 0.6·max(0, neg − pos·0.8), 0, 1 )
bonus: +0.10 kind ∈ inScope.kinds · +0.05 section ∈ inScope.sections · +0.05 per explicit link/concept to admitted in-scope knowledge (max +0.15)
       · +0.05 when a typed entity of an in-scope record field matches
score = clamp(base + bonus, 0, 1)
hard rules: a forbidden entity type (outOfScope.entities) → rejected regardless; empty or whitespace text → rejected
```

- Vectors use the active embedding (`hash-256` or the configured model), so purpose and memory live in one space.
- Purpose vectors are cached and recomputed when the purpose or the embedding config changes.
- The score is reported with its parts: `{score, decision, pos, neg, bonus, topMatch, topConflict, rules[]}`.

## O3. Decisions and quarantine

| mode | behaviour |
|---|---|
| `off` | no scoring |
| `advise` | everything is stored; items under `accept` get the `purpose:low` flag, are listed in the status line (`warn: 2 items below purpose`) and in the import report |
| `enforce` | ≥ accept → stored; review band → **quarantine**; < review → **rejected**: not stored, reported with reasons, journaled with hash and size but not the content |

**Quarantine:**
- Quarantined items are stored in section `review` with the access label `quarantine` (Addendum M), whether or not access control is enabled.
- They are invisible to everyone except users with the `review` grant (admins have it). They never appear in contexts, search, fill or stats of others.
- Their edges are kept, but no activation passes through them until they are admitted.
- `ADMIT` moves an item to its intended section, removes the label and runs auto-linking.
- `REJECT` deletes it, with a reason in the journal.

**Overrides:**
- A writer with the `purpose-override` grant (admins have it) can force admission: `… OVERRIDE PURPOSE REASON '…'`.
- The reason is required and is journaled. The item gets the `purpose:override` flag.

**Batches:**
- The decision is per item.
- In a BDQL script, rejected items do not abort the transaction. They are reported as `rejected` results. With `STRICT PURPOSE` the script aborts instead.
- `$X` of a rejected `REMEMBER … AS X` is unset; using it later is an error that names the rejection.

## O4. Interfaces

**BDQL:**

```sql
SHOW PURPOSE;                                         -- name, description, scope, thresholds, mode, counters (admitted / quarantined / rejected)
CHECK "Lunch menu for Friday";                        -- score + decision + reasons, nothing stored
CHECK "NaCore 2 warranty request from Marrow Creek" AS fact IN facts;
REMEMBER fact "…" OVERRIDE PURPOSE REASON 'needed for audit';
SELECT id, text, purpose.score, purpose.reasons FROM quarantine ORDER BY purpose.score DESC LIMIT 20;
ADMIT r412, r413 [IN pipeline];   REJECT r414 REASON 'off-topic';
IMPORT '/data/inbox' DRY RUN;                         -- the report shows admitted / quarantine / rejected per file with reasons
```

Pseudo-columns `purpose.score`, `purpose.decision`, `purpose.flag`; pseudo-table `quarantine`.

**MCP:**
- `initialize` instructions start with: "This brain is for: <description>. Store only knowledge in scope: <topics>. Out of scope: <topics>."
- `brain_view` adds `<purpose>` (name + one-line description) to the context XML/MD header.
- `brain_run` results report rejected and quarantined writes with reasons, so the agent learns what not to store.

**HTTP:**
- `GET /api/purpose`, `PUT /api/purpose` (admin, validated, part of the config);
- `POST /api/purpose/check {text, kind?, section?, fields?, linkTo?, links?}` — scores an item that does
  not exist yet. A real write is scored **after** its links exist, and links to in-scope knowledge are
  worth up to +0.15, which spans the whole review band. `linkTo` (ids or concept labels, as `LINK TO` /
  `CONCEPTS` would give) and `links` (a plain count) let the preview reproduce that; without them it is
  a lower bound, exact only for a write that links to nothing;
- `GET /api/quarantine`, `POST /api/quarantine/admit {ids, section?}`, `POST /api/quarantine/reject {ids, reason}`;
- ImportReport files gain `{admitted, quarantined, rejected, reasons[]}`.

**UI:**
- Settings → **Purpose** editor (name, description, topics, examples, thresholds, mode) with a live **Check** box: paste any text and see score, decision and contribution bars for pos, neg and bonus, plus the top matching and conflicting topic;
- **Review queue** tab for reviewers: quarantined items with score, reasons, intended section, and Admit / Reject (with reason) / Admit into… actions;
- the import wizard shows decisions per file and item;
- the status bar shows the quarantine count for reviewers;
- Storybook stories for each.

## O5. Examples and tests

**Examples:**
- each example declares a purpose;
- import-demo contains a few deliberately off-topic raw files (a lunch menu note, a personal note, a pasted card number), so the dry-run report shows quarantine and rejection;
- each `queries.bdql` gets `SHOW PURPOSE`, `CHECK` and review statements.

**Tests:**
- deterministic scores;
- all three modes;
- the quarantine is invisible to non-reviewers (via the M2 property test);
- ADMIT and REJECT flows, override with reason, STRICT PURPOSE abort, a forbidden entity is always rejected;
- import dry-run counts;
- the purpose appears in the MCP instructions and the context header;
- scoring is cached and recomputed on a config change.
