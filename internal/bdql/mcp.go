// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package bdql

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Gabski/brainDB/internal/brain"
	"github.com/Gabski/brainDB/internal/mcp"
)

// Syntax is the statement summary used for the brain_run cheat-sheet (one line per statement family).
var Syntax = []string{
	`CONTEXT [FOCUS id,… | FOR "text"] [DEPTH n] [THRESHOLD x] [HOP x] [FORMAT md|xml|json|yaml] [SET]`,
	`SELECT cols|*|fields.*|COUNT(*)|SUM(f)… FROM memory|concepts|connectors|links|entities|memory@src [WHERE …] [NEAR "text" [MODE hybrid|vector|graph]] [GROUP BY c] [ORDER BY c [DESC]] [LIMIT n] [OFFSET n] [FORMAT md|json|yaml|xml|csv]`,
	`  WHERE: = != > >= < <= ~ (contains), IN (…), BETWEEN a AND b, NOT, AND, OR, HAS <entity>; columns id kind section text weight activation depth via age pinned superseded lit concept route fields.<f> entities.<type>`,
	`SEARCH "text" [MODE hybrid|vector|tree|graph] [LIMIT n] [FOCUS];   QUERY 'compact: kind:deal stage=won value>10000 sort:-value limit:10'`,
	`INSPECT id [WITH item, links, near, test, suggest];  TEST id;  SUGGEST id;  SUGGEST CONNECTORS;  DISCOVER;  PATHS FROM id TO id [LIMIT n];  EXPLAIN <stmt>`,
	`REMEMBER kind "text" [IN section] [CONCEPTS "label",…] [LINK TO id [WEIGHT w],…] [FIELDS (f = v,…)] [SUPERSEDES id] [PIN] [FOCUS] [AS X]   -- then use $X`,
	`INSERT INTO kind (text, f1, f2) VALUES ('…', …), (…);   UPDATE memory SET text = '…', fields.f = '…' WHERE … [DRY RUN] [CONFIRM]`,
	`CONCEPT "label" [MEMBERS id,…] [RELATED "label",…] [AS C];  LINK a TO b [WEIGHT w] [TYPE assoc|similar];  UNLINK a FROM b`,
	`BOOST id,…;  WEAKEN id,…;  PIN id;  UNPIN id;  SUPERSEDE old WITH new;  FORGET id [FORCE];  FORGET WHERE … [DRY RUN] [CONFIRM]`,
	`FOCUS id,…;  FOCUS ADD id;  FOCUS REMOVE id;  FOCUS GOAL;  GOAL "text" [FOCUS id,…]`,
	`CONNECTOR anchor|bridge|hub|alias|time "label" [KEY 'k'] [TERMS 't',…] [CONNECT id,…] [AS H];  CONNECT id TO H:…;  RESOLVE email|phone|org|… [MIN 2] [DRY RUN];  RESOLVE name BY fields.a, fields.b`,
	`FILL TEMPLATE 'name.yaml' [OUTPUT json|yaml|xml|md] [SCOPE context|memory] [WITH SOURCES] [MIN x];  FILL $$ yaml template $$ AS yaml [OUTPUT md]`,
	`EXTRACT FROM "text";  EXTRACT id;  PATTERN ADD name 'regex' [TYPE t];  PATTERN TEST name 'text';  SHOW kinds|sections|concepts|connectors|sources|stats|templates|patterns|pii`,
	`INGEST 'path' [FOCUS];  IMPORT 'path' [UPDATE] [DRY RUN];  SOURCE ADD name 'uri' [WEIGHT w];  KIND ADD name [HALF LIFE d] [BASE w];  BEGIN; … COMMIT;  ROLLBACK;  LET $n = (SELECT …)`,
}

// CheatSheet returns the compact BDQL reference embedded in the brain_run description.
func CheatSheet() string { return strings.Join(Syntax, "\n") }

func init() {
	mcp.Register(&mcp.Tool{Name: "brain_run",
		Description: "Run a BDQL script (brainDB Query Language): one call can read the context, search, query, inspect, fill templates and write. " +
			"Statements end with ';'. A script with writes is atomic (all or nothing). Ids are bare (d121, A:pricing, H:customer-42, lab:x204); strings use quotes. " +
			"Typical turn: CONTEXT FOR \"…\"; SEARCH \"…\" LIMIT 5; REMEMBER fact \"…\" CONCEPTS \"…\" AS F; LINK $F TO d121; BOOST d121;\n" + CheatSheet(),
		InputSchema: json.RawMessage(`{"type":"object","required":["script"],"properties":{"script":{"type":"string","description":"BDQL statements separated by ;"},
"format":{"type":"string","enum":["md","json","yaml","xml","csv"],"description":"format for row results (default md)"},"dry_run":{"type":"boolean","description":"run and roll back"}}}`),
		Run: func(s *mcp.Server, args json.RawMessage) (string, error) {
			var a struct {
				Script string `json:"script"`
				Format string `json:"format"`
				DryRun bool   `json:"dry_run"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return "", fmt.Errorf("invalid arguments: %v", err)
			}
			r := &Runner{Store: s.Store, ReadOnly: s.ReadOnly || s.Store.ReadOnly, Actor: s.Actor, DryRun: a.DryRun, Principal: s.Principal}
			if NewRunner != nil {
				NewRunner(r)
			}
			res, err := r.Run(a.Script)
			txt := res.Render(a.Format)
			if err != nil {
				return "", fmt.Errorf("%s (the failed transaction was rolled back)", res.Error)
			}
			return strings.TrimSuffix(strings.TrimSuffix(txt, res.Status), "\n"), nil
		}})
}

// NewRunner, when set, configures runners created by the MCP tool and HTTP hook (USE, SHOW dbs, IMPORT, …).
var NewRunner func(r *Runner)

// HTTPRun is the server hook for POST /api/run and /api/run/explain.
func HTTPRun(st *brain.Store, script string, readOnly, explain bool, actor string, p *brain.Principal) (any, error) {
	if explain {
		stmts, err := Parse(script)
		if err != nil {
			e := map[string]any{"ok": false, "error": err.Error()}
			if be, ok := err.(*Error); ok {
				e["line"], e["col"], e["statement"] = be.Line, be.Col, be.Stmt
			}
			return e, err
		}
		return map[string]any{"ok": true, "statements": stmts}, nil
	}
	r := &Runner{Store: st, ReadOnly: readOnly, Actor: actor, Principal: p}
	if NewRunner != nil {
		NewRunner(r)
	}
	res, err := r.Run(script)
	return res, err
}
