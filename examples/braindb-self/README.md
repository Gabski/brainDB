# Example: braindb-self (brainDB's own project brain)

brainDB holding the knowledge needed to build brainDB. The decisions and their reasoning, the spec addenda, what is
built against what is only specified, the measured retrieval quality and the conditions it was measured under, the
CLI surface, the roadmap, and the practice of using a graph memory as an agent's knowledge base.

Nothing here is fictional. Every fact, decision and number comes from this repository: `CHANGELOG.md`,
`docs/SPEC.md`, `docs/BENCHMARKS.md` and the source. If the repository changes, this example is wrong and should be
updated — which is the point of an example that documents its own project.

**Why it exists.** An agent working on brainDB otherwise has to re-read about 3,400 lines of documentation to learn
that admission defaults to `advise`, that decay has floors since 0.0.2, and that the default embedding engine fails
on paraphrases. Here it reads a 14-memory slice, about 950 tokens, and starts.

## Files

| path | what it is |
|---|---|
| `seed.json` | 95 hand-written memories, 18 concepts, 72 links, 3 supersede pairs, 4 pinned items, 40 archive phrases, 5 connectors |
| `braindb.yaml` | config: 3 custom kinds, context defaults, import rules (two CSVs → records, `adr/*.md` → section `spec`), purpose with in/out-of-scope, custom patterns (`addendum`, `release`, `eval_run`) |
| `docs/adr/0001…0005.md` | five decision records: SQLite, decay floors, advise over enforce, pluggable embeddings, measure before claiming |
| `docs/agent-playbook.md` | the practice: context is a slice not a dump, kinds carry policy, supersede never delete, connectors carry the joins, judge retrieval before trusting it, what an agent writes at the end of a turn |
| `docs/roadmap-0.0.3.md` | what 0.0.3 is: one measurement, one migration, one promise not to break anything quietly |
| `docs/contributing-conventions.md` | repository conventions, including the ones CI enforces |
| `docs/eval-runs.csv` | 12 rows: the measured retrieval table from `docs/BENCHMARKS.md`, one row per mode and bucket |
| `docs/cli-surface.csv` | 19 rows: every `braindb` command with its status, surface, summary and key flags |
| `docs/agent-workspace/state.json` | an agent's ctxc working context: 7 items around the open benchmark question |
| `templates/release-notes.md`, `templates/feature-status.yaml`, `templates/agent-onboarding.json` | fill templates |
| `eval.json` | 24 hand-judged queries, 12 lexical and 12 paraphrase |
| `queries.bdql` | BDQL tour: `braindb run examples/braindb-self/queries.bdql --db braindb-self` |

Custom kinds:

| kind | prefix | section | half-life | floor |
|---|---|---|---|---|
| `addendum` | `a` | `spec` | 365 d | 0.85 |
| `command` | `c` | `cli` | 180 d | 0.70 |
| `benchmark` | `m` | `measurements` | 90 d | 0.60 |

Superseded pairs, which are the three behaviour changes of 0.0.2:

| old → new | change |
|---|---|
| `d140 → d141` | purpose admission enforces by default → `advise` by default |
| `d142 → d143` | every kind fades to zero → every kind has a floor |
| `d144 → d145` | hand-rolled storage engine → SQLite as the single dependency |

## Run

The short path, seed plus ingest. The two CSVs land as `record` in sections named after the file, because the
import rules in `braindb.yaml` are not read on this path:

```bash
braindb seed examples/braindb-self/seed.json -o brain.xml --ingest examples/braindb-self/docs
braindb serve --brain brain.xml
```

The full path, where the config applies and the CSV rows become `command` and `benchmark` memories in the sections
the rules name:

```bash
braindb db create braindb-self --data-dir data
cp examples/braindb-self/braindb.yaml data/braindb-self/
cp -r examples/braindb-self/templates data/braindb-self/
braindb seed examples/braindb-self/seed.json -o data/braindb-self/brain.xml
braindb import --db braindb-self --data-dir data examples/braindb-self/docs
braindb serve --data-dir data --db braindb-self
braindb run examples/braindb-self/queries.bdql --db braindb-self
```

The focus is `q180` (does a real embedding model fix the paraphrase collapse), `d151` (the default engine question
is reopened) and `t161` (SQLite behind the storage interface).

## Sizes from a reference run

Seed only: 158 nodes, 498 edges, 135 memories. After importing `docs/`: 215 memories, 11 files, 38 records.

| brain | depth | threshold | lit memories | tokens |
|---|---|---|---|---|
| seed | 1 | 0.3 | 12 / 135 (9%) | ~827 |
| seed | 2 | 0.3 | 31 / 135 (23%) | ~2,278 |
| seed + docs | 1 | 0.3 | 14 / 215 (7%) | ~946 |
| seed + docs | 2 | 0.3 | 34 / 215 (16%) | ~2,446 |

## The threshold lesson

This brain uses threshold **0.3**, not the 0.2 of the other examples. It is worth seeing why, because it is the
most common mistake when a brain is built by hand:

| depth | threshold | lit memories | tokens |
|---|---|---|---|
| 1 | 0.2 | 15 / 135 | ~961 |
| 2 | 0.2 | **63 / 135 (47%)** | ~4,015 · over the 4k warning |
| 2 | 0.25 | 42 / 135 (31%) | ~2,895 |
| 2 | 0.3 | 31 / 135 (23%) | ~2,278 |
| 3 | 0.3 | 31 / 135 | ~2,278 (depth 2 is reached and nothing more passes) |

72 hand-written links over 95 memories is a dense graph, and a dense graph conducts. At 0.2 the same focus lights
half the brain and spends 4,000 tokens on it; at 0.3 it answers the same questions with 2,278. Hand-linking is
worth doing, and the threshold is the price of doing it.

## Suggested searches

Verified against the seeded brain:

| mode | query | top hits |
|---|---|---|
| hybrid | `paraphrase recall collapse` | `q180`, `f110`, `f112` |
| vector | `decay floor durable kinds` | `f105`, `d143` |
| tree | `SQLite crash safety dependency` | `f128`, `d145`, `t161` |
| graph | `agent loop remember link supersede` | `s210`, `q187` |
| hybrid (after ingest) | `why advise instead of enforce` | `d141`, then the ADR 0003 chunk and file |

And the counter-example, which is the finding this brain documents:

| mode | query | result |
|---|---|---|
| graph | `what the agent writes at the end of a turn` | `f121`, `x222`, `q187` — **not** `s210`, which is the memory that answers it |

The second query means the same thing as the fourth row above and shares no stems with the target, so the built-in
`hash-256` engine has nothing to match. This is not a tuning problem; it is the default engine's mechanism.

## Retrieval measured on this brain

`eval.json` holds 24 hand-judged queries over the seeded brain, split the same way as the sales-company gold set.

```bash
braindb seed examples/braindb-self/seed.json -o brain.xml
braindb eval --brain brain.xml --gold examples/braindb-self/eval.json
```

Measured 20 Nov 2026 · brainDB 0.0.2 · Go 1.27.4 · `hash-256` (dim 256) · 158 nodes · k = 10.

**lexical** (12 queries)

| mode | recall@5 | recall@10 | MRR | nDCG@10 | queries with no hit |
|---|---|---|---|---|---|
| `vector` | 0.90 | 0.97 | 0.86 | 0.85 | 0/12 |
| `hybrid` | 0.96 | 1.00 | 0.86 | 0.87 | 0/12 |
| `graph` | 0.94 | 0.97 | **0.96** | **0.92** | 0/12 |
| `tree` | 0.97 | 1.00 | 0.86 | 0.88 | 0/12 |

**paraphrase** (12 queries)

| mode | recall@5 | recall@10 | MRR | nDCG@10 | queries with no hit |
|---|---|---|---|---|---|
| `vector` | 0.12 | 0.17 | 0.06 | 0.09 | **9/12** |
| `hybrid` | 0.08 | 0.21 | 0.06 | 0.10 | **9/12** |
| `graph` | 0.17 | 0.25 | 0.14 | 0.15 | **8/12** |
| `tree` | 0.08 | 0.25 | 0.04 | 0.09 | **9/12** |

**all 24 queries**

| mode | recall@5 | recall@10 | MRR | nDCG@10 | queries with no hit |
|---|---|---|---|---|---|
| `vector` | 0.51 | 0.57 | 0.46 | 0.47 | 9/24 |
| `hybrid` | 0.52 | 0.60 | 0.46 | 0.48 | 9/24 |
| `graph` | 0.56 | 0.61 | **0.55** | **0.53** | 8/24 |
| `tree` | 0.53 | 0.62 | 0.45 | 0.48 | 9/24 |

**This is a replication, on a different domain.** The sales-company set measured graph MRR 0.57 against vector 0.46
over 32 queries and 144 nodes; this set measures 0.55 against 0.46 over 24 queries and 158 nodes, with the same
pattern in both buckets: graph activation wins and never misses a lexical query, and three quarters of the
paraphrases return nothing relevant whatever the mode. Two domains, two gold sets, the same two conclusions.

See [docs/BENCHMARKS.md](../../docs/BENCHMARKS.md) for the first measurement and the caveats that apply to both.

## Template fills

```bash
braindb fill --brain brain.xml examples/braindb-self/templates/feature-status.yaml --output yaml --sources
braindb fill --brain brain.xml examples/braindb-self/templates/agent-onboarding.json --output json
braindb fill --brain brain.xml examples/braindb-self/templates/release-notes.md
```

`agent-onboarding.json` is the one to read first: it is the brief an agent should be given before it changes
anything in this repository. From a reference run it resolves to, among others:

| slot | value | source |
|---|---|---|
| `project.version` | 0.0.2 | `f101` |
| `model.node_classes` | knowledge nodes cost tokens, connector nodes do not | `f106` |
| `conventions.ci` | gofmt, go vet, go test -race, go build, then the web job | `d149` |
| `do_not_break.admission_mode` | purpose admission defaults to advise, not enforce | `d141` |
| `do_not_break.decay_defaults` | decision 365 d floor 0.80, fact 180 d floor 0.70, … | `f105` |
| `current_work.blocking_question` | does a real embedding model fix the paraphrase collapse | `q180` |

`feature-status.yaml` needs the imported database: its `planned[]` slot reads the `command` memories that come from
`docs/cli-surface.csv` under the config rules, so on a seed-only brain that one slot is reported as missing.

Two limits worth knowing before writing slots of your own. Slots filter by kind, section, concept, scope and score,
but **not** by `superseded` or by a field value, and a list slot *ranks*, it does not filter: `planned[]` asks for
"commands with status planned" and gets `migrate` first and working commands after it. When the answer has to be
exact, use BDQL — `WHERE kind = decision AND superseded` (statement 4) and `WHERE fields.status = 'planned'`
(statement 7) in `queries.bdql`. Use `field=name` to read a value straight out of a record, as the measurement
bullets in `release-notes.md` do.

## What this example is for

The pattern generalises to any project that wants its knowledge readable by an agent:

1. **Decisions as memories, with the reversed ones kept.** `SUPERSEDE`, never delete. The old decision is the
   explanation of the new one.
2. **Measurements as records**, each carrying its engine, node count, k and date. A number without its conditions
   is not a result.
3. **The spec as its own kind**, with a long half-life and a high floor, so reference knowledge never ranks out.
4. **State of play as tasks and questions**, floor 0, so it disappears when it stops being true.
5. **A purpose in `advise` mode**, which doubles as the instruction an agent receives about what belongs here.
6. **A gold set with a paraphrase bucket**, run after every change to kinds, links or engine.

`docs/agent-playbook.md` is that argument in full, and it is in the brain, so an agent can be handed the playbook
by the system the playbook describes.

## Two bugs this example found

Building it exercised paths the other examples did not, and found two:

- **An empty `purpose.outOfScope.entities` list forbade everything.** `patterns.ExtractTypes` treats an empty type
  list as "every type", so any item carrying a date, an amount or a version was rejected outright — in `advise`
  mode as well, because a forbidden entity bypasses the advise branch. Every other shipped example happens to set
  `entities: [card]`, which hid it. Fixed in `internal/brain/purpose.go`; this config deliberately leaves the list
  out, so the path stays covered.
- **The clause splitter cut tokens containing a full stop.** Template fill split candidate text on every `.`, so a
  filled slot reported `A brain is stored today as one brain` and `MRR 0` instead of `0.57`. A full stop now ends a
  clause only when whitespace follows. Fixed in `internal/brain/fill.go`.

Both have regression tests in `internal/brain/examplecfg_test.go`.
