# Example: sales-company (Tessaline Systems)

Tessaline Systems leases sodium-ion **NaCore** forklift and AGV battery packs to food plants and cold stores.
It is the middle of November 2026, and the sales team has two goals:

1. **Close Q4 2026** at or above the €1.62M commit. The target is €1.9M, and €1,219,500 is already closed-won.
   The Haldane Beverages expansion has just slipped to Q1.
2. **Renew Marrow Creek Foods**, the largest account (ARR €480,000, 212 packs across 14 plants, contract ends 31 Dec 2026),
   on a 3-year deal at €465,000 per year with a 6% multi-year discount, by 19 Dec 2026.

There are complications. A competitor (Grayfield Power, LFP packs at €159 per pack per month) is in the procurement round.
The champion is leaving. Marrow Creek also wants a 3,000-cycle guarantee for the new **NaCore 2** packs in a 45 °C bakery,
and only the partner lab (see `../research-lab` and `../federation`) can answer that.

All names, companies, e-mails (`*.example`) and numbers are fictional.

## Files

| path | what it is |
|---|---|
| `seed.json` | 64 hand-written memories, 17 concepts, 40 links, 5 supersede pairs, 3 pinned items, 51 archive phrases, 5 connectors, 1 source (`lab`), and 6 external links into the lab brain |
| `braindb.yaml` | config: kinds, context defaults, import rules (`pipeline.csv` rows become `deal` records), patterns (custom `sku`, `opportunity`, `invoice`; FX rates), privacy mask, connectors (incl. `entityAnchors`), source `lab` |
| `docs/sales-playbook.md` | deal review (MEDDICC), forecast categories, Grayfield battlecard, NaCore 2 claim rules (unlabelled) |
| `docs/pricing-policy-2026.md` | 2026 price list, discount authority, margins, current quotes; imported into section `pricing` with label `finance` |
| `docs/marrow-creek-call-notes-2026-11-19.md` | the latest renewal call |
| `docs/pipeline.csv` | 25 opportunities: `opportunity, deal, account, stage, value, owner, close_date, probability, forecast_category, product, next_step` |
| `docs/agent-workspace/state.json` | an agent's ctxc working context (9 items with `refs` to seed ids). It sits in a subfolder because a directory that holds `state.json` is treated as a ctxc workspace and its other files are skipped. |
| `templates/account-brief.yaml`, `templates/quote.xml`, `templates/deal-review.md` | fill templates (Addendum A4) |
| `queries.bdql` | BDQL tour: `braindb run examples/sales-company/queries.bdql --db sales-company` |

Custom kinds: `deal` (prefix `g`, section `pipeline`, half-life 21 d) and `account` (prefix `k`, section `accounts`, half-life 90 d).
Superseded pairs: 2025 → 2026 price list (`f106→f107`), 2-year → 3-year offer (`d120→d121`), €1.75M → €1.62M commit (`d122→d123`),
15% → 10% discount authority (`d124→d125`), and Haldane closing in Nov → Feb (`g190→g192`).

## Run

```bash
braindb seed examples/sales-company/seed.json -o brain.xml --ingest examples/sales-company/docs
braindb serve --brain brain.xml            # http://127.0.0.1:7791
braindb run examples/sales-company/queries.bdql --db sales-company
```

The focus is `d121` (renewal offer), `d123` (Q4 commit) and `g191` (Marrow Creek renewal deal). The context uses
depth 2, threshold **0.2** and hop 0.85. The threshold is raised from the default 0.16 because this brain is fresh and densely
linked: at 0.16 about 30% of memory lights up.

Expected sizes from a reference run of the activation rule before ingest. Archive placement is random, so expect ±3 items.

| depth | lit memories | tokens |
|---|---|---|
| 1 | ~11 / 115 (10%) | ~540 |
| 2 | ~20 / 115 (17%) | ~950 |

After ingesting `docs/`, the slice at depth 2 is about 31 / 147, ~1,900 tokens. The agent workspace items reference the focus.

## Focus and depth experiments

- **Default focus, depth 1 → 2.** Depth 1 shows the offer, proposal and forecast items. Depth 2 adds procurement, SLA and stakeholder
  items through the concepts `renewal`, `forecast` and `discount`.
- **Focus `q144` (NaCore 2 at 45 °C).** You get `d129` (no guarantee above 35 °C), `t137` and `f112`. With federation, the lab's
  45 °C result comes in too.
- **Focus `f105` (Grayfield offer).** This shows the competitive cluster: `t132`, `q141`, `i171`, `g193` and `k198`.
- **Show superseded on.** The old offer, price list, commit and discount rule come back, faded and linked to their replacements.
- **Threshold 0.16 vs 0.25.** Compare how the slice grows and shrinks around `A:renewal`.

## Suggested searches

| mode | query | should rank high |
|---|---|---|
| hybrid | `renewal 3-year offer discount` | `d121`, `x180`, `g191` |
| vector | `Grayfield Power competitor price` | `f105`, `t132`, `q141` (shows the rank lift from enriched vectors) |
| tree | `forecast commit Haldane slip` | `d123` → `g192` → `n153` / `k197` |
| graph | `Plant 7 swelling SLA credit` | `d126`, `f113`, then `f108` via links |
| hybrid (after ingest) | `NaCore 2 cycle life at 45 °C` | `d129`, the playbook chunk "Product claims", `q144`, `f112` |

Paths: `q141 → d121` (competitor price to offer), `t137 → d123` (lab request to forecast).
Suggest on `q144` should propose the call-notes chunk and `t136`.

## Raw queries (compact syntax)

```
kind:deal stage=negotiation sort:-value select:id,fields.account,fields.value,fields.owner
kind:deal superseded:false close_date<=2026-12-31 value>50000 sort:close_date
kind:deal owner~kim sort:-value limit:10
kind:account sort:-arr select:fields.account,fields.owner,fields.renewal_date,fields.arr
concept:renewal kind:question,task lit:true
section:pipeline forecast_category=commit sort:-value format:md
"Grayfield" near:"competitor price" limit:5
```

The first query returns `g191` (Marrow Creek, 1,395,000) plus the `negotiation` rows from `pipeline.csv`
(OPP-4471, OPP-4455 Oakhurst, OPP-4463 Varga, OPP-4490 Selby).

## Template fills

```bash
braindb fill --brain brain.xml examples/sales-company/templates/account-brief.yaml --output json --sources
braindb fill --brain brain.xml examples/sales-company/templates/quote.xml --output xml
braindb fill --brain brain.xml examples/sales-company/templates/deal-review.md
```

The values deterministic extraction should find, with their sources:

| slot | value | source |
|---|---|---|
| account name / owner / email | Marrow Creek Foods / Jonas Brandt / jonas.brandt@tessaline.example | `k196` |
| renewal date | 2026-12-31 | `k196` (`fields.renewal_date`), `f101` |
| ARR | €480,000 | `f101` |
| renewal offer / price / discount | 3-year, €465,000 per year, 6% | `d121` |
| target close | 2026-12-19 | `g191` |
| procurement contact | elin.dahl@marrowcreek.example | `t130` |
| pack price per month | €189 | `f107` (the superseded `f106` €178 must lose) |
| competitor | Grayfield Power, €159 per pack per month | `f105` |
| high-temperature restriction | no cycle-life guarantee above 35 °C | `d129` |

With entity-aware slots, the account name may come back as `Marrow Creek Foods GmbH` (org with suffix `GmbH`).
Slots with weak evidence, such as `risks[].evidence`, may come back in `missing`. That is expected: the agent fills them and stores
the result with `REMEMBER`.

## Connectors (Addendum H)

| connector | type | joins |
|---|---|---|
| `H:customer-marrow-creek` | anchor `account:Marrow Creek Foods` | 14 memories about the account |
| `H:customer-haldane` | anchor | Haldane deals, account and DPA note |
| `H:q4-close` | hub | the Q4 forecast workstream |
| `H:arr` | alias | ARR ↔ annual recurring revenue ↔ annual contract value ↔ ACV |
| `H:bridge-lab` | bridge `source:lab` | the NaCore 2 claim items and their lab evidence (`lab:f104`, `lab:f108`, `lab:d123`, …) |

Try `SEARCH "ACV of the key account"` (alias expansion) and `SELECT … WHERE route = 'H:customer-marrow-creek'`.
For the federated setup, see `../federation/README.md`.

## Entities (Addendum I)

The data is written to give the pattern library real work:

| entity type | examples in this data |
|---|---|
| dates | `2026-12-19`, `31 Dec 2026`, `14.02.2027`, `25.11.2026`, `Q1 2027` |
| money | `€465,000`, `46 200,00 zł`, `1 450 000 NOK`, `USD 3.2k` |
| percents | `6%`, `41%`, `99.4%` |
| units | `48 V`, `3.7 V`, `4.3 V`, `45 °C` |
| emails, one person written two ways | `jonas.brandt@tessaline.example` / `Jonas.Brandt@tessaline.example`; `elin.dahl@…` / `Elin.Dahl@MarrowCreek.example` |
| phones, one number written two ways | `+47 22 48 91 30` / `22 48 91 30`; `+49 40 3070 5521` / `+49 (0)40 3070-5521` |
| urls | `https://tessaline.example/deals/OPP-4471`, `marrowcreek.example` |
| organisations | Marrow Creek Foods GmbH, Haldane Beverages sp. z o.o., Ostler Dairy AS, Grayfield Power Ltd, Tessaline Systems AS |
| id codes | `SKU-48100`, `SKU-48200`, `OPP-4471`, `INV-2026-0187` |
| VAT and IBAN | `DE999000121`, `PL9990001232`, `NO 998 877 660 MVA`, `NO51 9999 0123 450`. All are fictional, and their checksums are valid so the validators accept them. |

Config:

- `patterns`: locales en, nb, de, pl; default region NO; `dateOrder: dmy`; FX rates for EUR, USD, NOK and PLN;
- `privacy.mask: [phone, iban]`;
- `connectors.entityAnchors: [email, phone, vat]`.

Try these statements:

```sql
EXTRACT FROM "Call Elin Dahl (+49 40 3070 5521) before 15.12.2026 about the €465,000 offer";
RESOLVE email MIN 2 DRY RUN;
RESOLVE phone MIN 2 DRY RUN;
```

## Access (Addendum M)

`braindb.yaml` turns access control on. Labels on the seed and on imports:

| label | where | count |
|---|---|---|
| `finance` | section `pricing`: price lists `f106`/`f107`, offers `d120`/`d121`, discount rules `d124`/`d125`, `x180`, `x181`, `pricing-policy-2026.md`; also margins `f109`, margin question `q141`, deal desk task `t131`, invoice `f116`, bank details `f117` | 13 |
| `internal` | pipeline and accounts sections, forecast items (`f104`, `d122`, `d123`), `f101`, `d126`, call notes, `pipeline.csv`, agent workspace | 13 + imports |
| `hr` | `n154` (parental leave), `n155` (commission plan) | 2 |
| `customer:ostler` | `k198`, `g193` (Ostler Dairy evaluation under NDA) | 2 |

| user | role | sees |
|---|---|---|
| `kim` | sales-rep | everything except `hr`; `finance` only in section `pricing` |
| `ilse` | analyst | unlabelled + `finance` + `internal` |
| `grokbot` | none, `dbs: [sales-company]` | unlabelled (public) knowledge only |
| `admin` | admin | everything |

The same query as two users:

```sql
AS USER grokbot; SEARCH "discount for Marrow Creek" LIMIT 5;
--  i170 (free Telemetry pilot idea), q140 (3-year term question), t130 (send proposal), f103 (3 quotes rule) …
--  no d121 / d125 / x180 / g191: they do not exist in grokbot's view, and the scores do not depend on them.
AS USER kim;     SEARCH "discount for Marrow Creek" LIMIT 5;
--  d121 (3-year, €465,000, 6%), d125 (deal desk above 10%), x180 (proposal v3), g191 (renewal deal), i170
```

Fill makes the difference visible:

```bash
braindb fill --brain brain.xml --user grokbot examples/sales-company/templates/account-brief.yaml --output json
#  missing (measured): account.name, owner, owner_email, renewal_date, plants, renewal.annual_price, target_close, open_deals, next_steps
#  (arr / discount fall back to unrelated public figures: the €250,000 ARR threshold in the playbook, an 8% campaign in the archive)
braindb fill --brain brain.xml --user kim examples/sales-company/templates/account-brief.yaml --output json
#  arr €480,000 (f101), annual_price €465,000 (d121), discount 6%, owner Jonas Brandt; nothing from hr
```

`kim` cannot see `f109` (41% margin): it is `finance` outside section `pricing`. So `q141`'s margin question resolves only for `ilse`.
`queries.bdql` section 18 covers `WHOAMI`, `SHOW grants FOR kim`, `EXPLAIN ACCESS`, `AS USER`, `LABEL … DRY RUN` and `REMEMBER … LABELS (…)`.

## Purpose and admission (Addendum O)

`braindb.yaml` declares the purpose **Tessaline sales brain** with:

- in-scope topics, concepts, kinds and sections;
- out-of-scope topics;
- `entities: [card]`;
- accept and reject examples;
- `admission: {mode: enforce, accept: 0.35, review: 0.20, records: table, chunks: file, exempt: [admin]}`.

Measured with `braindb run` (`CHECK`, real scorer, hash-256): the warranty request 0.49 → admitted, the lunch menu 0.04 → rejected, kim's notes "Trade fair in Hamburg … booth size still open" 0.23 and "Company car policy … changes in January" 0.27 → quarantine. Out-of-scope topics include trade fairs/travel, HR policies/company cars and office events. The docs and `pipeline.csv` are admitted (estimated 0.47–0.69).
`queries.bdql` section 19 runs `SHOW PURPOSE`, two `CHECK`s, writes that land in quarantine or are rejected, the review queue
(`SELECT … FROM quarantine`), `ADMIT`, `REJECT` and one `REMEMBER … OVERRIDE PURPOSE REASON '…'`.
Borderline scores can differ slightly with the configured embedding.
