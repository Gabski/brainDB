# Example: import-demo (create a database from a pile of files)

Pellucid Coffee Systems is a fictional distributor of commercial espresso machines, grinders and service plans in the Nordics.
Its knowledge sits in six unrelated exports, and brainDB turns them into one database (Addendum E):

| file | format | content |
|---|---|---|
| `raw/notes/*.md` | Markdown, YAML front-matter (`title, kind, section, tags, customer_id, account, product, date`), `[[wiki links]]` | 7 work notes (Q4 account plan, Meridian escalation, AX-1 known issue, churn signals, 2027 prices, service plans, weekly support review) plus 4 deliberately off-topic notes for the purpose filter: `lunch-menu.md`, `personal-reminder.md`, `webshop-payment-test.md` (contains the published test card number 4111 1111 1111 1111) and `team-offsite-ideas.md` |
| `raw/crm.sql` | PostgreSQL dump | `customers` (15, via COPY; with `legal_name` such as `… AS / AB / ApS / Oy`, `website`, and `vat_no` for the Norwegian ones, `NO … MVA` with valid mod-11), `contacts` (25, via multi-row INSERT, inline `REFERENCES`), `orders` (40: 30 via COPY + 10 via INSERT, table-constraint FKs to customers and contacts). Low-cardinality columns: `segment` (enterprise / mid-market / smb), `status` (active / trial / churned; orders: open / shipped / invoiced / cancelled) |
| `raw/mysql-sample.sql` | MySQL dump | `suppliers` (4) and `products` (8) with backticks, `ENGINE=InnoDB`, multi-row INSERT, escaped quotes (`King\'s Lynn`, `\"made in-house\"`), FK `products.supplier_id → suppliers.id` |
| `raw/tickets.jsonl` | JSON Lines | 30 support tickets: `customer_id`, `order_id`, `product`, `priority`, `status`, nested `customer.{name,segment}` and `reporter.{name,email,phone}`. The reporter's email is written in upper or title case, and the phone in national or compact form, so the same contact appears differently than in `crm.sql`. |
| `raw/products.json` | nested JSON | 8 products under `$.data.items`, with nested `specs`, `price`, `supplier`, `warranty` |
| `raw/pricing.csv` | CSV | 26 price rows: `sku, segment, list_price_eur, discount_pct, net_price_eur, valid_from, valid_to, note` |

`braindb.yaml` maps each file. It declares:

- custom kinds `customer` (`u`), `contact` (`v`), `purchase` (`o`; the order rows), `ticket` (`k`) and `product` (`y`);
- per-table text templates, concept columns and FKs;
- the JSON path `$.data.items` for `products.json`;
- connectors (Addendum H): customer and product anchors, three aliases, an import hub and weekly time buckets.

The kind for order rows is called `purchase` because `order` collides with `ORDER BY` in BDQL.

## Run

```bash
braindb create demo --from examples/import-demo/raw --config examples/import-demo/braindb.yaml --dry-run   # preview
braindb create demo --from examples/import-demo/raw --config examples/import-demo/braindb.yaml
braindb serve --data-dir data --db demo
braindb run examples/import-demo/queries.bdql --db demo
```

## Expected import report

| file | importer | status | records / nodes | edges (before auto-link) |
|---|---|---|---|---|
| `notes/*.md` (7 work notes) | markdown | created | 7 file nodes + ~14 chunks (split at h1/h2) | `part`/`seq` per file; 11 `assoc` edges from 17 `[[wiki links]]` (11 distinct pairs); `tags` → concepts |
| `crm.sql` | sql (postgres) | created | 80 records: customers 15, contacts 25, orders 40 | 105 FK edges: contacts→customers 25, orders→customers 40, orders→contacts 40 |
| `mysql-sample.sql` | sql (mysql) | created | 12 records: suppliers 4, products 8 | 6 FK edges: products→suppliers (2 service plans have `NULL` supplier) |
| `tickets.jsonl` | json | created | 30 records (kind `ticket`) | — |
| `products.json` | json (`$.data.items`) | created | 8 records (kind `product`), fields such as `specs.boiler_l`, `price.list`, `supplier.name` | — |
| `pricing.csv` | csv | created | 26 records (section `pricing`) | `part`/`seq` |
| `notes/` off-topic (4 files) | markdown | 1 quarantined, 3 rejected | see the purpose table below | |
| **total** | | 16 files | **156 records + 7 files + ~14 chunks admitted; 1 file quarantined; 3 rejected** | |

Concepts from low-cardinality columns:

- `segment`: enterprise, mid-market, smb;
- customer `status`: active, trial, churned;
- order `status`: open, shipped, invoiced, cancelled;
- ticket `priority`: low, medium, high, urgent;
- ticket and order `product` / `sku`: 8 SKUs;
- product `category`;
- note tags such as escalation, churn and AX-1.

Connectors after the import:

- one **anchor per customer** (`customer:1` … `customer:15`), joining the SQL row, its contacts and orders, its tickets
  (`tickets.customer_id`) and the notes that name it (`fields.customer_id`). For example, `customer:12` (Meridian Airports Lounge) joins
  1 customer row, 2 contacts, its orders, 3 tickets and 3 notes.
- one **anchor per SKU**;
- the aliases **ARR**, **SLA** and **net price**;
- one **hub** for the import batch.

Run the import a second time and every item reports `skipped` (unchanged hash). Change a row and run with `--update`:
the old record is superseded, not deleted.

## Queries across sources

| question | how | expected |
|---|---|---|
| Tickets of customers with open orders | `SELECT fields.customer_id, COUNT(*) FROM memory WHERE kind = purchase AND fields.status = 'open' GROUP BY fields.customer_id;` then the tickets of customers 1, 4, 5, 7, 8, 9, 11, 12, 14, 15 that are not closed | 14 open orders across 10 customers; 14 non-closed tickets, including TCK-3101 (Granite & Grain, urgent, AX-1), TCK-3109 (Meridian, urgent, AX-1), TCK-3121 (Westbrook, urgent, SV-CARE) and TCK-3129 (Saltmarsh, urgent, SV-CARE) |
| Contacts of enterprise customers | `kind:customer segment=enterprise` → ids 1, 3, 5, 8, 12; then `kind:contact customer_id` in those | 10 contacts (Anna Holm, Jonas Berg, Ida Nyberg, Lars Lind, Oskar Aas, Nora Vik, Maja Brekke, Axel Falk, Mats Tangen, Karin Ruud) |
| Everything about Meridian Airports Lounge | `CONTEXT FOCUS <H:customer-12> DEPTH 1` (anchor) | SQL row, 2 contacts, orders, tickets TCK-3109 / 3114 / 3119, and the notes "Meridian escalation", "AX-1 milk system", "Q4 account plan" |
| Churned customers still filing tickets | `kind:ticket status!=closed "Oxbow"` or `SEARCH "churned customer urgent ticket"` | TCK-3125 (Oxbow Bookshop Café, urgent), matching the "Churn signals" note |
| Enterprise price of the PX-300 in 2026 vs 2027 | `section:pricing sku=PX-300 segment=enterprise` | €9,592 net (2026, 12% off €10,900) and €10,076 (2027, list €11,450) |
| Margin check: supplier cost vs list price | `section:catalog sku=PX-200` plus `kind:product sku=PX-200` | cost €4,345 (MySQL) vs list €7,900 (JSON) |
| Alias routing | `SEARCH "service level for enterprise customers"` | the alias **SLA** expands the query, so `SV-PRIO` (4 h response) and the "CareService plans" note rank high |

Compact raw queries:

```
kind:purchase status=open sort:-total select:fields.id,fields.customer_id,fields.sku,fields.total
kind:ticket priority=urgent status!=closed select:fields.id,fields.customer.name,fields.product
kind:customer segment=enterprise select:fields.id,fields.name,fields.owner
kind:product sort:-price.list select:fields.sku,fields.name,fields.price.list,fields.supplier.name
section:pricing segment=enterprise discount_pct>=12 format:md
```

## Entities and anchors by value (Addendum I)

The same person shows up three ways. Contact 23, Astrid Engen of Westbrook Fitness AS, appears as:

- `astrid.engen@westbrook.example`, `+47 24 40 87 67` (crm.sql);
- `ASTRID.ENGEN@WESTBROOK.EXAMPLE` or `Astrid.Engen@Westbrook.Example`, `24408767` (tickets.jsonl);
- `24 40 87 67` (churn-signals.md).

`RESOLVE email` / `RESOLVE phone` (or `connectors.entityAnchors: [email, phone, vat]` on import) should create one anchor per person.
`entities.domain = 'meridian.example'` joins the contacts, the ticket reporters, the customer website and the escalation note.

The notes add more entity formats:

- money: `€48,000`, `12 500 NOK`, `USD 3.2k`, `1 200,00 zł`;
- dates: `15.12.2026`, `21.11.2026`, `Q1 2027`;
- `SKU-40707`;
- the organisations Autoflow GmbH and Baltic Brew Service sp. z o.o.

Config: custom patterns `sku` and `ticket`, FX rates (EUR, USD, NOK, SEK, DKK, PLN), `privacy.mask: [phone]`.

## BDQL

`braindb run examples/import-demo/queries.bdql --db demo`

`queries.bdql` runs the full tour, including `RESOLVE customer BY … DRY RUN`, `SELECT … FROM connectors`,
`SELECT … WHERE route = …`, a write transaction, `INSERT INTO purchase` / `INSERT INTO ticket`, `UPDATE`, `FORGET … DRY RUN`
and an inline `FILL`.

## Access (Addendum M)

`access.defaults.import: {"crm.sql#orders": [finance]}` labels all 40 order records `finance` at import.
Customers, contacts, tickets, notes and prices stay unlabelled.

| user | role | sees |
|---|---|---|
| `rosa` | support | everything except `finance`: no orders |
| `lee` | analyst | `finance` (orders) plus unlabelled data |
| `admin` | admin | everything |

```sql
AS USER rosa; SELECT COUNT(*) FROM memory WHERE kind = purchase;    -- 0: the orders do not exist for rosa
AS USER lee;  SELECT COUNT(*) FROM memory WHERE kind = purchase;    -- 40
AS USER rosa; SEARCH "Meridian Airports Lounge open order" LIMIT 5; -- customer row, tickets, escalation note; no order 1038
```

The cross-source question "tickets of customers with open orders" therefore works for `lee` and `admin`. For `rosa`, step 1 returns
no rows. In the inline `FILL` in `queries.bdql`, the slot `open_order_value` is `missing` for `rosa` and €10,800 (order 1038) for `lee`.

## Purpose and admission (Addendum O)

`braindb.yaml` declares the purpose:

- **name:** "Pellucid customer knowledge base";
- **in scope:** customers, orders, prices, products, support, escalations and churn;
- **out of scope:** lunch menus, personal reminders, payment card data and office social events;
- **forbidden entities:** `card`;
- **admission:** `mode: enforce`, accept 0.35, review 0.20, `records: table`, `chunks: file`, `exempt: [admin]`.

Dry runs and `CHECK` always show the would-be decision, even for exempt admins, so
`braindb create demo --from examples/import-demo/raw --config examples/import-demo/braindb.yaml --dry-run` reports the decisions below.

| file / table | scored as | est. score | decision | main reason |
|---|---|---|---|---|
| `crm.sql#customers` | table | 0.51 | admitted | matches "key customer accounts" and "customer table" topics |
| `crm.sql#contacts` | table | 0.43 | admitted | "contact table" topic |
| `crm.sql#orders` | table | 0.51 | admitted | "order table" topic |
| `mysql-sample.sql#suppliers` / `#products` | table | 0.68 / 0.76 | admitted | "suppliers and lead times", "product costs" |
| `tickets.jsonl` | table | 0.52 | admitted | "support ticket table" |
| `products.json` | table | 0.68 | admitted | espresso machines, grinders, filters |
| `pricing.csv` | table | 0.68 | admitted | "pricing: price list rows …" |
| 7 work notes | file | 0.51–0.81 | admitted | in-scope topics, plus a link bonus from `[[wiki links]]` |
| `notes/team-offsite-ideas.md` | file | **0.29** (measured) | **quarantine** | conflicts with "office social events"; one line on churn risk and service plans keeps it in the review band |
| `notes/lunch-menu.md` | file | 0.18 | **rejected** | "lunch menus and canteen" |
| `notes/personal-reminder.md` | file | 0.15 | **rejected** | "personal reminders and private errands" |
| `notes/webshop-payment-test.md` | file | — | **rejected** | hard rule: forbidden entity `card` (Luhn-valid 4111 1111 1111 1111), whatever the score |

With `records: table`, each table is scored once, using its name, its columns and up to 20 sample rows, and its records inherit
the decision. Decisions and the offsite score were measured with `braindb create --dry-run` (real scorer, hash-256). The
real report says `purpose: 166 admitted, 1 quarantined, 3 rejected` (lunch menu 0.15, personal reminders 0.15, card note
rejected by the forbidden-entity rule). Table scores are estimates from the spec formula. Borderline scores can differ
slightly with the configured embedding; the offsite note sits mid-band, between 0.22 and 0.29 across small wording variants.

Review the queue as admin (see `queries.bdql` section 19):

```sql
SELECT id, fields.title, purpose.score, purpose.reasons FROM quarantine;
REJECT <id of team-offsite-ideas> REASON 'office social event, not customer knowledge';
```
