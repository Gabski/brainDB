# Example: federation (sales brain ↔ lab brain)

Two brainDB servers run side by side:

- **sales**: Tessaline Systems, `../sales-company`. Sells NaCore sodium-ion forklift packs.
- **lab**: Corvane Institute, `../research-lab`. Develops the EL-7 electrolyte used in NaCore 2.

The sales spec sheet for NaCore 2 (`f112`: 3,000 cycles, −20 to 45 °C, 3.7 V nominal / 4.3 V max) depends on the lab's results.
The key account asks for a 3,000-cycle guarantee at 45 °C (`q144`). Only the lab knows the answer: EL-7 fades *faster* at 45 °C
(`lab:f108`), and the go/no-go on pouch validation is 15 Dec 2026 (`lab:d123`).

## How the brains are joined

`sales-company/seed.json` declares the source and six external links:

```json
"sources": [{"name": "lab", "uri": "http://lab:7791", "tokenEnv": "LAB_TOKEN", "weight": 0.7, "ttl": 300, "mode": "live", "federateDepth": 1}],
"externalLinks": [["f112", "lab:f104", 0.8], ["q144", "lab:f108", 0.85], ["d129", "lab:d123", 0.75],
                  ["f113", "lab:f107", 0.6], ["t137", "lab:e191", 0.7], ["q145", "lab:f113", 0.7]]
```

| sales memory | → lab memory | why |
|---|---|---|
| `f112` NaCore 2 draft spec (3,000 cycles at 25 °C) | `f104` EL-7 91.4% after 200 cycles at 25 °C | evidence behind the cycle-life claim |
| `q144` 3,000-cycle guarantee at 45 °C? | `f108` EL-7 fades faster at 45 °C (84.0% vs 86.5%) | the answer: not yet |
| `d129` no guarantee above 35 °C until 15 Dec | `d123` go/no-go moved to 15 Dec 2026 | the date the sales rule waits for |
| `f113` Plant 7 failures from cell swelling | `f107` pouch gas 0.42 ± 0.06 mL with EL-7 | gas evolution evidence for the new cells |
| `t137` ask the lab for RUN-2614 data | `e191` RUN-2614, 45 °C cycling | the run itself |
| `q145` usable capacity at −20 °C? | `f113` EL-7 delivers 78% at −20 °C | the cold-store answer |

**Bridge connector.** All of these go through one bridge connector, `H:bridge-lab` (Addendum H, `connectors.bridges: true`). Every
sales↔lab edge is then easy to see, audit or remove in one place. The bridge is transparent: activation passes through it without
spending a depth level. On the lab side, `H:partner-tessaline` anchors everything the lab knows about the partner.

The lab mirrors the topic: `f114` (partner needs), `q144` (which claim can Tessaline print?), `t137` (send a validated summary),
`p198` (NaCore 2 prototype cells), and the minutes of 19 Nov, where Jonas Brandt asked on the phone.

## Run

This file is its own compose project. Each service starts from a clean data directory and installs one example before it listens, so the lab user `tessaline` is loaded with `LAB_TOKEN`:

```bash
export LAB_TOKEN=$(openssl rand -hex 16)      # optional; the lab then requires this token, and sales sends it
docker compose -f examples/federation/docker-compose.federation.yml down -v
docker compose -f examples/federation/docker-compose.federation.yml up -d --build
# sales UI: http://127.0.0.1:7791   lab UI: http://127.0.0.1:7792
docker compose -f examples/federation/docker-compose.federation.yml exec -T sales \
  braindb run --data-dir /data --db sales-company /opt/braindb/federation/queries.bdql
```

The compose file does three things:

- installs `research-lab` and `sales-company` into separate data directories (`BRAINDB_INSTALL`);
- gives both services `LAB_TOKEN` (the token is never written to the brain; only `token-env="LAB_TOKEN"` is);
- makes `sales` wait for a healthy `lab`, and mounts this folder read-only at `/opt/braindb/federation`.

Without Docker, start the lab on 7792 and point the source at it:

```bash
braindb seed examples/research-lab/seed.json -o lab.xml --ingest examples/research-lab/docs
braindb serve --brain lab.xml --port 7792 &
braindb seed examples/sales-company/seed.json -o sales.xml --ingest examples/sales-company/docs
braindb source add lab http://127.0.0.1:7792 --token-env LAB_TOKEN --weight 0.7 --brain sales.xml
braindb serve --brain sales.xml
```

## Queries that only succeed through federation

Run these against the **sales** brain. Without the `lab` source they return only the question, never the evidence.

| query | through federation | without it |
|---|---|---|
| `SEARCH "cycle life guarantee at 45 °C" SOURCES lab;` | `q144`, `d129`, plus `lab:f108` (84.0% vs 86.5%), `lab:e191` (RUN-2614) and `lab:d123` (go/no-go), scores × 0.7 | `q144`, `d129`, `f112` only |
| `CONTEXT FOCUS q144 DEPTH 2 SOURCES lab;` | `lab:f108` enters at depth 1 through the bridge, then `lab:e191` and `lab:d123` (`federateDepth` 1), shown with `route="H:bridge-lab"` | a stub of `lab:f108` (cached text), no further lab nodes |
| `SELECT … FROM memory@lab WHERE kind = experiment;` | RUN-2611, 2614, 2617, 2620 | error: unknown source |
| `SELECT fields.electrolyte, AVG(fields.capacity_retention_pct) FROM memory@lab WHERE section = 'measurements' GROUP BY fields.electrolyte, fields.condition;` | per-condition means from the lab CSV (EL-7 25 °C 91.4, 45 °C 84.0, −20 °C 78.0; EL-0 82.1 / 86.5) | none |
| `FILL … "capacity retention after 150 cycles at 45 °C:percent\|scope=federated"` | 84.0% from `lab:f108` | `missing` |
| `CONTEXT FOR "usable capacity at −20 °C for the cold store" SOURCES *;` | `q145` → `lab:f113` (78%) | `q145`, `q909` |

**Access across brains (Addendum M).** The sales brain calls the lab with `LAB_TOKEN`, which the lab maps to its user `tessaline`
(role `partner`). That user can read `ip-sensitive` facts, decisions and experiments, but **not hypotheses**. So `lab:h187` (the TMSB
decomposition hypothesis) never reaches the sales brain, while `lab:f108` and `lab:e191` do. The external links and the bridge only point
at nodes the partner can see. On the sales side, local users see remote stubs only when their own clearance allows:
`AS USER grokbot; SEARCH … SOURCES lab;` returns no `lab:…` rows.

**Purpose: each side filters its own writes (Addendum O).** Sources are read-only, so the sales brain never writes into the lab.
Each brain checks only its own incoming items against its own `purpose`:

- the sales purpose admits product claims backed by lab evidence: `CHECK "NaCore 2 keeps 84.0% … (lab RUN-2614)"` ≈0.50;
- it quarantines lab-internal operations: `CHECK "Cycler channel 14 is noisy …"` ≈0.31;
- the lab purpose lists "sales pipeline, quotes and discounts" as out of scope.

Federated reads (context, search, SELECT on `memory@lab`) are not admissions and are never filtered by the reader's purpose.

Entities connect the two brains too. `jonas.brandt@tessaline.example` appears in the sales account `k196` and in the lab minutes
(Jonas Brandt joined by phone), so `RESOLVE email MIN 2 DRY RUN` over `memory@*` proposes one anchor across both brains.

**Status in this build:** `queries.bdql` Part A (declared source, external links and stubs, the bridge connector, local context, search, fill and purpose checks) runs green against the sales brain. Part B (remote transport: `SOURCES`, `memory@lab` / `memory@*`, `SOURCE SYNC`, `scope=federated`) is commented out under `-- requires: federation (planned)`. Today `SOURCE SYNC` errors, and `SOURCES` / `memory@lab` answer from local memory only. The table above describes the planned behaviour.

See `queries.bdql` in this folder for the full script (`braindb run examples/federation/queries.bdql --db sales-company`): context, search, SELECT on `memory@lab` and `memory@*`, external links,
the bridge connector and federated fills.
