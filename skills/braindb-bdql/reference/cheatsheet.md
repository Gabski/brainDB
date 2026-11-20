# BDQL cheat-sheet

Statements end with `;`. Keywords are case-insensitive. `--` starts a comment.

| element | form |
|---|---|
| strings | `'…'`, `"…"`, or `$$ … $$` (multi-line) |
| ids | bare: `d121`, `lab:x204` (external), `A:pricing` (concept), `H:customer-42` (connector) |
| variables | `AS F` → `$F` (id); `LET $n = (SELECT …)` (value) |
| operators | `= != <> > >= < <= ~ (contains) IN (…) NOT AND OR ( )`; numbers, money and percent compare numerically |
| missing values | a node that does not carry the field matches **no** comparison, not even `!=` or `NOT IN`. Use `fields.x != ''` to mean "has a value" |
| unknown columns | a name no row in the table defines is an error, not NULL: `SELECT banana` → `unknown column "banana" in memory (has: …)` |
| pseudo-columns | `lit`, `pinned`, `superseded`, `external`, `connector`, `route`, `score` (only with NEAR), `age` (days), `touched` (ISO-8601 last touch), `weight`, `activation`, `depth` |
| columns | `id kind section text weight activation depth via age touched pinned superseded concepts route source fields.<name>`, `fields.*` |
| time | relative: `age` in days. Absolute: `touched`, e.g. `WHERE touched < '2026-11-01'` (ISO-8601 strings sort chronologically). There is no `AS OF` / time travel |
| patterns | `PATTERN ADD/REMOVE` is written to `braindb.yaml` when the script commits, so it survives a restart |

## Session

```sql
USE <db>;
SHOW kinds | sections | concepts | connectors | sources | dbs | stats | config | templates;
GOAL "<text>" [FOCUS id, …];
FOCUS id, …;   FOCUS ADD id;   FOCUS REMOVE id;   FOCUS GOAL;
BEGIN; … COMMIT;   ROLLBACK;
```

## Read

```sql
CONTEXT [FOCUS id, … | FOR "<text>"] [DEPTH n] [THRESHOLD x] [HOP x] [SOURCES name, … | *] [FORMAT md|xml|json|yaml] [SET];
SELECT <cols | aggregates> FROM memory | concepts | connectors | links | memory@<source> | memory@*
  [WHERE <predicate>] [NEAR "<text>" [MODE hybrid|vector|tree|graph]]
  [GROUP BY <col>] [ORDER BY <col> [ASC|DESC], …] [LIMIT n] [OFFSET n] [FORMAT md|json|yaml|xml|csv];
  -- aggregates: COUNT(*), SUM(f), AVG(f), MIN(f), MAX(f)
SEARCH "<text>" [MODE hybrid|vector|tree|graph] [LIMIT n] [SOURCES …] [FOCUS];
QUERY '<compact syntax, e.g. kind:deal stage=negotiation value>10000 sort:-value limit:10>' [FORMAT …];
INSPECT id [WITH item, links, near, test, suggest];
TEST id;   SUGGEST id;   DISCOVER;   SUGGEST CONNECTORS [LIMIT n];
PATHS FROM id TO id [LIMIT n];
EXPLAIN <SELECT | SEARCH | CONTEXT …>;
```

## Write (a script with writes is atomic)

```sql
REMEMBER <kind> "<text>" [IN <section>] [CONCEPTS "<label>", …] [LINK TO id [WEIGHT w], …]
  [FIELDS (name = value, …)] [SUPERSEDES id] [PIN] [FOCUS] [AS X];
INSERT INTO <kind> (text, <field>, …) VALUES (…), (…);         -- columns other than text/section/concepts → fields
UPDATE memory SET text = '…', fields.<f> = '…' WHERE <predicate> [DRY RUN] [CONFIRM];
CONCEPT "<label>" [MEMBERS id, …] [RELATED "<label>", …] [AS C];
LINK id TO id [WEIGHT w] [TYPE assoc|similar];   UNLINK id FROM id;
BOOST id, …;   WEAKEN id, …;   PIN id;   UNPIN id;
SUPERSEDE old WITH new;
FORGET id [FORCE];   FORGET WHERE <predicate> [DRY RUN] [CONFIRM];
KIND ADD <name> [COLOR '#hex'] [HALF LIFE days] [BASE w] [PREFIX p] [SECTION s];
```

## Connectors ⧉ (structure, no knowledge, no tokens)

```sql
CONNECTOR anchor|bridge|hub|alias|time "<label>" [KEY '<key>'] [TERMS '<t>', …] [CONNECT id, …] [GAIN g] [TRANSPARENT] [AS H];
CONNECT id TO H:…;   DISCONNECT id FROM H:…;
RESOLVE <entity> BY fields.<a>[, fields.<b>] [MIN 2] [DRY RUN];
SELECT id, type, label, key, degree FROM connectors WHERE type = anchor ORDER BY degree DESC LIMIT 20;
SELECT id, text FROM memory WHERE route = 'H:customer-42' LIMIT 20;
```

## Entities (built-in pattern library)

Types: `date datetime duration money percent number_unit email phone url domain person org iban vat card ip version id_code hashtag mention geo address`.

```sql
SHOW patterns;   SHOW pii;
SELECT type, value, COUNT(*) FROM entities WHERE type = <type> GROUP BY value ORDER BY COUNT(*) DESC LIMIT 20;
SELECT id, text FROM memory WHERE HAS <type> AND entities.<type> = '<normalised value>' LIMIT 10;
  -- typed comparison: entities.money > 'EUR 10000' (same currency unless fx configured), entities.date BETWEEN 'YYYY-MM-DD' AND 'YYYY-MM-DD'
EXTRACT FROM "<text>";   EXTRACT id;
PATTERN ADD <name> '<RE2 regex>' [TYPE <type>] [NORMALIZE upper|lower|none] [PII true|false];
PATTERN TEST <name> '<text>';   PATTERN REMOVE <name>;
RESOLVE email | phone | domain | org | person | vat | id_code [MIN 2] [DRY RUN];   -- anchors by normalised entity value
```

Matching uses normalised values: phones in E.164 (`+4722334455`), dates in ISO, emails lower-cased, money as `CUR amount`.

## Documents, files, sources

```sql
FILL TEMPLATE '<name>' [OUTPUT json|yaml|xml|md] [SCOPE context|memory|federated] [WITH SOURCES] [MIN x];
FILL $$ <template text> $$ AS yaml|json|xml|md [OUTPUT …];
  -- slot syntax: "description[:type][|option…]"; types text name (= person) number money percent date email phone url domain org duration iban vat address version bool list id (entity types come from the pattern library)
  -- options kind= section= concept= scope= limit= min= field=; list slots: key[]: "items|kind=deal|limit=5", key[].x: ".text" / ".fields.stage"
INGEST '<path>' [FOCUS];
IMPORT '<path>' [UPDATE] [DRY RUN];
SOURCE ADD <name> '<uri>' [TOKEN ENV <VAR>] [WEIGHT w] [DEPTH n];   SOURCE SYNC <name>;   SOURCE REMOVE <name>;
```

## Purpose and admission

```sql
SHOW PURPOSE;
CHECK "<text>" [AS <kind>] [IN <section>];          -- score, decision (admitted | quarantine | rejected), reasons; nothing stored
SELECT id, text, purpose.score, purpose.reasons FROM quarantine ORDER BY purpose.score DESC LIMIT 20;
ADMIT id, … [IN <section>];   REJECT id, … REASON '<why>';
REMEMBER … OVERRIDE PURPOSE REASON '<why>';         -- needs the purpose-override grant
STRICT PURPOSE;                                     -- at the start of a script: any rejected write aborts the script
```

## Access (users, roles, labels)

```sql
WHOAMI;
SHOW users | roles | labels | grants [FOR <user>];
CREATE USER <name> ROLES (<role>, …) TOKEN ENV <VAR>;   ALTER USER <name> ADD ROLE <role>;   DROP USER <name>;
CREATE ROLE <role>;   GRANT read[, write] ON LABEL <label> TO ROLE|USER <name> [IN SECTION <s>];
REVOKE … FROM …;   DENY read ON LABEL <label> TO ROLE <role>;
LABEL id, … ADD|REMOVE <label>;   LABEL WHERE <predicate> ADD <label> [DRY RUN] [CONFIRM];
REMEMBER … LABELS (<label>, …);   INSERT INTO … VALUES (…) LABELS (<label>);
EXPLAIN ACCESS id FOR <user>;   AS USER <user>; <statements>;     -- admin only
```

A node is visible only to principals holding all of its labels. Invisible ids behave as nonexistent.

## Limits and guards

- Max 200 statements per script.
- Max 1000 rows per result.
- 30 s timeout.
- `UPDATE` / `FORGET` need `WHERE`.
- Bulk writes over 100 nodes need `CONFIRM`.
- Read-only deployments reject all writes.
