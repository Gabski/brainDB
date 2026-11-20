// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package bdql

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Gabski/brainDB/internal/brain"
	"github.com/Gabski/brainDB/internal/patterns"
)

// Limits.
var (
	MaxStatements = 200
	MaxRows       = 1000
	Timeout       = 30 * time.Second
	BulkLimit     = 100
)

// StmtResult is the result of one statement.
type StmtResult struct {
	Index     int           `json:"index"`
	Statement string        `json:"statement"`
	Kind      string        `json:"kind"` // rows | text | changes | plan
	Columns   []string      `json:"columns,omitempty"`
	Rows      []*brain.OMap `json:"rows,omitempty"`
	Text      string        `json:"text,omitempty"`
	Changes   []string      `json:"changes,omitempty"`
	IDs       []string      `json:"ids,omitempty"`
	Format    string        `json:"format,omitempty"`
	ElapsedMs float64       `json:"elapsed_ms"`
}

// Result is the result of a script.
type Result struct {
	Results []StmtResult `json:"results"`
	Status  string       `json:"status"`
	OK      bool         `json:"ok"`
	Error   string       `json:"error,omitempty"`
}

// Runner executes BDQL scripts against a store.
type Runner struct {
	Store     *brain.Store
	ReadOnly  bool
	Actor     string
	DryRun    bool
	Format    string // default rows format (md)
	Use       func(name string) (*brain.Store, error)
	ShowDBs   func() any
	ShowConf  func(st *brain.Store) any
	Import    func(st *brain.Store, path string, update, dry bool) (string, error)
	SyncSrc   func(st *brain.Store, name string) (string, error)
	SavePat   func(st *brain.Store) error
	Templates func(st *brain.Store) []string // directories with templates
	// Principal is the identity the script runs as (nil = admin / access disabled); DB names the database for grants.
	Principal  *brain.Principal
	DB         string
	SaveAccess func(st *brain.Store, a *brain.AccessConfig) error
}

type execState struct {
	r         *Runner
	b         *brain.Brain // brain used by the current statement (the principal's view for reads)
	real      *brain.Brain
	p         *brain.Principal
	origAdmin bool
	st        *brain.Store
	vars      map[string]string
	changes   []string
	touched   []string
	strict    bool
	rejVars   map[string]string
	// patDirty records that PATTERN ADD/REMOVE changed the live options. Persisting them has to wait
	// until the transaction is over: Store.Mutate holds the store lock for the whole script, and
	// saving takes that same lock.
	patDirty bool
}

// errReadOnly is returned for writes in read-only contexts.
var errReadOnly = errors.New("read-only: write statements are not allowed here")

func stmtErr(s *Stmt, err error) error {
	var be *Error
	if errors.As(err, &be) {
		return err
	}
	return &Error{Stmt: s.Index, Line: s.Line, Col: s.Col, Msg: err.Error()}
}

func hasWrite(s *Stmt) bool {
	if s.Kind == "EXPLAIN" {
		return false
	}
	if s.Kind == "PATTERN" && s.Sub != "TEST" {
		return true
	}
	return s.Write
}

// Run parses and executes a script. Scripts with writes run as one transaction (or several with BEGIN/COMMIT).
func (r *Runner) Run(script string) (*Result, error) {
	res := &Result{Results: []StmtResult{}}
	stmts, err := Parse(script)
	if err != nil {
		res.Error = err.Error()
		res.Status = r.Store.B.Status("")
		return res, err
	}
	if len(stmts) > MaxStatements {
		err := fmt.Errorf("script has %d statements (max %d)", len(stmts), MaxStatements)
		res.Error = err.Error()
		return res, err
	}
	st := r.Store
	if len(stmts) > 0 && stmts[0].Kind == "USE" {
		if r.Use == nil {
			err := stmtErr(stmts[0], errors.New("USE is not available: this session is bound to one database"))
			res.Error = err.Error()
			return res, err
		}
		s2, err := r.Use(stmts[0].Name)
		if err != nil {
			err = stmtErr(stmts[0], err)
			res.Error = err.Error()
			return res, err
		}
		st = s2
		res.Results = append(res.Results, StmtResult{Index: 1, Statement: stmts[0].Src, Kind: "text", Text: "using " + stmts[0].Name})
		stmts = stmts[1:]
	}
	writes := false
	for _, s := range stmts {
		if s.Kind == "USE" {
			err := stmtErr(s, errors.New("USE must be the first statement of a script"))
			res.Error = err.Error()
			return res, err
		}
		if hasWrite(s) {
			writes = true
			if r.ReadOnly {
				err := stmtErr(s, fmt.Errorf("%w (%s)", errReadOnly, s.Kind))
				res.Error = err.Error()
				res.Status = st.B.Status("")
				return res, err
			}
		}
	}
	x := &execState{r: r, st: st, vars: map[string]string{}, p: r.Principal}
	x.origAdmin = r.Principal == nil || r.Principal.Admin
	var runErr error
	finalStatus := ""
	body := func(b *brain.Brain) (string, []string, error) {
		x.b, x.real = b, b
		defer func() { finalStatus = x.rb().Status("") }()
		snap := b.Snapshot()
		committed := false
		deadline := time.Now().Add(Timeout)
		for _, s := range stmts {
			if time.Now().After(deadline) {
				runErr = stmtErr(s, fmt.Errorf("script timed out after %s", Timeout))
				break
			}
			switch s.Kind {
			case "BEGIN":
				snap = b.Snapshot()
				res.Results = append(res.Results, StmtResult{Index: s.Index, Statement: s.Src, Kind: "text", Text: "begin"})
				continue
			case "COMMIT":
				snap = b.Snapshot()
				committed = true
				res.Results = append(res.Results, StmtResult{Index: s.Index, Statement: s.Src, Kind: "text", Text: "commit"})
				continue
			case "ROLLBACK":
				b.Restore(snap)
				res.Results = append(res.Results, StmtResult{Index: s.Index, Statement: s.Src, Kind: "text", Text: "rolled back"})
				continue
			}
			t0 := time.Now()
			x.b = x.rb()
			if hasWrite(s) {
				x.b = x.real
			}
			out, err := x.exec(s)
			if err != nil {
				runErr = stmtErr(s, err)
				break
			}
			out.Index, out.Statement = s.Index, s.Src
			out.ElapsedMs = float64(time.Since(t0).Microseconds()) / 1000
			res.Results = append(res.Results, *out)
		}
		if runErr != nil || r.DryRun {
			b.Restore(snap)
			if r.DryRun && runErr == nil {
				res.Results = append(res.Results, StmtResult{Kind: "text", Text: "dry run: all changes rolled back"})
			}
			if !committed {
				if runErr == nil {
					return "", nil, errDryRun
				}
				return "", nil, runErr
			}
		}
		return strings.Join(x.changes, " "), x.touched, nil
	}
	if writes && !r.DryRun {
		err = st.Mutate(r.Actor, "brain_run", body)
	} else {
		err = st.Read(func(b *brain.Brain) error { _, _, e := body(b); return e })
	}
	if errors.Is(err, errDryRun) {
		err = nil
	}
	if err == nil && runErr != nil {
		err = runErr
	}
	// Persist pattern changes once the transaction is closed: a failed script must not leave them in
	// braindb.yaml, and saving needs the store lock that Mutate held.
	if x.patDirty && err == nil && !r.DryRun && r.SavePat != nil {
		if perr := r.SavePat(st); perr != nil {
			res.Error = perr.Error()
			return res, perr
		}
	}
	res.Status = finalStatus
	if res.Status == "" {
		res.Status = st.B.ViewFor(r.Principal, r.dbName()).Status("")
	}
	if err != nil {
		res.Error = err.Error()
		return res, err
	}
	res.OK = true
	return res, nil
}

var errDryRun = errors.New("dry run")

func (x *execState) id(r Ref) (string, error) {
	if r.Var != "" {
		v, ok := x.vars[r.Var]
		if why, rej := x.rejVars[r.Var]; !ok && rej {
			return "", fmt.Errorf("$%s is unset: its REMEMBER was rejected by the purpose filter (%s)", r.Var, why)
		}
		if !ok {
			return "", fmt.Errorf("unknown variable $%s (set it with AS %s or LET $%s = …)", r.Var, r.Var, r.Var)
		}
		return v, nil
	}
	return r.ID, nil
}

func (x *execState) ids(rs []Ref) ([]string, error) {
	out := []string{}
	for _, r := range rs {
		id, err := x.id(r)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func (x *execState) val(v Value) (string, error) {
	if v.Kind == "var" {
		s, ok := x.vars[v.Text]
		if !ok {
			return "", fmt.Errorf("unknown variable $%s", v.Text)
		}
		return s, nil
	}
	return v.Text, nil
}

// didYouMean improves "no such item X" errors.
func (x *execState) didYouMean(err error) error {
	msg := err.Error()
	const p = "no such item "
	if i := strings.Index(msg, p); i >= 0 {
		id := strings.Fields(msg[i+len(p):])
		if len(id) > 0 {
			if c := x.rb().ClosestID(id[0]); c != "" && c != id[0] {
				return fmt.Errorf("%s (did you mean %s?)", msg, c)
			}
		}
	}
	if strings.Contains(msg, "kind must be one of") {
		return err
	}
	return err
}

func (x *execState) ops(ops []brain.Op) (*brain.ExecResult, error) {
	if x.p != nil && !x.p.Admin {
		for i := range ops {
			if ops[i].Op == "remember" {
				ops[i].Labels = append(ops[i].Labels, x.p.Default...)
			}
		}
		if err := x.real.CheckOps(x.p, x.r.dbName(), ops); err != nil {
			return nil, x.didYouMean(err)
		}
	}
	exempt := x.purposeExempt()
	for i := range ops {
		if ops[i].Op == "remember" {
			ops[i].Exempt = exempt
			if ops[i].Override != "" && x.p != nil && !x.p.Admin && !x.p.Can("purpose-override", "*", "", "", x.r.dbName()) {
				return nil, errors.New("access denied: OVERRIDE PURPOSE needs the purpose-override grant")
			}
		}
	}
	r, err := x.real.Exec(ops)
	if err != nil {
		return nil, x.didYouMean(err)
	}
	if len(r.Rejected) > 0 && x.strict {
		rj := r.Rejected[0]
		return nil, fmt.Errorf("STRICT PURPOSE: %q rejected by the purpose filter (score %.2f; %s)", rj.Text, rj.Score, rj.Reasons)
	}
	x.changes = append(x.changes, r.Changes...)
	x.touched = append(x.touched, r.Touched...)
	return r, nil
}

func changesOf(r *brain.ExecResult) *StmtResult {
	ch := append([]string(nil), r.Changes...)
	for _, q := range r.Quarantined {
		ch = append(ch, fmt.Sprintf("(quarantined %s: score %.2f; %s)", q.ID, q.Score, q.Reasons))
	}
	for _, q := range r.Rejected {
		ch = append(ch, fmt.Sprintf("(rejected %q: score %.2f; %s)", q.Text, q.Score, q.Reasons))
	}
	for _, a := range r.Auto {
		m := "≈"
		if a.Type == brain.EdgeConcept {
			m = "◆"
		}
		ch = append(ch, fmt.Sprintf("%s%s%s", a.A, m, a.B))
	}
	return &StmtResult{Kind: "changes", Changes: ch, IDs: r.Touched}
}

func sp(s string) *string { return &s }

func fptr(s string) *float64 {
	if s == "" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &v
}

func iptr(s string) *int {
	if s == "" {
		return nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &v
}

// where builds the row predicate.
func (x *execState) where(e Expr) (func(get func(string) any) (bool, error), error) {
	if e == nil {
		return nil, nil
	}
	var eval func(e Expr, get func(string) any) (bool, error)
	eval = func(e Expr, get func(string) any) (bool, error) {
		switch t := e.(type) {
		case BinExpr:
			l, err := eval(t.L, get)
			if err != nil {
				return false, err
			}
			if t.Op == "AND" && !l {
				return false, nil
			}
			if t.Op == "OR" && l {
				return true, nil
			}
			return eval(t.R, get)
		case NotExpr:
			v, err := eval(t.X, get)
			return !v, err
		case HasExpr:
			v, _ := get("has:" + t.Type).(bool)
			return v, nil
		case BareExpr:
			v := get(t.Col)
			switch b := v.(type) {
			case bool:
				return b, nil
			case nil:
				return false, nil
			case string:
				return b != "", nil
			case brain.EntVals:
				return len(b.Vals) > 0, nil
			}
			return true, nil
		case CmpExpr:
			lit, err := x.val(t.Val)
			if err != nil {
				return false, err
			}
			return x.b.CompareValues(get(t.Col), t.Op, lit), nil
		case InExpr:
			v := get(t.Col)
			if v == nil {
				return false, nil // NULL is neither IN nor NOT IN a list
			}
			hit := false
			for _, iv := range t.Vals {
				lit, err := x.val(iv)
				if err != nil {
					return false, err
				}
				if x.b.CompareValues(v, "=", lit) {
					hit = true
					break
				}
			}
			return hit != t.Not, nil
		case BetweenExpr:
			lo, err := x.val(t.Lo)
			if err != nil {
				return false, err
			}
			hi, err := x.val(t.Hi)
			if err != nil {
				return false, err
			}
			v := get(t.Col)
			if ev, ok := v.(brain.EntVals); ok {
				for _, one := range ev.Vals {
					sv := brain.EntVals{Type: ev.Type, Vals: []string{one}}
					if x.b.CompareValues(sv, ">=", lo) && x.b.CompareValues(sv, "<=", hi) {
						return true, nil
					}
				}
				return false, nil
			}
			return x.b.CompareValues(v, ">=", lo) && x.b.CompareValues(v, "<=", hi), nil
		}
		return false, fmt.Errorf("unsupported condition")
	}
	return func(get func(string) any) (bool, error) { return eval(e, get) }, nil
}

func (x *execState) selectSpec(sel *SelectStmt) (*brain.SelectSpec, error) {
	w, err := x.where(sel.Where)
	if err != nil {
		return nil, err
	}
	return &brain.SelectSpec{From: sel.From, Where: w, Near: sel.Near, Mode: sel.Mode, Columns: sel.Columns, GroupBy: sel.GroupBy,
		Order: sel.Order, Limit: sel.Limit, Offset: sel.Offset, MaxRows: MaxRows}, nil
}

func (x *execState) matchIDs(where Expr) ([]string, error) { return x.matchIDsFrom("memory", where) }

func (x *execState) matchIDsFrom(from string, where Expr) ([]string, error) {
	spec, err := x.selectSpec(&SelectStmt{From: from, Where: where, Columns: []brain.SelCol{{Col: "id"}}})
	if err != nil {
		return nil, err
	}
	spec.MaxRows = 1 << 30
	r, err := x.rb().Select(spec)
	if err != nil {
		return nil, err
	}
	return r.IDs, nil
}

func rowsResult(cols []string, rows []*brain.OMap, ids []string, format string) *StmtResult {
	return &StmtResult{Kind: "rows", Columns: cols, Rows: rows, IDs: ids, Format: format}
}

func (x *execState) exec(s *Stmt) (*StmtResult, error) {
	b := x.b
	needAdmin := func() error {
		if !x.admin() && x.real.AccessEnabled() {
			return errAdmin
		}
		return nil
	}
	switch s.Kind {
	case "WHOAMI", "AS", "CREATE", "ALTER", "DROP", "GRANT", "REVOKE", "DENY":
		return x.accessStmt(s)
	case "STRICT":
		x.strict = true
		return &StmtResult{Kind: "text", Text: "strict purpose: a rejected write aborts the script"}, nil
	case "CHECK":
		c := x.rb().CheckPurpose(s.Str, s.Opts["kind"], s.Opts["section"], nil)
		m := brain.NewOMap().Set("score", c.Score).Set("decision", c.Decision).Set("pos", c.Pos).Set("neg", c.Neg).Set("bonus", c.Bonus).
			Set("topMatch", c.TopMatch).Set("topConflict", c.TopConflict).Set("rules", strings.Join(c.Rules, "; "))
		return rowsResult([]string{"score", "decision", "pos", "neg", "bonus", "topMatch", "topConflict", "rules"}, []*brain.OMap{m}, nil, ""), nil
	case "ADMIT", "REJECT":
		if !x.canReview() {
			return nil, errors.New("access denied: this statement needs the review grant")
		}
		ids, err := x.ids(s.Refs)
		if err != nil {
			return nil, err
		}
		if s.Kind == "ADMIT" {
			done, err := x.real.Admit(ids, s.Opts["section"])
			if err != nil {
				return nil, err
			}
			ch := []string{}
			for _, id := range done {
				ch = append(ch, "admitted:"+id)
			}
			x.changes = append(x.changes, ch...)
			x.touched = append(x.touched, done...)
			return &StmtResult{Kind: "changes", Changes: ch, IDs: done}, nil
		}
		if err := x.real.RejectQuarantined(ids); err != nil {
			return nil, err
		}
		ch := []string{}
		for _, id := range ids {
			ch = append(ch, "rejected:"+id)
		}
		if r := s.Opts["reason"]; r != "" {
			ch = append(ch, "reason: "+r)
		}
		x.changes = append(x.changes, ch...)
		return &StmtResult{Kind: "changes", Changes: ch}, nil
	case "LABEL":
		var ids []string
		var err error
		if s.Sub == "WHERE" {
			ids, err = x.matchIDs(s.Where)
		} else {
			ids, err = x.ids(s.Refs)
		}
		if err != nil {
			return nil, err
		}
		if s.Flags["dry"] {
			return &StmtResult{Kind: "text", Text: fmt.Sprintf("dry run: would %s labels %s on %d items: %s", s.Opts["op"], strings.Join(s.Strs, ","), len(ids), strings.Join(firstN(ids, 50), ", ")), IDs: ids}, nil
		}
		if s.Sub == "WHERE" && len(ids) > BulkLimit && !s.Flags["confirm"] {
			return nil, fmt.Errorf("LABEL would change %d items (> %d): add CONFIRM, or DRY RUN to review", len(ids), BulkLimit)
		}
		if len(ids) == 0 {
			return &StmtResult{Kind: "changes", Changes: []string{"no rows matched"}}, nil
		}
		o := brain.Op{Op: "label", IDs: ids}
		if s.Opts["op"] == "add" {
			o.Add = s.Strs
		} else {
			o.Remove = s.Strs
		}
		r, err := x.ops([]brain.Op{o})
		if err != nil {
			return nil, err
		}
		return changesOf(r), nil
	case "SHOW":
		switch s.Name {
		case "users", "roles", "labels", "grants":
			return x.showAccess(s)
		case "purpose":
			p := x.real.Purpose()
			if p == nil {
				return &StmtResult{Kind: "text", Text: "no purpose configured: the admission filter is off"}, nil
			}
			cnt := x.real.PurposeCounts()
			doc := brain.NewOMap().Set("name", p.Name).Set("description", strings.TrimSpace(p.Description)).Set("scope", p.InScope).
				Set("outOfScope", p.OutOfScope).Set("admission", p.Admission).Set("counters", cnt)
			return &StmtResult{Kind: "text", Text: brain.EmitYAML(doc)}, nil
		case "config", "sources", "dbs":
			if err := needAdmin(); err != nil && s.Name == "config" {
				return nil, err
			}
		}
		return x.show(s)
	case "CONTEXT":
		focus, err := x.ids(s.Refs)
		if err != nil {
			return nil, err
		}
		if s.Flags["set"] {
			c := brain.ContextChange{Depth: iptr(s.Opts["depth"]), Threshold: fptr(s.Opts["threshold"]), Hop: fptr(s.Opts["hop"])}
			if s.Str != "" && len(focus) == 0 {
				r, err := x.rb().Search(s.Str, "hybrid", 8)
				if err != nil {
					return nil, err
				}
				for i, row := range r.Rows {
					if i < 4 && row.Vector != nil && row.Depth != nil && *row.Depth == 0 {
						focus = append(focus, row.ID)
					}
				}
				if len(focus) == 0 && len(r.Rows) > 0 {
					focus = []string{r.Rows[0].ID}
				}
			}
			if len(focus) > 0 {
				f := brain.StrList(focus)
				c.Focus = &f
				if x.p != nil && !x.p.Admin {
					if err := x.real.CheckOps(x.p, x.r.dbName(), []brain.Op{{Op: "focus", IDs: focus}}); err != nil {
						return nil, x.didYouMean(err)
					}
				}
			}
			if _, err := b.SetContext(c); err != nil {
				return nil, x.didYouMean(err)
			}
			x.changes = append(x.changes, "focus="+strings.Join(b.Focus(), ","))
			x.touched = append(x.touched, b.Focus()...)
			f := s.Opts["format"]
			b = x.rb()
			if f == "md" {
				return &StmtResult{Kind: "text", Text: b.ContextMD()}, nil
			}
			if f == "" || f == "xml" {
				return &StmtResult{Kind: "text", Text: b.ContextXML()}, nil
			}
		}
		b = x.rb()
		cq := brain.ContextQuery{Focus: focus, Query: s.Str, Depth: iptr(s.Opts["depth"]), Threshold: fptr(s.Opts["threshold"]), Hop: fptr(s.Opts["hop"]),
			Format: s.Opts["format"], Sources: s.Words}
		r, err := b.QueryContext(cq)
		if err != nil {
			return nil, x.didYouMean(err)
		}
		return &StmtResult{Kind: "text", Text: r.Output, IDs: r.Focus}, nil
	case "SELECT":
		spec, err := x.selectSpec(s.Sel)
		if err != nil {
			return nil, err
		}
		r, err := b.Select(spec)
		if err != nil {
			return nil, err
		}
		return rowsResult(r.Columns, r.Rows, r.IDs, s.Sel.Format), nil
	case "SEARCH":
		k := 8
		if n := iptr(s.Opts["limit"]); n != nil {
			k = *n
		}
		b = x.rb()
		r, err := b.Search(s.Str, s.Opts["mode"], k)
		if err != nil {
			return nil, err
		}
		if s.Flags["focus"] {
			x.real.SearchFocus(r)
			x.changes = append(x.changes, "focus="+strings.Join(b.Focus(), ","))
		}
		cols := []string{"id", "kind", "score", "text"}
		rows := []*brain.OMap{}
		ids := []string{}
		add := func(row brain.SearchRow, extra string) {
			m := brain.NewOMap().Set("id", row.ID)
			n := nodeLite(b, row.ID)
			m.Set("kind", n[0]).Set("score", round4(row.Score)).Set("text", n[1])
			if extra != "" {
				m.Set("via", extra)
			}
			rows = append(rows, m)
			ids = append(ids, row.ID)
		}
		for _, row := range r.Rows {
			v := row.Via
			if row.Parent != nil {
				v = *row.Parent
			}
			add(row, v)
		}
		for _, row := range r.Via {
			add(row, "via "+row.Via)
		}
		hasVia := false
		for _, m := range rows {
			if _, ok := m.Vals["via"]; ok {
				hasVia = true
			}
		}
		if hasVia {
			cols = append(cols, "via")
			for _, m := range rows {
				if _, ok := m.Vals["via"]; !ok {
					m.Set("via", "")
				}
			}
		}
		return rowsResult(cols, rows, ids, s.Opts["format"]), nil
	case "QUERY":
		q, err := brain.ParseQuery(s.Str)
		if err != nil {
			return nil, err
		}
		f := s.Opts["format"]
		if f == "" {
			f = q.Format
		}
		r, err := b.RunQuery(q, "md")
		if err != nil {
			return nil, err
		}
		return rowsResult(r.Columns, r.Rows, nil, f), nil
	case "INSPECT", "TEST", "SUGGEST", "DISCOVER", "PATHS":
		return x.inspect(s)
	case "EXPLAIN":
		if s.Sub == "ACCESS" {
			return x.accessStmt(s)
		}
		return x.explain(s.Inner)
	case "REMEMBER":
		o := brain.Op{Op: "remember", Kind: s.Name, Text: sp(s.Str), Section: s.Opts["section"], Concepts: s.Strs, Pin: s.Flags["pin"], Labels: s.Words,
			Override: s.Opts["override"]}
		for _, l := range s.Links {
			id, err := x.id(l.Ref)
			if err != nil {
				return nil, err
			}
			o.Links = append(o.Links, brain.LinkSpec{To: id, W: l.Weight})
		}
		if len(s.Refs) > 0 {
			id, err := x.id(s.Refs[0])
			if err != nil {
				return nil, err
			}
			o.Supersedes = id
		}
		for _, f := range s.Fields {
			v, err := x.val(f.Val)
			if err != nil {
				return nil, err
			}
			o.Fields = o.Fields.Set(strings.TrimPrefix(f.Col, "fields."), v)
		}
		if s.Flags["focus"] {
			o.Focus = json.RawMessage("true")
		}
		r, err := x.ops([]brain.Op{o})
		if err != nil {
			return nil, err
		}
		if s.As != "" {
			switch {
			case len(r.Created) > 0:
				x.vars[s.As] = r.Created[0]
			case len(r.Quarantined) > 0:
				x.vars[s.As] = r.Quarantined[0].ID
			case len(r.Rejected) > 0:
				if x.rejVars == nil {
					x.rejVars = map[string]string{}
				}
				x.rejVars[s.As] = fmt.Sprintf("score %.2f", r.Rejected[0].Score)
			}
		}
		return changesOf(r), nil
	case "INSERT":
		ops := []brain.Op{}
		for _, row := range s.Rows {
			o := brain.Op{Op: "remember", Kind: s.Name, Labels: s.Words, Override: s.Opts["override"], Origin: "insert"}
			for i, c := range s.Cols {
				v, err := x.val(row[i])
				if err != nil {
					return nil, err
				}
				switch strings.ToLower(c) {
				case "text":
					o.Text = sp(v)
				case "section":
					o.Section = v
				case "concepts":
					o.Concepts = splitList(v)
				default:
					o.Fields = o.Fields.Set(strings.TrimPrefix(c, "fields."), v)
				}
			}
			if o.Text == nil {
				parts := []string{}
				for _, f := range o.Fields {
					parts = append(parts, f.Name+": "+f.Value)
				}
				o.Text = sp(strings.Join(parts, " · "))
			}
			ops = append(ops, o)
		}
		r, err := x.ops(ops)
		if err != nil {
			return nil, err
		}
		return changesOf(r), nil
	case "UPDATE":
		if s.Name != "memory" && s.Name != "connectors" {
			return nil, fmt.Errorf("UPDATE works on memory or connectors (got %s)", s.Name)
		}
		if s.Where == nil {
			return nil, errors.New("UPDATE needs a WHERE clause (bulk guard)")
		}
		ids, err := x.matchIDsFrom(s.Name, s.Where)
		if err != nil {
			return nil, err
		}
		if s.Flags["dry"] {
			return &StmtResult{Kind: "text", Text: fmt.Sprintf("dry run: would update %d items: %s", len(ids), strings.Join(firstN(ids, 50), ", ")), IDs: ids}, nil
		}
		if len(ids) > BulkLimit && !s.Flags["confirm"] {
			return nil, fmt.Errorf("UPDATE would change %d items (> %d): add CONFIRM, or DRY RUN to review", len(ids), BulkLimit)
		}
		if len(ids) == 0 {
			return &StmtResult{Kind: "changes", Changes: []string{"no rows matched"}}, nil
		}
		ops := []brain.Op{}
		for _, id := range ids {
			o := brain.Op{Op: "edit", ID: id}
			for _, a := range s.Fields {
				v, err := x.val(a.Val)
				if err != nil {
					return nil, err
				}
				switch strings.ToLower(a.Col) {
				case "text", "label":
					o.Text = sp(v)
				case "section":
					o.Section = v
				case "kind":
					o.Kind = v
				case "type":
					if s.Name != "connectors" {
						return nil, errors.New("SET type works on connectors (use kind for memory)")
					}
					o.CType = strings.ToLower(v)
				case "key":
					o.Key = v
				default:
					o.Fields = o.Fields.Set(strings.TrimPrefix(a.Col, "fields."), v)
				}
			}
			ops = append(ops, o)
		}
		r, err := x.ops(ops)
		if err != nil {
			return nil, err
		}
		return changesOf(r), nil
	case "CONCEPT":
		ms, err := x.ids(s.Refs)
		if err != nil {
			return nil, err
		}
		r, err := x.ops([]brain.Op{{Op: "concept", Label: s.Str, Members: ms, Related: s.Strs}})
		if err != nil {
			return nil, err
		}
		if s.As != "" && len(r.Created) > 0 {
			x.vars[s.As] = r.Created[0]
		}
		return changesOf(r), nil
	case "LINK", "UNLINK", "CONNECT", "DISCONNECT":
		ids, err := x.ids(s.Refs)
		if err != nil {
			return nil, err
		}
		op := strings.ToLower(s.Kind)
		r, err := x.ops([]brain.Op{{Op: op, A: ids[0], B: ids[1], W: fptr(s.Opts["weight"]), Type: s.Opts["type"]}})
		if err != nil {
			return nil, err
		}
		return changesOf(r), nil
	case "BOOST", "WEAKEN", "PIN", "UNPIN":
		ids, err := x.ids(s.Refs)
		if err != nil {
			return nil, err
		}
		ops := []brain.Op{}
		for _, id := range ids {
			ops = append(ops, brain.Op{Op: strings.ToLower(s.Kind), ID: id})
		}
		r, err := x.ops(ops)
		if err != nil {
			return nil, err
		}
		return changesOf(r), nil
	case "SUPERSEDE":
		ids, err := x.ids(s.Refs)
		if err != nil {
			return nil, err
		}
		r, err := x.ops([]brain.Op{{Op: "supersede", Old: ids[0], New: ids[1]}})
		if err != nil {
			return nil, err
		}
		return changesOf(r), nil
	case "FORGET":
		var ids []string
		var err error
		if s.Sub == "WHERE" {
			if ids, err = x.matchIDs(s.Where); err != nil {
				return nil, err
			}
		} else if ids, err = x.ids(s.Refs); err != nil {
			return nil, err
		}
		if s.Flags["dry"] {
			return &StmtResult{Kind: "text", Text: fmt.Sprintf("dry run: would forget %d items: %s", len(ids), strings.Join(firstN(ids, 50), ", ")), IDs: ids}, nil
		}
		if s.Sub == "WHERE" && len(ids) > BulkLimit && !s.Flags["confirm"] {
			return nil, fmt.Errorf("FORGET would delete %d items (> %d): add CONFIRM, or DRY RUN to review", len(ids), BulkLimit)
		}
		if len(ids) == 0 {
			return &StmtResult{Kind: "changes", Changes: []string{"no rows matched"}}, nil
		}
		ops := []brain.Op{}
		for _, id := range ids {
			ops = append(ops, brain.Op{Op: "forget", ID: id, Force: s.Flags["force"]})
		}
		r, err := x.ops(ops)
		if err != nil {
			return nil, err
		}
		return changesOf(r), nil
	case "FOCUS":
		var o brain.Op
		ids, err := x.ids(s.Refs)
		if err != nil {
			return nil, err
		}
		switch s.Sub {
		case "ADD":
			o = brain.Op{Op: "focus", Add: ids}
		case "REMOVE":
			o = brain.Op{Op: "focus", Remove: ids}
		case "GOAL":
			if _, err := b.SetContext(brain.ContextChange{Focus: &brain.StrList{"goal"}}); err != nil {
				return nil, err
			}
			x.changes = append(x.changes, "focus=goal")
			return &StmtResult{Kind: "changes", Changes: []string{"focus=" + strings.Join(b.Focus(), ",")}}, nil
		default:
			o = brain.Op{Op: "focus", IDs: ids}
		}
		r, err := x.ops([]brain.Op{o})
		if err != nil {
			return nil, err
		}
		return changesOf(r), nil
	case "GOAL":
		o := brain.Op{Op: "goal", Text: sp(s.Str)}
		if len(s.Refs) > 0 {
			ids, err := x.ids(s.Refs)
			if err != nil {
				return nil, err
			}
			d, _ := json.Marshal(ids)
			o.Focus = d
		}
		r, err := x.ops([]brain.Op{o})
		if err != nil {
			return nil, err
		}
		return changesOf(r), nil
	case "FILL":
		return x.fill(s)
	case "INGEST":
		if x.p != nil && !x.p.Admin && !x.real.CanWriteAny(x.p, x.r.dbName()) {
			return nil, errors.New("access denied: no write grant")
		}
		r, err := b.IngestPath(s.Str, brain.IngestOptions{Focus: s.Flags["focus"]})
		if err != nil {
			return nil, err
		}
		x.changes = append(x.changes, fmt.Sprintf("ingest +%d", len(r.Fresh)))
		return &StmtResult{Kind: "text", Text: strings.Join(r.Report, "\n"), IDs: r.Fresh}, nil
	case "IMPORT":
		if x.r.Import != nil {
			out, err := x.r.Import(x.st, s.Str, s.Flags["update"], s.Flags["dry"])
			if err != nil {
				return nil, err
			}
			if !s.Flags["dry"] {
				x.changes = append(x.changes, "import "+s.Str)
			}
			return &StmtResult{Kind: "text", Text: out}, nil
		}
		if s.Flags["dry"] {
			return &StmtResult{Kind: "text", Text: "dry run: would ingest " + s.Str}, nil
		}
		r, err := b.IngestPath(s.Str, brain.IngestOptions{})
		if err != nil {
			return nil, err
		}
		x.changes = append(x.changes, fmt.Sprintf("import +%d", len(r.Fresh)))
		return &StmtResult{Kind: "text", Text: strings.Join(r.Report, "\n"), IDs: r.Fresh}, nil
	case "SOURCE":
		if s.Sub != "LS" {
			if err := needAdmin(); err != nil {
				return nil, err
			}
		}
		return x.source(s)
	case "KIND":
		if err := needAdmin(); err != nil {
			return nil, err
		}
		k := brain.NewKind{Name: s.Name, Color: s.Opts["color"], Prefix: s.Opts["prefix"], Section: s.Opts["section"], Label: s.Opts["label"], Base: fptr(s.Opts["base"])}
		if h := fptr(s.Opts["halfLife"]); h != nil {
			k.HalfLife = *h
		}
		key, err := b.AddKind(k)
		if err != nil {
			return nil, err
		}
		x.changes = append(x.changes, "+kind:"+key)
		return &StmtResult{Kind: "changes", Changes: []string{"+kind:" + key}}, nil
	case "LET":
		if s.Inner != nil {
			spec, err := x.selectSpec(s.Inner.Sel)
			if err != nil {
				return nil, err
			}
			r, err := b.Select(spec)
			if err != nil {
				return nil, err
			}
			if len(r.Rows) == 0 {
				return nil, fmt.Errorf("LET $%s: the subquery returned no rows", s.Name)
			}
			row := r.Rows[0]
			v := ""
			if len(row.Keys) > 0 {
				v = asStr(row.Vals[row.Keys[0]])
			}
			x.vars[s.Name] = v
			return &StmtResult{Kind: "text", Text: fmt.Sprintf("$%s = %s", s.Name, v)}, nil
		}
		v, err := x.val(*s.Val)
		if err != nil {
			return nil, err
		}
		x.vars[s.Name] = v
		return &StmtResult{Kind: "text", Text: fmt.Sprintf("$%s = %s", s.Name, v)}, nil
	case "CONNECTOR":
		ids, err := x.ids(s.Refs)
		if err != nil {
			return nil, err
		}
		o := brain.Op{Op: "connector", CType: s.Sub, Label: s.Str, Key: s.Opts["key"], Terms: s.Strs, Connect: ids, Gain: fptr(s.Opts["gain"])}
		if t, ok := s.Opts["transparent"]; ok {
			v := t == "true"
			o.Transparent = &v
		}
		r, err := x.ops([]brain.Op{o})
		if err != nil {
			return nil, err
		}
		if s.As != "" && len(r.Created) > 0 {
			x.vars[s.As] = r.Created[0]
		}
		return changesOf(r), nil
	case "RESOLVE":
		if err := needAdmin(); err != nil {
			return nil, err
		}
		min := 2
		if n := iptr(s.Opts["min"]); n != nil {
			min = *n
		}
		var r *brain.ResolveResult
		var err error
		if len(s.Words) > 0 {
			r, err = b.Resolve(s.Name, s.Words, min, s.Flags["dry"])
		} else {
			r, err = b.ResolveEntities(strings.ToLower(s.Name), min, s.Flags["dry"])
		}
		if err != nil {
			return nil, err
		}
		rows := []*brain.OMap{}
		for _, a := range r.Anchors {
			rows = append(rows, brain.NewOMap().Set("label", a.Label).Set("key", a.Key).Set("members", strings.Join(firstN(a.Members, 12), ",")).Set("count", len(a.Members)))
		}
		if !s.Flags["dry"] {
			for _, h := range r.Created {
				x.changes = append(x.changes, "+⧉"+h)
			}
			x.touched = append(x.touched, r.Created...)
		}
		return rowsResult([]string{"label", "key", "members", "count"}, rows, r.Created, ""), nil
	case "EXTRACT":
		var es []patterns.Entity
		if s.Str != "" || len(s.Refs) == 0 {
			es = b.ExtractText(s.Str, nil)
		} else {
			id, err := x.id(s.Refs[0])
			if err != nil {
				return nil, err
			}
			if es, err = b.Entities(id); err != nil {
				return nil, x.didYouMean(err)
			}
		}
		rows := []*brain.OMap{}
		for _, e := range es {
			rows = append(rows, brain.NewOMap().Set("type", e.Type).Set("value", e.Value).Set("raw", e.Raw).Set("confidence", e.Confidence))
		}
		return rowsResult([]string{"type", "value", "raw", "confidence"}, rows, nil, ""), nil
	case "PATTERN":
		if s.Sub != "TEST" {
			if err := needAdmin(); err != nil {
				return nil, err
			}
		}
		switch s.Sub {
		case "ADD":
			c := &patterns.Custom{Name: s.Name, Regex: s.Str, Type: s.Opts["type"], Normalize: s.Opts["normalize"], PII: s.Opts["pii"] == "true"}
			if err := b.AddPattern(c); err != nil {
				return nil, err
			}
			x.patDirty = true
			x.changes = append(x.changes, "+pattern:"+s.Name)
			return &StmtResult{Kind: "changes", Changes: []string{"+pattern:" + s.Name}}, nil
		case "REMOVE":
			if err := b.RemovePattern(s.Name); err != nil {
				return nil, err
			}
			x.patDirty = true
			x.changes = append(x.changes, "-pattern:"+s.Name)
			return &StmtResult{Kind: "changes", Changes: []string{"-pattern:" + s.Name}}, nil
		case "TEST":
			c := b.CustomPattern(s.Name)
			types := []string{s.Name}
			if c != nil {
				types = []string{c.Type, c.Name}
			}
			es := b.ExtractText(s.Str, types)
			rows := []*brain.OMap{}
			for _, e := range es {
				rows = append(rows, brain.NewOMap().Set("type", e.Type).Set("value", e.Value).Set("raw", e.Raw).Set("start", e.Start))
			}
			return rowsResult([]string{"type", "value", "raw", "start"}, rows, nil, ""), nil
		}
	}
	return nil, fmt.Errorf("statement %s is not supported", s.Kind)
}

func nodeLite(b *brain.Brain, id string) [2]string {
	k, t := b.Brief(id)
	return [2]string{k, t}
}

func round4(x float64) float64 {
	v, _ := strconv.ParseFloat(strconv.FormatFloat(x, 'f', 4, 64), 64)
	return v
}

func asStr(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case bool:
		return strconv.FormatBool(t)
	}
	return fmt.Sprint(v)
}

func splitList(s string) []string {
	out := []string{}
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

func firstN(xs []string, n int) []string {
	if len(xs) > n {
		return xs[:n]
	}
	return xs
}

func (x *execState) inspect(s *Stmt) (*StmtResult, error) {
	b := x.b
	switch s.Kind {
	case "DISCOVER":
		r, err := b.Inspect("", []string{"discover"}, "")
		if err != nil {
			return nil, err
		}
		return &StmtResult{Kind: "text", Text: r.Text}, nil
	case "SUGGEST":
		if s.Sub == "CONNECTORS" {
			n := 10
			if l := iptr(s.Opts["limit"]); l != nil {
				n = *l
			}
			cs := b.SuggestConnectors(n)
			rows := []*brain.OMap{}
			for _, c := range cs {
				rows = append(rows, brain.NewOMap().Set("type", c.Type).Set("label", c.Label).Set("key", c.Key).Set("members", strings.Join(firstN(c.Members, 10), ",")).Set("evidence", c.Evidence))
			}
			return rowsResult([]string{"type", "label", "key", "members", "evidence"}, rows, nil, ""), nil
		}
	}
	ids, err := x.ids(s.Refs)
	if err != nil {
		return nil, err
	}
	what, to := s.Words, ""
	switch s.Kind {
	case "TEST":
		what = []string{"test"}
	case "SUGGEST":
		what = []string{"suggest"}
	case "PATHS":
		what, to = []string{"paths"}, ids[1]
	}
	r, err := b.Inspect(ids[0], what, to)
	if err != nil {
		return nil, x.didYouMean(err)
	}
	return &StmtResult{Kind: "text", Text: r.Text, IDs: ids}, nil
}

func (x *execState) explain(s *Stmt) (*StmtResult, error) {
	if s == nil {
		return nil, errors.New("EXPLAIN needs a statement")
	}
	n, e := x.b.Counts()
	L := []string{"statement: " + s.Kind}
	if s.Sel != nil {
		from := s.Sel.From
		L = append(L, "table: "+from)
		if s.Sel.Where != nil {
			d, _ := json.Marshal(s.Sel.Where)
			L = append(L, "filter: "+string(d))
			L = append(L, fmt.Sprintf("access: full scan over %d nodes (no index matches this filter)", n))
		} else {
			L = append(L, fmt.Sprintf("access: full scan over %d nodes", n))
		}
		if s.Sel.Near != "" {
			m := s.Sel.Mode
			if m == "" {
				m = "hybrid"
			}
			L = append(L, fmt.Sprintf("ranking: NEAR %q (%s: vector seeds → graph activation over %d edges)", s.Sel.Near, m, e))
		}
		if len(s.Sel.GroupBy) > 0 {
			L = append(L, "group by: "+strings.Join(s.Sel.GroupBy, ", "))
		}
		if s.Sel.Limit > 0 {
			L = append(L, fmt.Sprintf("limit: %d", s.Sel.Limit))
		}
	}
	L = append(L, fmt.Sprintf("writes: %v", hasWrite(s)))
	d, _ := json.MarshalIndent(s, "", "  ")
	return &StmtResult{Kind: "plan", Text: strings.Join(L, "\n") + "\nast:\n" + string(d)}, nil
}

func (x *execState) fill(s *Stmt) (*StmtResult, error) {
	tpl, tf := s.Str, s.Opts["as"]
	if s.Sub == "TEMPLATE" {
		p, err := x.findTemplate(s.Str)
		if err != nil {
			return nil, err
		}
		d, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		tpl = string(d)
		if tf == "" {
			switch strings.ToLower(filepath.Ext(p)) {
			case ".json":
				tf = "json"
			case ".yaml", ".yml":
				tf = "yaml"
			case ".xml":
				tf = "xml"
			case ".md", ".markdown", ".txt":
				tf = "md"
			}
		}
	}
	req := brain.FillRequest{Template: tpl, TemplateFormat: tf, Output: s.Opts["output"], Scope: s.Opts["scope"], WithSources: s.Flags["sources"], Min: fptr(s.Opts["min"])}
	r, err := x.b.Fill(req)
	if err != nil {
		return nil, err
	}
	txt := r.Output
	if len(r.Missing) > 0 {
		txt += "\nmissing slots (" + strconv.Itoa(len(r.Missing)) + "): " + strings.Join(r.Missing, ", ")
	}
	ids := []string{}
	for _, v := range r.Values {
		if v.Source != "" {
			ids = append(ids, v.Source)
		}
	}
	return &StmtResult{Kind: "text", Text: txt, IDs: ids}, nil
}

func (x *execState) findTemplate(name string) (string, error) {
	if _, err := os.Stat(name); err == nil {
		return name, nil
	}
	dirs := []string{}
	if x.r.Templates != nil {
		dirs = x.r.Templates(x.st)
	} else {
		if x.st.Path != "" {
			dirs = append(dirs, filepath.Join(filepath.Dir(x.st.Path), "templates"))
		}
		m, _ := filepath.Glob("examples/*/templates")
		dirs = append(dirs, m...)
	}
	for _, d := range dirs {
		p := filepath.Join(d, name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("template %q not found (looked in %s)", name, strings.Join(dirs, ", "))
}

func (x *execState) source(s *Stmt) (*StmtResult, error) {
	b := x.b
	switch s.Sub {
	case "ADD":
		c := brain.SourceConfig{Name: s.Name, URI: s.Str, TokenEnv: s.Opts["tokenEnv"], Weight: fptr(s.Opts["weight"]), FederateDepth: iptr(s.Opts["depth"]), TTL: iptr(s.Opts["ttl"])}
		if err := b.AddSource(c); err != nil {
			return nil, err
		}
		x.changes = append(x.changes, "+source:"+s.Name)
		return &StmtResult{Kind: "changes", Changes: []string{"+source:" + s.Name}}, nil
	case "REMOVE":
		if err := b.RemoveSource(s.Name); err != nil {
			return nil, err
		}
		x.changes = append(x.changes, "-source:"+s.Name)
		return &StmtResult{Kind: "changes", Changes: []string{"-source:" + s.Name}}, nil
	case "SYNC":
		if x.r.SyncSrc == nil {
			return nil, errors.New("source sync is not available in this build")
		}
		t, err := x.r.SyncSrc(x.st, s.Name)
		if err != nil {
			return nil, err
		}
		return &StmtResult{Kind: "text", Text: t}, nil
	}
	return x.show(&Stmt{Name: "sources"})
}

func (x *execState) show(s *Stmt) (*StmtResult, error) {
	b := x.b
	toRows := func(v any) ([]string, []*brain.OMap) {
		d, _ := json.Marshal(v)
		gv, _ := brain.DecodeOrderedJSON(d)
		l, _ := gv.([]any)
		rows := []*brain.OMap{}
		cols := []string{}
		seen := map[string]bool{}
		for _, it := range l {
			if m, ok := it.(*brain.OMap); ok {
				for _, k := range m.Keys {
					if !seen[k] {
						seen[k] = true
						cols = append(cols, k)
					}
					if inner, ok := m.Vals[k].(*brain.OMap); ok {
						d, _ := json.Marshal(inner)
						m.Vals[k] = string(d)
					}
					if arr, ok := m.Vals[k].([]any); ok {
						ps := []string{}
						for _, a := range arr {
							ps = append(ps, fmt.Sprint(a))
						}
						m.Vals[k] = strings.Join(ps, ", ")
					}
				}
				rows = append(rows, m)
			}
		}
		return cols, rows
	}
	switch s.Name {
	case "kinds":
		c, r := toRows(b.State().Kinds)
		return rowsResult(c, r, nil, ""), nil
	case "sections":
		return rowsResult([]string{"name", "items"}, b.Sections(), nil, ""), nil
	case "concepts", "connectors":
		spec := &brain.SelectSpec{From: s.Name, MaxRows: MaxRows, Order: []brain.OrderKey{{Col: "degree", Desc: true}}}
		r, err := b.Select(spec)
		if err != nil {
			return nil, err
		}
		return rowsResult(r.Columns, r.Rows, r.IDs, ""), nil
	case "sources":
		c, r := toRows(b.SourceStatus())
		if len(c) == 0 {
			c = []string{"name", "uri", "status"}
		}
		return rowsResult(c, r, nil, ""), nil
	case "dbs":
		if x.r.ShowDBs == nil {
			n, e := b.Counts()
			return rowsResult([]string{"name", "nodes", "edges", "step"}, []*brain.OMap{brain.NewOMap().Set("name", x.st.Name).Set("nodes", n).Set("edges", e).Set("step", b.CurrentStep())}, nil, ""), nil
		}
		c, r := toRows(x.r.ShowDBs())
		return rowsResult(c, r, nil, ""), nil
	case "stats":
		st := b.State().Stats
		d, _ := json.Marshal(st)
		gv, _ := brain.DecodeOrderedJSON(d)
		m := gv.(*brain.OMap)
		rows := []*brain.OMap{}
		for _, k := range m.Keys {
			rows = append(rows, brain.NewOMap().Set("metric", k).Set("value", fmt.Sprint(m.Vals[k])))
		}
		n, e := b.Counts()
		rows = append(rows, brain.NewOMap().Set("metric", "nodes").Set("value", strconv.Itoa(n)), brain.NewOMap().Set("metric", "edges").Set("value", strconv.Itoa(e)))
		return rowsResult([]string{"metric", "value"}, rows, nil, ""), nil
	case "config":
		if x.r.ShowConf == nil {
			return &StmtResult{Kind: "text", Text: "no braindb.yaml configuration loaded"}, nil
		}
		v := x.r.ShowConf(x.st)
		if t, ok := v.(string); ok {
			return &StmtResult{Kind: "text", Text: t}, nil
		}
		return &StmtResult{Kind: "text", Text: brain.EmitYAML(v)}, nil
	case "templates":
		dirs := []string{}
		if x.r.Templates != nil {
			dirs = x.r.Templates(x.st)
		} else {
			if x.st.Path != "" {
				dirs = append(dirs, filepath.Join(filepath.Dir(x.st.Path), "templates"))
			}
			m, _ := filepath.Glob("examples/*/templates")
			dirs = append(dirs, m...)
		}
		rows := []*brain.OMap{}
		for _, d := range dirs {
			es, _ := os.ReadDir(d)
			for _, e := range es {
				if !e.IsDir() {
					rows = append(rows, brain.NewOMap().Set("name", e.Name()).Set("path", filepath.Join(d, e.Name())))
				}
			}
		}
		return rowsResult([]string{"name", "path"}, rows, nil, ""), nil
	case "patterns":
		c, r := toRows(b.Patterns())
		return rowsResult(c, r, nil, ""), nil
	case "pii":
		cnt := b.PIICounts()
		keys := []string{}
		for k := range cnt {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		rows := []*brain.OMap{}
		for _, k := range keys {
			rows = append(rows, brain.NewOMap().Set("type", k).Set("count", cnt[k]))
		}
		return rowsResult([]string{"type", "count"}, rows, nil, ""), nil
	}
	return nil, fmt.Errorf("SHOW %s: expected kinds, sections, concepts, connectors, sources, dbs, stats, config, templates, patterns or pii", s.Name)
}

// ---------------------------------------------------------------- rendering

// Render renders a script result for MCP/CLI (rows as Markdown or the requested format, then the status line).
func (r *Result) Render(defaultFormat string) string {
	if defaultFormat == "" {
		defaultFormat = "md"
	}
	parts := []string{}
	multi := len(r.Results) > 1
	for _, s := range r.Results {
		head := ""
		if multi && s.Statement != "" {
			st := strings.Join(strings.Fields(s.Statement), " ")
			if len([]rune(st)) > 80 {
				st = string([]rune(st)[:79]) + "…"
			}
			head = fmt.Sprintf("-- [%d] %s\n", s.Index, st)
		}
		switch s.Kind {
		case "rows":
			f := s.Format
			if f == "" {
				f = defaultFormat
			}
			out, err := brain.RenderRows(s.Columns, s.Rows, f)
			if err != nil {
				out = err.Error()
			}
			if len(s.Rows) == 0 {
				out = "(no rows)"
			}
			parts = append(parts, head+strings.TrimRight(out, "\n"))
		case "changes":
			parts = append(parts, head+"changes: "+strings.Join(s.Changes, " "))
		default:
			parts = append(parts, head+strings.TrimRight(s.Text, "\n"))
		}
	}
	out := strings.Join(parts, "\n\n")
	if r.Error != "" {
		if out != "" {
			out += "\n\n"
		}
		out += "[error] " + r.Error
	}
	if r.Status != "" {
		out += "\n" + r.Status
	}
	return strings.TrimLeft(out, "\n")
}

func (x *execState) canReview() bool {
	return x.p == nil || x.p.Admin || x.p.Can("review", "*", "", "", x.r.dbName())
}

// purposeExempt reports whether the principal holds a role exempt from the purpose filter. The implicit local
// operator (no named user) is filtered too, so imports and examples show real decisions.
func (x *execState) purposeExempt() bool {
	p := x.real.Purpose()
	if p == nil || x.p == nil || x.p == brain.AdminPrincipal {
		return false
	}
	for _, r := range x.p.Roles {
		for _, e := range p.Admission.Exempt {
			if r == e {
				return true
			}
		}
	}
	return false
}
