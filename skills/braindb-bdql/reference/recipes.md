# BDQL recipes

Copy, replace the `<…>` parts, run as one script. All recipes follow the efficiency rules: filters first, LIMIT always,
only the needed columns.

## Orientation

```sql
-- first contact with an unknown brain
SHOW stats;  SHOW kinds;  SHOW sections;  SHOW concepts;
SELECT kind, COUNT(*) FROM memory GROUP BY kind;
SELECT * FROM memory WHERE kind = <kind> LIMIT 1;            -- discover field names
```

```sql
-- start a task: light the relevant slice and keep it as the focus
GOAL "<task in one sentence>";
CONTEXT FOR "<task keywords>" DEPTH 2 SET FORMAT md;
```

## Answering

```sql
-- fuzzy question → best evidence
SEARCH "<question>" LIMIT 5;

-- exact rows ranked by relevance inside a filtered set
SELECT id, text, fields.<f> FROM memory
  WHERE kind = <kind> AND NOT superseded AND fields.<key> = '<value>'
  NEAR "<topic>" ORDER BY score DESC LIMIT 10;

-- numbers
SELECT fields.<group>, COUNT(*), SUM(fields.<amount>) FROM memory
  WHERE kind = <kind> AND fields.<date> >= '2026-10-01' GROUP BY fields.<group>;

-- why are these related?
PATHS FROM <id> TO <id> LIMIT 2;

-- everything known about one entity (through its anchor connector)
SELECT id, kind, text FROM memory WHERE route = 'H:<entity-key>' ORDER BY weight DESC LIMIT 20;

-- only what is current and in context
SELECT id, text FROM memory WHERE lit AND kind IN (decision, fact) AND NOT superseded ORDER BY activation DESC LIMIT 10;
```

## Entities

```sql
-- what does the brain know about a person / company / domain
SELECT id, kind, text FROM memory WHERE entities.email = '<email>' OR entities.person = '<Full Name>' ORDER BY weight DESC LIMIT 20;
SELECT id, text FROM memory WHERE entities.domain = '<domain>' LIMIT 20;

-- deadlines and amounts
SELECT id, text, entities.date FROM memory WHERE kind = task AND entities.date BETWEEN '<YYYY-MM-DD>' AND '<YYYY-MM-DD>' ORDER BY entities.date LIMIT 20;
SELECT id, text, entities.money FROM memory WHERE entities.money >= '<CUR amount>' ORDER BY entities.money DESC LIMIT 20;

-- most frequent contacts
SELECT value, COUNT(*) FROM entities WHERE type = email GROUP BY value ORDER BY COUNT(*) DESC LIMIT 10;

-- one anchor per person/company seen in several places
RESOLVE email DRY RUN;
RESOLVE email;

-- teach the brain a new identifier format
PATTERN ADD <name> '<RE2 regex>' TYPE id_code NORMALIZE upper;
PATTERN TEST <name> '<sample text>';
```

## Recording

```sql
-- a result derived from evidence
REMEMBER fact "<self-contained sentence with values>" CONCEPTS "<A>", "<B>" LINK TO <evidence ids> AS F;
BOOST <evidence ids>;

-- a decision that replaces an older one
REMEMBER decision "<new decision with date/owner>" CONCEPTS "<A>" LINK TO <reasons> AS D;
SUPERSEDE <old decision id> WITH $D;
FOCUS <task id>, $D;

-- open work
REMEMBER task "<what, who, by when>" CONCEPTS "<A>" FIELDS (owner = '<name>', due = '<YYYY-MM-DD>') LINK TO <context id>;
REMEMBER question "<what is unknown and why it matters>" CONCEPTS "<A>" LINK TO <context id>;

-- structured rows
INSERT INTO <kind> (text, <f1>, <f2>, <f3>) VALUES ('<text>', '<v1>', <v2>, '<v3>'), ('<text>', '<v1>', <v2>, '<v3>');
UPDATE memory SET fields.<f> = '<new>' WHERE id = <id>;
```

## Organising

```sql
-- the same entity scattered across imports → one anchor
RESOLVE <entity> BY fields.<id field>, fields.<name field> DRY RUN;
RESOLVE <entity> BY fields.<id field>, fields.<name field>;

-- a workstream entry point
CONNECTOR hub "<workstream>" CONNECT <id>, <id>, <id> AS H;
FOCUS $H;

-- vocabulary the brain should understand
CONNECTOR alias "<term>" TERMS '<synonym 1>', '<synonym 2>';

-- let the brain propose structure
DISCOVER;
SUGGEST CONNECTORS LIMIT 10;
SUGGEST <id>;
```

## Documents

```sql
FILL TEMPLATE '<template file>' OUTPUT md WITH SOURCES;

FILL $$
client:
  name: "client name"
  owner: "account owner:name"
  arr: "annual contract value:money"
  renewal: "contract renewal date:date"
open_deals[]: "open deals|kind=deal|limit=5"
open_deals[].title: ".text"
open_deals[].value: ".fields.value"
$$ AS yaml OUTPUT json WITH SOURCES;
```

If `missing` is not empty, find those values, `REMEMBER` them with `FIELDS`, then fill again.

## Data in and out

```sql
IMPORT '<dir or file>' DRY RUN;     -- review importers, tables, record counts
IMPORT '<dir or file>';
IMPORT '<dir>' UPDATE;              -- re-import changed files: old versions are superseded
SELECT id, fields.* FROM memory WHERE section = '<table>' LIMIT 100 FORMAT csv;
```

## Other brains

```sql
SOURCE ADD <name> '<http://host:7791>' TOKEN ENV <VAR> WEIGHT 0.7;
SEARCH "<question>" SOURCES <name> LIMIT 5;
SELECT id, text FROM memory@<name> WHERE kind = fact NEAR "<topic>" LIMIT 5;
LINK <local id> TO <name>:<remote id> WEIGHT 0.7;
```

## Maintenance

```sql
SELECT id, text, age FROM memory WHERE kind = question AND age > 14 AND NOT superseded ORDER BY age DESC LIMIT 20;
SELECT id, text FROM memory WHERE kind = task AND fields.due < '<today>' AND NOT superseded LIMIT 20;
FORGET WHERE kind = note AND age > 90 AND NOT pinned DRY RUN;
EXPLAIN SELECT id FROM memory WHERE fields.<f> = '<v>' NEAR "<topic>" LIMIT 10;
```

## Anti-patterns

| instead of | write |
|---|---|
| `SELECT * FROM memory NEAR "x"` (no filter, no limit, all columns) | `SELECT id, text FROM memory WHERE kind = fact NEAR "x" LIMIT 5` |
| five separate `brain_run` calls for one turn | one script, using `AS X` / `$X` |
| `FORGET d131` for an outdated decision | `SUPERSEDE d131 WITH $NEW` |
| `REMEMBER note "see above"` | one self-contained sentence with names, values, dates |
| many `LINK` statements between every pair of rows of one customer | `RESOLVE customer BY fields.customer_id` (one ⧉ anchor) |
| `FOCUS` with 20 ids | a hub connector or 1–5 ids, plus `DEPTH` |
| `WHERE text ~ '600 100 200'` to find a phone number | `WHERE entities.phone = '+48600100200'` |
| inventing values for FILL `missing` slots | search, ask, REMEMBER, fill again |
| `SOURCES *` on every query | local first; federate only when needed |
