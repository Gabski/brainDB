# Example: research-lab (Corvane Institute, electrolyte group)

A university lab is developing **EL-7**, a sodium-ion electrolyte: the baseline EL-0 (1.0 M NaPF6 in EC:DEC) plus 2 wt% FEC
and 0.5 wt% TMSB. Industry partner Tessaline Systems will use it in its NaCore 2 forklift packs.

The current goal is to **decide by 15 Dec 2026 whether EL-7 goes to 500-cycle pouch-cell validation**. Where things stand:

- At 25 °C, EL-7 clearly wins: 91.4% vs 82.1% capacity after 200 cycles.
- At 45 °C it fades faster: 84.0% vs 86.5% after 150 cycles.
- The open question is whether TMSB decomposes above 40 °C (`h187`) or HF from salt hydrolysis is to blame (`q140`).
  An additive ablation run (RUN-2620) is under way.
- Tessaline wants to print "3,000 cycles" and "−20 to 45 °C" on its spec sheet.

All people, institutions, e-mails (`*.example`) and numbers are fictional.

## Files

| path | what it is |
|---|---|
| `seed.json` | 67 hand-written memories, 17 concepts, 42 links, 5 supersede pairs, 3 pinned items, 51 archive phrases, 5 connectors |
| `braindb.yaml` | config: kinds, context defaults, import rules (`measurements.csv` → records in section `measurements`, protocol → procedures), patterns (custom `run_id`, `sample_id`, `cell_id`), privacy mask, connectors |
| `docs/protocol-v3.md` | test protocol v3 (prep, cell build, formation, cycling, EIS, gas, low temperature, data) |
| `docs/meeting-minutes-2026-11-19.md` | group meeting with results, discussion, decisions and actions; Tessaline on the phone |
| `docs/measurements.csv` | 36 rows: `cell_id, sample_id, run_id, electrolyte, condition, temperature_c, conductivity_mS_cm, capacity_retention_pct, cycles, coulombic_eff_pct, run_date, operator, note`. The means match the seed facts (91.4 / 82.1 / 84.0 / 86.5 / 78 / 0.42 ± 0.06 mL). |
| `docs/agent-workspace/state.json` | an agent's ctxc working context (7 items: statistics, draft answer to Tessaline, agent rule) |
| `templates/experiment-report.md`, `templates/sample-sheet.json`, `templates/weekly-status.yaml` | fill templates |
| `queries.bdql` | BDQL tour: `braindb run examples/research-lab/queries.bdql --db research-lab` |

Custom kinds:

| kind | prefix | section | half-life |
|---|---|---|---|
| `experiment` | `e` | `experiments` | 30 d |
| `hypothesis` | `h` | `hypotheses` | 45 d |
| `sample` | `p` | `samples` | 60 d |

Superseded pairs:

| old → new | change |
|---|---|
| `d120 → d121` | protocol v2 → v3 |
| `d122 → d123` | go/no-go on 30 Nov → 15 Dec |
| `f106 → f107` | pouch gas: 0.9 mL from a leaking seal → 0.42 mL with n=4 |
| `h185 → h186` | "FEC alone" → FEC + TMSB synergy |
| `p196 → p195` | batch B2 (41 ppm water) → B3 |

## Run

```bash
braindb seed examples/research-lab/seed.json -o brain.xml --ingest examples/research-lab/docs
braindb serve --brain brain.xml
braindb run examples/research-lab/queries.bdql --db research-lab
```

The focus is `d123` (go/no-go on 15 Dec), `f108` (45 °C fade) and `h187` (TMSB decomposition hypothesis),
with depth 2, threshold 0.2 and hop 0.85.

Expected sizes from a reference run before ingest, ±3 items:

| depth | lit memories | tokens |
|---|---|---|
| 1 | ~9 / 118 (8%) | ~380 |
| 2 | ~17 / 118 (14%) | ~760 |

After ingesting `docs/`, the slice at depth 2 is about 27 / 143, ~1,700 tokens.

## Focus and depth experiments

- **Depth 1 → 3.** Depth 1 holds the run, the question and the PI note. Depth 2 adds the ablation, pouch build, synergy hypothesis
  and Tessaline items. Depth 3 adds little: the slice is stable.
- **Focus `f104` (25 °C win).** This lights the other side of the story: RUN-2611, `x180`, the conductivity facts and protocol v3.
- **Focus `p195` (sample S-0457).** This follows sample traceability: KF titration `f110`, batch decision `d125`, superseded batch `p196`.
- **Show superseded on.** The leaking-seal gas value and the "FEC alone" hypothesis come back, faded.
- **Threshold 0.16.** The slice grows to about 28% through the `funding` and `temperature` concepts.

## Suggested searches

| mode | query | should rank high |
|---|---|---|
| hybrid | `45 °C fade TMSB decomposition` | `h187`, `q140`, `f108` |
| vector | `SEI resistance impedance` | `f111`, `h188` |
| tree | `Tessaline NaCore 2 cycle life claim` | `q144` → `f114` → `t137` |
| graph | `pouch gas leaking seal` | `f107`, `e192`, then `t131` via links |
| hybrid (after ingest) | `sample S-0457 water ppm` | `p195`, `d125`, `f110` |

Paths: `h187 → d123`, and `x183 → t130` (literature to ablation).

## Raw queries (compact syntax)

```
section:measurements temperature_c=45 sort:-capacity_retention_pct select:fields.cell_id,fields.electrolyte,fields.capacity_retention_pct
section:measurements electrolyte=EL-7 run_id=RUN-2611 select:fields.cell_id,fields.capacity_retention_pct,fields.coulombic_eff_pct
section:measurements conductivity_mS_cm>10 select:fields.sample_id,fields.temperature_c,fields.conductivity_mS_cm
kind:sample superseded:false sort:-prepared select:fields.sample_id,fields.batch,fields.water_ppm,fields.location
kind:experiment status=running select:fields.run_id,fields.operator,fields.start_date
kind:hypothesis superseded:false near:"high temperature fade"
run_date>=2026-11-15 operator~Petrescu format:md
```

The 45 °C query returns 8 rows: 4 EL-7 cells (83.1–84.6%) and 4 EL-0 cells (86.0–86.9%).

## Template fills

```bash
braindb fill --brain brain.xml examples/research-lab/templates/weekly-status.yaml --output yaml --sources
braindb fill --brain brain.xml examples/research-lab/templates/sample-sheet.json --output json
braindb fill --brain brain.xml examples/research-lab/templates/experiment-report.md
```

The values extraction should find:

| slot | value | source |
|---|---|---|
| decision date | 2026-12-15 | `d123` |
| retention at 25 °C / 45 °C | 91.4% / 84.0% | `f104`, `f108` |
| capacity at −20 °C | 78% | `f113` |
| PI | Hanne Solberg, hanne.solberg@corvane.example | `n150` |
| sample | S-0457, EL-7, batch B3, 250 mL, 14 ppm, prepared 2026-11-09 by Ravi Menon, glovebox shelf 2 | `p195` |
| water limit / shelf life | 20 ppm / 21 days | `s161` |
| glovebox regeneration due | 2026-12-01 | `f109` |
| running experiments | RUN-2614 (Ana Petrescu) and RUN-2620 (Ravi Menon) | experiments |

Per-cell rows come from the `measurements` records.

## Connectors (Addendum H)

| connector | type | joins |
|---|---|---|
| `H:partner-tessaline` | anchor `partner:Tessaline Systems` | the partner's requests and samples |
| `H:sample-s-0457` | anchor | the EL-7 B3 batch: sample, KF titration, batch decision and the runs that use it |
| `H:run-2614` | anchor | the 45 °C run, its result and the follow-ups |
| `H:go-no-go` | hub | the decision workstream |
| `H:sei-resistance` | alias | SEI resistance ↔ R_SEI ↔ interphase resistance ↔ SEI impedance |

The sales brain links into this one: `lab:f104`, `lab:f107`, `lab:f108`, `lab:f113`, `lab:d123`, `lab:e191`.
See `../federation/README.md`.

## Entities (Addendum I)

| entity type | examples in this data |
|---|---|
| units | `4.3 V`, `4.5 V`, `3.7 V`, `8.6 mS/cm`, `12.1 mS/cm`, `25 °C`, `45 °C`, `−20 °C`, `2 mAh/cm2`, `250 mL`, `14 ppm` |
| dates | `2026-11-09`, `15 Dec 2026`, `15.12.2026`, `18.11.2026`, `Q1 2027` |
| money | `€310,000`, `NOK 4.8M`, `USD 3.2k`, `1 200,00 zł` |
| percents | `91.4%`, `84.0%`, `2 wt%` |
| id codes | `RUN-2611`, `S-0457`, `C-0641`, `INV-2026-0311`, `NFR-2025-4471` |
| people with titles | `Dr. Hanne Solberg`, `PI Dr. Hanne Solberg` |
| one email written two ways | `hanne.solberg@corvane.example` / `Hanne.Solberg@corvane.example` |
| one phone written two ways | `+47 55 58 20 14` / `55 58 20 14` |
| urls | `https://corvane.example/protocols/v3` |
| organisations | Tessaline Systems AS, Separex sp. z o.o. |
| IBAN | `NO54 9999 0543 212`. Fictional, with a valid mod-97 checksum. |

Config: custom patterns `run_id`, `sample_id` and `cell_id`; `privacy.mask: [phone, iban]`; `connectors.entityAnchors: [email, phone]`.

## Access (Addendum M)

Unpublished results carry the label `ip-sensitive` (a patent filing is pending):

- the seed: 18 memories, i.e. the result facts `f104`–`f113`, all four hypotheses, the four experiments, the idea `i171` and the data files `x180`/`x181`;
- imports: `measurements.csv`, the meeting minutes and the agent workspace, through `access.defaults.import`;
- new hypotheses, through `access.defaults.kinds`.

The protocol, samples, tasks, questions and lab-ops facts stay public.

| user | role | sees |
|---|---|---|
| `ana` | researcher | everything (`ip-sensitive` read/write) |
| `tessaline` | partner (token `LAB_TOKEN`, used by the sales brain's source) | `ip-sensitive` facts, decisions and experiments; **no hypotheses** |
| `visitor` | none | public knowledge only |
| `admin` | admin | everything |

```sql
AS USER visitor; SEARCH "45 °C fade TMSB" LIMIT 5;   -- q140 (open question), d123 (go/no-go date), protocol items; no f108, no h187
AS USER ana;     SEARCH "45 °C fade TMSB" LIMIT 5;   -- h187, q140, f108 (84.0% vs 86.5%)
```

```bash
braindb fill --brain brain.xml --user visitor examples/research-lab/templates/weekly-status.yaml
#  missing (measured): headline.retention_25c, headline.retention_45c, running_experiments, hypotheses
#  (low_temp_capacity / sei_resistance fall back to unrelated public text, since the real values are ip-sensitive)
braindb fill --brain brain.xml --user ana examples/research-lab/templates/weekly-status.yaml
#  91.4%, 84.0%, 78%, 38 ohm, hypotheses h186 / h187 / h188
```

## Purpose and admission (Addendum O)

`braindb.yaml` declares the purpose **Corvane electrolyte lab brain** with:

- in-scope topics, concepts, kinds and sections;
- out-of-scope topics;
- `entities: [card]`;
- accept and reject examples;
- `admission: {mode: enforce, accept: 0.35, review: 0.20, records: table, chunks: file, exempt: [admin]}`.

Measured with `braindb run` (`CHECK`, real scorer, hash-256): the RUN-2620 result 0.44 → admitted, "Marrow Creek discount approval …" 0.15 → rejected, ana's notes "Conference trip to Ulm … poster" 0.28 and "Group Christmas dinner … Lukas books the table" 0.24 → quarantine. Out-of-scope topics include conference travel and lab social events. The docs and `measurements.csv` are admitted (estimated 0.52–0.77).
`queries.bdql` section 19 runs `SHOW PURPOSE`, two `CHECK`s, writes that land in quarantine or are rejected, the review queue
(`SELECT … FROM quarantine`), `ADMIT`, `REJECT` and one `REMEMBER … OVERRIDE PURPOSE REASON '…'`.
Borderline scores can differ slightly with the configured embedding.
