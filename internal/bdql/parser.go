// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package bdql

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Gabski/brainDB/internal/brain"
)

// Statement keywords (for errors and did-you-mean).
var stmtKeywords = []string{"USE", "SHOW", "CONTEXT", "SELECT", "SEARCH", "QUERY", "INSPECT", "TEST", "SUGGEST", "DISCOVER", "PATHS",
	"EXPLAIN", "REMEMBER", "INSERT", "UPDATE", "CONCEPT", "LINK", "UNLINK", "BOOST", "WEAKEN", "PIN", "UNPIN", "SUPERSEDE", "FORGET",
	"FOCUS", "GOAL", "FILL", "INGEST", "IMPORT", "SOURCE", "KIND", "BEGIN", "COMMIT", "ROLLBACK", "LET", "CONNECTOR", "CONNECT",
	"DISCONNECT", "RESOLVE", "EXTRACT", "PATTERN", "WHOAMI", "CREATE", "ALTER", "DROP", "GRANT", "REVOKE", "DENY", "LABEL", "AS", "CHECK", "ADMIT", "REJECT", "STRICT"}

var keywords = map[string]bool{}

func init() {
	for _, k := range stmtKeywords {
		keywords[k] = true
	}
	for _, k := range strings.Fields(`FROM WHERE AND OR NOT IN BETWEEN HAS NEAR MODE ORDER GROUP BY ASC DESC LIMIT OFFSET FORMAT FOR DEPTH
		THRESHOLD HOP SOURCES SET WITH TO AS CONCEPTS FIELDS SUPERSEDES WEIGHT TYPE INTO VALUES MEMBERS RELATED FORCE DRY RUN CONFIRM
		ADD REMOVE TEMPLATE OUTPUT SCOPE MIN TOKEN ENV SYNC COLOR HALF LIFE BASE PREFIX SECTION KEY TERMS GAIN TRANSPARENT OPAQUE
		NORMALIZE PII TTL LABEL LS LIST CONNECTORS TRANSACTION LABELS USER ROLE ROLES DBS ON ACCESS DISABLE ENABLE KIND OVERRIDE PURPOSE REASON`) {
		keywords[k] = true
	}
}

// Ref is an id or a variable reference.
type Ref struct {
	ID  string `json:"id,omitempty"`
	Var string `json:"var,omitempty"`
}

func (r Ref) String() string {
	if r.Var != "" {
		return "$" + r.Var
	}
	return r.ID
}

// Value is a literal or variable.
type Value struct {
	Text string `json:"text"`
	Kind string `json:"kind"` // string | number | word | bool | var
}

// Expr is a WHERE predicate node.
type Expr interface{ isExpr() }

// BinExpr is AND / OR.
type BinExpr struct {
	Op   string `json:"op"`
	L, R Expr
}

// NotExpr negates.
type NotExpr struct{ X Expr }

// CmpExpr compares a column with a value.
type CmpExpr struct {
	Col string `json:"col"`
	Op  string `json:"op"`
	Val Value  `json:"val"`
}

// InExpr is col [NOT] IN (…).
type InExpr struct {
	Col  string  `json:"col"`
	Vals []Value `json:"vals"`
	Not  bool    `json:"not,omitempty"`
}

// BetweenExpr is col BETWEEN lo AND hi.
type BetweenExpr struct {
	Col    string `json:"col"`
	Lo, Hi Value
}

// HasExpr is HAS <entity type>.
type HasExpr struct {
	Type string `json:"type"`
}

// BareExpr is a boolean column (lit, pinned, …).
type BareExpr struct {
	Col string `json:"col"`
}

func (BinExpr) isExpr()     {}
func (NotExpr) isExpr()     {}
func (CmpExpr) isExpr()     {}
func (InExpr) isExpr()      {}
func (BetweenExpr) isExpr() {}
func (HasExpr) isExpr()     {}
func (BareExpr) isExpr()    {}

// Assign is SET col = value.
type Assign struct {
	Col string `json:"col"`
	Val Value  `json:"val"`
}

// LinkTo is one LINK TO target.
type LinkTo struct {
	Ref    Ref      `json:"ref"`
	Weight *float64 `json:"weight,omitempty"`
}

// SelectStmt is a SELECT.
type SelectStmt struct {
	Columns []brain.SelCol   `json:"columns"`
	From    string           `json:"from"`
	Where   Expr             `json:"where,omitempty"`
	Near    string           `json:"near,omitempty"`
	Mode    string           `json:"mode,omitempty"`
	GroupBy []string         `json:"groupBy,omitempty"`
	Order   []brain.OrderKey `json:"orderBy,omitempty"`
	Limit   int              `json:"limit,omitempty"`
	Offset  int              `json:"offset,omitempty"`
	Format  string           `json:"format,omitempty"`
}

// Stmt is one parsed statement.
type Stmt struct {
	Kind  string `json:"kind"`
	Sub   string `json:"sub,omitempty"` // sub-command (ADD, REMOVE, TEMPLATE, …)
	Index int    `json:"index"`
	Line  int    `json:"line"`
	Col   int    `json:"col"`
	Src   string `json:"statement"`
	Write bool   `json:"write"`

	Sel    *SelectStmt       `json:"select,omitempty"`
	Refs   []Ref             `json:"refs,omitempty"`
	Refs2  []Ref             `json:"refs2,omitempty"`
	Strs   []string          `json:"strings,omitempty"`
	Words  []string          `json:"words,omitempty"`
	Str    string            `json:"text,omitempty"`
	Name   string            `json:"name,omitempty"`
	As     string            `json:"as,omitempty"`
	Links  []LinkTo          `json:"links,omitempty"`
	Fields []Assign          `json:"fields,omitempty"`
	Cols   []string          `json:"columns,omitempty"`
	Rows   [][]Value         `json:"rows,omitempty"`
	Where  Expr              `json:"where,omitempty"`
	Opts   map[string]string `json:"options,omitempty"`
	Flags  map[string]bool   `json:"flags,omitempty"`
	Inner  *Stmt             `json:"inner,omitempty"`
	Val    *Value            `json:"value,omitempty"`
}

type parser struct {
	src  string
	toks []token
	p    int
	stmt int
}

// Parse parses a script into statements.
func Parse(src string) ([]*Stmt, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	ps := &parser{src: src, toks: toks}
	out := []*Stmt{}
	for {
		for ps.isPunct(";") {
			ps.p++
		}
		if ps.peek().kind == tEOF {
			return out, nil
		}
		ps.stmt = len(out) + 1
		start := ps.peek()
		s, err := ps.statement()
		if err != nil {
			return nil, err
		}
		end := ps.toks[ps.p-1].end
		if !ps.isPunct(";") && ps.peek().kind != tEOF {
			return nil, ps.errAt(ps.peek(), fmt.Sprintf("expected ; after %s, got %s", s.Kind, describe(ps.peek())))
		}
		s.Index, s.Line, s.Col = len(out)+1, start.line, start.col
		s.Src = strings.TrimSpace(src[start.off:end])
		out = append(out, s)
	}
}

func describe(t token) string {
	switch t.kind {
	case tEOF:
		return "end of input"
	case tString:
		return fmt.Sprintf("string %q", t.text)
	}
	return fmt.Sprintf("%q", t.text)
}

func (ps *parser) peek() token { return ps.toks[ps.p] }
func (ps *parser) peekN(n int) token {
	if ps.p+n < len(ps.toks) {
		return ps.toks[ps.p+n]
	}
	return ps.toks[len(ps.toks)-1]
}

func (ps *parser) errAt(t token, msg string) error {
	return &Error{Stmt: ps.stmt, Line: t.line, Col: t.col, Msg: msg, Excerpt: excerpt(ps.src, t.line, t.col)}
}

func (ps *parser) isKw(kws ...string) bool {
	t := ps.peek()
	if t.kind != tIdent {
		return false
	}
	u := strings.ToUpper(t.text)
	for _, k := range kws {
		if u == k {
			return true
		}
	}
	return false
}

func (ps *parser) acceptKw(kws ...string) bool {
	if ps.isKw(kws...) {
		ps.p++
		return true
	}
	return false
}

func (ps *parser) expectKw(kws ...string) error {
	if ps.acceptKw(kws...) {
		return nil
	}
	t := ps.peek()
	msg := fmt.Sprintf("expected %s, got %s", strings.Join(kws, " or "), describe(t))
	if t.kind == tIdent {
		if c := brain.Closest(strings.ToUpper(t.text), kws, 2); c != "" {
			msg += fmt.Sprintf(" (did you mean %s?)", c)
		}
	}
	return ps.errAt(t, msg)
}

func (ps *parser) isPunct(p string) bool {
	t := ps.peek()
	return t.kind == tPunct && t.text == p
}

func (ps *parser) expectPunct(p string) error {
	if ps.isPunct(p) {
		ps.p++
		return nil
	}
	return ps.errAt(ps.peek(), fmt.Sprintf("expected %q, got %s", p, describe(ps.peek())))
}

func (ps *parser) atEnd() bool { return ps.isPunct(";") || ps.peek().kind == tEOF }

func (ps *parser) str() (string, error) {
	t := ps.peek()
	if t.kind != tString {
		return "", ps.errAt(t, "expected a quoted string, got "+describe(t))
	}
	ps.p++
	return t.text, nil
}

func (ps *parser) word() (string, error) {
	t := ps.peek()
	if t.kind == tIdent || t.kind == tString {
		ps.p++
		return t.text, nil
	}
	return "", ps.errAt(t, "expected a name, got "+describe(t))
}

func (ps *parser) number() (float64, error) {
	t := ps.peek()
	if t.kind != tNumber {
		return 0, ps.errAt(t, "expected a number, got "+describe(t))
	}
	ps.p++
	v, err := strconv.ParseFloat(strings.TrimRight(t.text, "%kKmMbn"), 64)
	if err != nil {
		return 0, ps.errAt(t, "bad number "+t.text)
	}
	return v, nil
}

func (ps *parser) integer() (int, error) {
	v, err := ps.number()
	return int(v), err
}

func (ps *parser) ref() (Ref, error) {
	t := ps.peek()
	switch t.kind {
	case tVar:
		ps.p++
		return Ref{Var: t.text}, nil
	case tIdent, tString:
		ps.p++
		return Ref{ID: t.text}, nil
	case tNumber:
		ps.p++
		return Ref{ID: t.text}, nil
	}
	return Ref{}, ps.errAt(t, "expected an id or $variable, got "+describe(t))
}

func (ps *parser) refs() ([]Ref, error) {
	out := []Ref{}
	for {
		r, err := ps.ref()
		if err != nil {
			return nil, err
		}
		out = append(out, r)
		if !ps.isPunct(",") {
			return out, nil
		}
		ps.p++
	}
}

func (ps *parser) strs() ([]string, error) {
	out := []string{}
	for {
		s, err := ps.word()
		if err != nil {
			return nil, err
		}
		out = append(out, s)
		if !ps.isPunct(",") {
			return out, nil
		}
		ps.p++
	}
}

func (ps *parser) words() ([]string, error) {
	out := []string{}
	for {
		t := ps.peek()
		if t.kind == tPunct && t.text == "*" {
			ps.p++
			out = append(out, "*")
		} else {
			w, err := ps.word()
			if err != nil {
				return nil, err
			}
			out = append(out, w)
		}
		if !ps.isPunct(",") {
			return out, nil
		}
		ps.p++
	}
}

func (ps *parser) value() (Value, error) {
	t := ps.peek()
	switch t.kind {
	case tString:
		ps.p++
		return Value{t.text, "string"}, nil
	case tNumber:
		ps.p++
		return Value{t.text, "number"}, nil
	case tVar:
		ps.p++
		return Value{t.text, "var"}, nil
	case tIdent:
		ps.p++
		switch strings.ToLower(t.text) {
		case "true", "false":
			return Value{strings.ToLower(t.text), "bool"}, nil
		case "null":
			return Value{"", "string"}, nil
		}
		return Value{t.text, "word"}, nil
	}
	return Value{}, ps.errAt(t, "expected a value, got "+describe(t))
}

func (ps *parser) setOpt(s *Stmt, k, v string) {
	if s.Opts == nil {
		s.Opts = map[string]string{}
	}
	s.Opts[k] = v
}

func (ps *parser) setFlag(s *Stmt, k string) {
	if s.Flags == nil {
		s.Flags = map[string]bool{}
	}
	s.Flags[k] = true
}

func (ps *parser) dryConfirm(s *Stmt) error {
	for {
		switch {
		case ps.acceptKw("DRY"):
			if err := ps.expectKw("RUN"); err != nil {
				return err
			}
			ps.setFlag(s, "dry")
		case ps.acceptKw("CONFIRM"):
			ps.setFlag(s, "confirm")
		case ps.acceptKw("FORCE"):
			ps.setFlag(s, "force")
		default:
			return nil
		}
	}
}

// statement parses one statement.
func (ps *parser) statement() (*Stmt, error) {
	t := ps.peek()
	if t.kind != tIdent {
		return nil, ps.errAt(t, "expected a statement keyword, got "+describe(t))
	}
	kw := strings.ToUpper(t.text)
	s := &Stmt{Kind: kw}
	ps.p++
	var err error
	switch kw {
	case "USE":
		s.Name, err = ps.word()
	case "SHOW":
		s.Name, err = ps.word()
		s.Name = strings.ToLower(s.Name)
		if err == nil && ps.acceptKw("FOR") {
			var u string
			u, err = ps.word()
			ps.setOpt(s, "for", u)
		}
	case "WHOAMI":
	case "STRICT":
		err = ps.expectKw("PURPOSE")
	case "CHECK":
		if s.Str, err = ps.str(); err != nil {
			return nil, err
		}
		for !ps.atEnd() && err == nil {
			switch {
			case ps.acceptKw("AS"):
				var k string
				k, err = ps.word()
				ps.setOpt(s, "kind", k)
			case ps.acceptKw("IN"):
				var k string
				k, err = ps.word()
				ps.setOpt(s, "section", k)
			default:
				err = ps.expectKw("AS", "IN")
			}
		}
	case "ADMIT":
		s.Write = true
		if s.Refs, err = ps.refs(); err != nil {
			return nil, err
		}
		if ps.acceptKw("IN") {
			var sec string
			sec, err = ps.word()
			ps.setOpt(s, "section", sec)
		}
	case "REJECT":
		s.Write = true
		if s.Refs, err = ps.refs(); err != nil {
			return nil, err
		}
		if ps.acceptKw("REASON") {
			var r string
			r, err = ps.str()
			ps.setOpt(s, "reason", r)
		}
	case "AS":
		if err = ps.expectKw("USER"); err != nil {
			return nil, err
		}
		s.Name, err = ps.word()
	case "CREATE":
		err = ps.createStmt(s)
	case "ALTER":
		s.Write = true
		if err = ps.expectKw("USER"); err != nil {
			return nil, err
		}
		if s.Name, err = ps.word(); err != nil {
			return nil, err
		}
		for !ps.atEnd() && err == nil {
			switch {
			case ps.acceptKw("ADD", "REMOVE"):
				op := strings.ToUpper(ps.toks[ps.p-1].text)
				var what, v string
				if what, err = ps.word(); err != nil {
					return nil, err
				}
				v, err = ps.word()
				s.Words = append(s.Words, op+" "+strings.ToUpper(what)+" "+v)
			case ps.acceptKw("DISABLE"):
				s.Words = append(s.Words, "DISABLE")
			case ps.acceptKw("ENABLE"):
				s.Words = append(s.Words, "ENABLE")
			case ps.acceptKw("TOKEN"):
				if err = ps.expectKw("ENV"); err != nil {
					return nil, err
				}
				var v string
				v, err = ps.word()
				s.Words = append(s.Words, "TOKEN "+v)
			default:
				err = ps.expectKw("ADD", "REMOVE", "DISABLE", "ENABLE", "TOKEN")
			}
		}
	case "DROP":
		s.Write = true
		var what string
		if what, err = ps.word(); err != nil {
			return nil, err
		}
		s.Sub = strings.ToUpper(what)
		if s.Sub != "USER" && s.Sub != "ROLE" {
			return nil, ps.errAt(ps.toks[ps.p-1], "expected DROP USER or DROP ROLE")
		}
		s.Name, err = ps.word()
	case "GRANT", "REVOKE", "DENY":
		err = ps.grantStmt(s, kw)
	case "LABEL":
		s.Write = true
		if ps.acceptKw("WHERE") {
			s.Sub = "WHERE"
			if s.Where, err = ps.expr(); err != nil {
				return nil, err
			}
		} else if s.Refs, err = ps.refs(); err != nil {
			return nil, err
		}
		if ps.acceptKw("ADD") {
			ps.setOpt(s, "op", "add")
		} else if ps.acceptKw("REMOVE") {
			ps.setOpt(s, "op", "remove")
		} else {
			return nil, ps.expectKw("ADD", "REMOVE")
		}
		if s.Strs, err = ps.strs(); err != nil {
			return nil, err
		}
		err = ps.dryConfirm(s)
		if s.Flags["dry"] {
			s.Write = false
		}
	case "CONTEXT":
		err = ps.contextStmt(s)
	case "SELECT":
		s.Sel, err = ps.selectStmt()
	case "SEARCH":
		if s.Str, err = ps.str(); err != nil {
			return nil, err
		}
		for !ps.atEnd() && err == nil {
			switch {
			case ps.acceptKw("MODE"):
				var m string
				m, err = ps.word()
				ps.setOpt(s, "mode", strings.ToLower(m))
			case ps.acceptKw("LIMIT"):
				var n int
				n, err = ps.integer()
				ps.setOpt(s, "limit", strconv.Itoa(n))
			case ps.acceptKw("SOURCES"):
				s.Words, err = ps.words()
			case ps.acceptKw("FOCUS"):
				ps.setFlag(s, "focus")
				s.Write = true
			case ps.acceptKw("FORMAT"):
				var f string
				f, err = ps.word()
				ps.setOpt(s, "format", strings.ToLower(f))
			default:
				err = ps.expectKw("MODE", "LIMIT", "SOURCES", "FOCUS", "FORMAT")
			}
		}
	case "QUERY":
		if s.Str, err = ps.str(); err != nil {
			return nil, err
		}
		if ps.acceptKw("FORMAT") {
			var f string
			f, err = ps.word()
			ps.setOpt(s, "format", strings.ToLower(f))
		}
	case "INSPECT":
		var r Ref
		if r, err = ps.ref(); err != nil {
			return nil, err
		}
		s.Refs = []Ref{r}
		if ps.acceptKw("WITH") {
			s.Words, err = ps.words()
		}
	case "TEST":
		var r Ref
		r, err = ps.ref()
		s.Refs = []Ref{r}
	case "SUGGEST":
		if ps.acceptKw("CONNECTORS") {
			s.Sub = "CONNECTORS"
			if ps.acceptKw("LIMIT") {
				var n int
				n, err = ps.integer()
				ps.setOpt(s, "limit", strconv.Itoa(n))
			}
		} else {
			var r Ref
			r, err = ps.ref()
			s.Refs = []Ref{r}
		}
	case "DISCOVER":
	case "PATHS":
		if err = ps.expectKw("FROM"); err != nil {
			return nil, err
		}
		var a, b Ref
		if a, err = ps.ref(); err != nil {
			return nil, err
		}
		if err = ps.expectKw("TO"); err != nil {
			return nil, err
		}
		if b, err = ps.ref(); err != nil {
			return nil, err
		}
		s.Refs = []Ref{a, b}
		if ps.acceptKw("LIMIT") {
			var n int
			n, err = ps.integer()
			ps.setOpt(s, "limit", strconv.Itoa(n))
		}
	case "EXPLAIN":
		if ps.acceptKw("ACCESS") {
			s.Sub = "ACCESS"
			var r Ref
			if r, err = ps.ref(); err != nil {
				return nil, err
			}
			s.Refs = []Ref{r}
			if err = ps.expectKw("FOR"); err != nil {
				return nil, err
			}
			s.Name, err = ps.word()
			break
		}
		s.Inner, err = ps.statement()
	case "REMEMBER":
		err = ps.rememberStmt(s)
	case "INSERT":
		err = ps.insertStmt(s)
	case "UPDATE":
		err = ps.updateStmt(s)
	case "CONCEPT":
		s.Write = true
		if s.Str, err = ps.word(); err != nil {
			return nil, err
		}
		for !ps.atEnd() && err == nil {
			switch {
			case ps.acceptKw("MEMBERS"):
				s.Refs, err = ps.refs()
			case ps.acceptKw("RELATED"):
				s.Strs, err = ps.strs()
			case ps.acceptKw("AS"):
				s.As, err = ps.word()
			default:
				err = ps.expectKw("MEMBERS", "RELATED", "AS")
			}
		}
	case "LINK", "CONNECT":
		s.Write = true
		var a, b Ref
		if a, err = ps.ref(); err != nil {
			return nil, err
		}
		if err = ps.expectKw("TO"); err != nil {
			return nil, err
		}
		if b, err = ps.ref(); err != nil {
			return nil, err
		}
		s.Refs = []Ref{a, b}
		for !ps.atEnd() && err == nil {
			switch {
			case ps.acceptKw("WEIGHT"):
				var w float64
				w, err = ps.number()
				ps.setOpt(s, "weight", strconv.FormatFloat(w, 'f', -1, 64))
			case ps.acceptKw("TYPE"):
				var ty string
				ty, err = ps.word()
				ps.setOpt(s, "type", strings.ToLower(ty))
			default:
				err = ps.expectKw("WEIGHT", "TYPE")
			}
		}
	case "UNLINK", "DISCONNECT":
		s.Write = true
		var a, b Ref
		if a, err = ps.ref(); err != nil {
			return nil, err
		}
		if err = ps.expectKw("FROM"); err != nil {
			return nil, err
		}
		b, err = ps.ref()
		s.Refs = []Ref{a, b}
	case "BOOST", "WEAKEN", "PIN", "UNPIN":
		s.Write = true
		s.Refs, err = ps.refs()
	case "SUPERSEDE":
		s.Write = true
		var a, b Ref
		if a, err = ps.ref(); err != nil {
			return nil, err
		}
		if err = ps.expectKw("WITH", "BY"); err != nil {
			return nil, err
		}
		b, err = ps.ref()
		s.Refs = []Ref{a, b}
	case "FORGET":
		s.Write = true
		if ps.acceptKw("WHERE") {
			s.Sub = "WHERE"
			if s.Where, err = ps.expr(); err != nil {
				return nil, err
			}
		} else {
			if s.Refs, err = ps.refs(); err != nil {
				return nil, err
			}
		}
		err = ps.dryConfirm(s)
		if s.Flags["dry"] {
			s.Write = false
		}
	case "FOCUS":
		s.Write = true
		switch {
		case ps.acceptKw("ADD"):
			s.Sub = "ADD"
			s.Refs, err = ps.refs()
		case ps.acceptKw("REMOVE"):
			s.Sub = "REMOVE"
			s.Refs, err = ps.refs()
		case ps.acceptKw("GOAL"):
			s.Sub = "GOAL"
		default:
			s.Refs, err = ps.refs()
		}
	case "GOAL":
		s.Write = true
		if s.Str, err = ps.str(); err != nil {
			return nil, err
		}
		if ps.acceptKw("FOCUS") {
			s.Refs, err = ps.refs()
		}
	case "FILL":
		err = ps.fillStmt(s)
	case "INGEST":
		s.Write = true
		if s.Str, err = ps.str(); err != nil {
			return nil, err
		}
		if ps.acceptKw("FOCUS") {
			ps.setFlag(s, "focus")
		}
	case "IMPORT":
		s.Write = true
		if s.Str, err = ps.str(); err != nil {
			return nil, err
		}
		for !ps.atEnd() && err == nil {
			switch {
			case ps.acceptKw("UPDATE"):
				ps.setFlag(s, "update")
			case ps.acceptKw("DRY"):
				err = ps.expectKw("RUN")
				ps.setFlag(s, "dry")
				s.Write = false
			default:
				err = ps.expectKw("UPDATE", "DRY")
			}
		}
	case "SOURCE":
		err = ps.sourceStmt(s)
	case "KIND":
		err = ps.kindStmt(s)
	case "BEGIN":
		ps.acceptKw("TRANSACTION")
	case "COMMIT", "ROLLBACK":
	case "LET":
		t := ps.peek()
		if t.kind != tVar {
			return nil, ps.errAt(t, "expected $name after LET")
		}
		ps.p++
		s.Name = t.text
		if !(ps.peek().kind == tOp && ps.peek().text == "=") {
			return nil, ps.errAt(ps.peek(), "expected = after LET $"+s.Name)
		}
		ps.p++
		if ps.isPunct("(") {
			ps.p++
			if err = ps.expectKw("SELECT"); err != nil {
				return nil, err
			}
			inner := &Stmt{Kind: "SELECT"}
			if inner.Sel, err = ps.selectStmt(); err != nil {
				return nil, err
			}
			s.Inner = inner
			err = ps.expectPunct(")")
		} else {
			var v Value
			v, err = ps.value()
			s.Val = &v
		}
	case "CONNECTOR":
		s.Write = true
		var ty string
		if ty, err = ps.word(); err != nil {
			return nil, err
		}
		s.Sub = strings.ToLower(ty)
		if s.Str, err = ps.word(); err != nil {
			return nil, err
		}
		for !ps.atEnd() && err == nil {
			switch {
			case ps.acceptKw("KEY"):
				var k string
				k, err = ps.word()
				ps.setOpt(s, "key", k)
			case ps.acceptKw("TERMS"):
				s.Strs, err = ps.strs()
			case ps.acceptKw("CONNECT"):
				s.Refs, err = ps.refs()
			case ps.acceptKw("GAIN"):
				var g float64
				g, err = ps.number()
				ps.setOpt(s, "gain", strconv.FormatFloat(g, 'f', -1, 64))
			case ps.acceptKw("TRANSPARENT"):
				ps.setOpt(s, "transparent", "true")
			case ps.acceptKw("OPAQUE"):
				ps.setOpt(s, "transparent", "false")
			case ps.acceptKw("AS"):
				s.As, err = ps.word()
			default:
				err = ps.expectKw("KEY", "TERMS", "CONNECT", "GAIN", "TRANSPARENT", "OPAQUE", "AS")
			}
		}
	case "RESOLVE":
		s.Write = true
		if s.Name, err = ps.word(); err != nil {
			return nil, err
		}
		for !ps.atEnd() && err == nil {
			switch {
			case ps.acceptKw("BY"):
				s.Words, err = ps.words()
			case ps.acceptKw("MIN"):
				var n int
				n, err = ps.integer()
				ps.setOpt(s, "min", strconv.Itoa(n))
			case ps.acceptKw("DRY"):
				err = ps.expectKw("RUN")
				ps.setFlag(s, "dry")
				s.Write = false
			default:
				err = ps.expectKw("BY", "MIN", "DRY")
			}
		}
	case "EXTRACT":
		if ps.acceptKw("FROM") {
			s.Str, err = ps.str()
		} else {
			var r Ref
			r, err = ps.ref()
			s.Refs = []Ref{r}
		}
	case "PATTERN":
		err = ps.patternStmt(s)
	default:
		msg := fmt.Sprintf("unknown statement %q", t.text)
		if c := brain.Closest(kw, stmtKeywords, 2); c != "" {
			msg += fmt.Sprintf(" (did you mean %s?)", c)
		}
		return nil, ps.errAt(t, msg)
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (ps *parser) contextStmt(s *Stmt) error {
	var err error
	for !ps.atEnd() && err == nil {
		switch {
		case ps.acceptKw("FOCUS"):
			s.Refs, err = ps.refs()
		case ps.acceptKw("FOR"):
			s.Str, err = ps.str()
		case ps.acceptKw("DEPTH"):
			var n int
			n, err = ps.integer()
			ps.setOpt(s, "depth", strconv.Itoa(n))
		case ps.acceptKw("THRESHOLD"):
			var x float64
			x, err = ps.number()
			ps.setOpt(s, "threshold", strconv.FormatFloat(x, 'f', -1, 64))
		case ps.acceptKw("HOP"):
			var x float64
			x, err = ps.number()
			ps.setOpt(s, "hop", strconv.FormatFloat(x, 'f', -1, 64))
		case ps.acceptKw("SOURCES"):
			s.Words, err = ps.words()
		case ps.acceptKw("FORMAT"):
			var f string
			f, err = ps.word()
			ps.setOpt(s, "format", strings.ToLower(f))
		case ps.acceptKw("SET"):
			ps.setFlag(s, "set")
			s.Write = true
		default:
			err = ps.expectKw("FOCUS", "FOR", "DEPTH", "THRESHOLD", "HOP", "SOURCES", "FORMAT", "SET")
		}
	}
	return err
}

var aggs = map[string]bool{"COUNT": true, "SUM": true, "AVG": true, "MIN": true, "MAX": true}

func (ps *parser) column() (brain.SelCol, error) {
	t := ps.peek()
	if t.kind == tPunct && t.text == "*" {
		ps.p++
		return brain.SelCol{Col: "*"}, nil
	}
	if t.kind == tIdent && aggs[strings.ToUpper(t.text)] && ps.peekN(1).kind == tPunct && ps.peekN(1).text == "(" {
		ps.p += 2
		c := brain.SelCol{Agg: strings.ToUpper(t.text)}
		if ps.isPunct("*") {
			ps.p++
			c.Col = "*"
		} else {
			w, err := ps.word()
			if err != nil {
				return c, err
			}
			c.Col = w
		}
		return c, ps.expectPunct(")")
	}
	if t.kind != tIdent {
		return brain.SelCol{}, ps.errAt(t, "expected a column, got "+describe(t))
	}
	ps.p++
	return brain.SelCol{Col: t.text}, nil
}

func (ps *parser) selectStmt() (*SelectStmt, error) {
	sel := &SelectStmt{}
	for {
		c, err := ps.column()
		if err != nil {
			return nil, err
		}
		sel.Columns = append(sel.Columns, c)
		if !ps.isPunct(",") {
			break
		}
		ps.p++
	}
	if err := ps.expectKw("FROM"); err != nil {
		return nil, err
	}
	from, err := ps.word()
	if err != nil {
		return nil, err
	}
	sel.From = strings.ToLower(from)
	for !ps.atEnd() && !ps.isPunct(")") {
		switch {
		case ps.acceptKw("WHERE"):
			if sel.Where, err = ps.expr(); err != nil {
				return nil, err
			}
		case ps.acceptKw("NEAR"):
			if sel.Near, err = ps.str(); err != nil {
				return nil, err
			}
			if ps.acceptKw("MODE") {
				m, err := ps.word()
				if err != nil {
					return nil, err
				}
				sel.Mode = strings.ToLower(m)
			}
		case ps.acceptKw("GROUP"):
			if err := ps.expectKw("BY"); err != nil {
				return nil, err
			}
			for {
				c, err := ps.column()
				if err != nil {
					return nil, err
				}
				sel.GroupBy = append(sel.GroupBy, c.Name())
				if !ps.isPunct(",") {
					break
				}
				ps.p++
			}
		case ps.acceptKw("ORDER"):
			if err := ps.expectKw("BY"); err != nil {
				return nil, err
			}
			for {
				c, err := ps.column()
				if err != nil {
					return nil, err
				}
				k := brain.OrderKey{Col: c.Name()}
				if ps.acceptKw("DESC") {
					k.Desc = true
				} else {
					ps.acceptKw("ASC")
				}
				sel.Order = append(sel.Order, k)
				if !ps.isPunct(",") {
					break
				}
				ps.p++
			}
		case ps.acceptKw("LIMIT"):
			if sel.Limit, err = ps.integer(); err != nil {
				return nil, err
			}
		case ps.acceptKw("OFFSET"):
			if sel.Offset, err = ps.integer(); err != nil {
				return nil, err
			}
		case ps.acceptKw("FORMAT"):
			f, err := ps.word()
			if err != nil {
				return nil, err
			}
			sel.Format = strings.ToLower(f)
		default:
			return nil, ps.expectKw("WHERE", "NEAR", "GROUP", "ORDER", "LIMIT", "OFFSET", "FORMAT")
		}
	}
	return sel, nil
}

// expr := or
func (ps *parser) expr() (Expr, error) {
	l, err := ps.andExpr()
	if err != nil {
		return nil, err
	}
	for ps.acceptKw("OR") {
		r, err := ps.andExpr()
		if err != nil {
			return nil, err
		}
		l = BinExpr{"OR", l, r}
	}
	return l, nil
}

func (ps *parser) andExpr() (Expr, error) {
	l, err := ps.notExpr()
	if err != nil {
		return nil, err
	}
	for ps.acceptKw("AND") {
		r, err := ps.notExpr()
		if err != nil {
			return nil, err
		}
		l = BinExpr{"AND", l, r}
	}
	return l, nil
}

func (ps *parser) notExpr() (Expr, error) {
	if ps.acceptKw("NOT") {
		x, err := ps.notExpr()
		if err != nil {
			return nil, err
		}
		return NotExpr{x}, nil
	}
	return ps.primary()
}

func (ps *parser) primary() (Expr, error) {
	if ps.isPunct("(") {
		ps.p++
		e, err := ps.expr()
		if err != nil {
			return nil, err
		}
		return e, ps.expectPunct(")")
	}
	if ps.acceptKw("HAS") {
		w, err := ps.word()
		return HasExpr{strings.ToLower(w)}, err
	}
	t := ps.peek()
	if t.kind != tIdent {
		return nil, ps.errAt(t, "expected a column or condition, got "+describe(t))
	}
	ps.p++
	col := t.text
	not := false
	if ps.isKw("NOT") && (strings.EqualFold(ps.peekN(1).text, "IN") || strings.EqualFold(ps.peekN(1).text, "BETWEEN")) {
		ps.p++
		not = true
	}
	switch {
	case ps.acceptKw("IN"):
		if err := ps.expectPunct("("); err != nil {
			return nil, err
		}
		vals := []Value{}
		for {
			v, err := ps.value()
			if err != nil {
				return nil, err
			}
			vals = append(vals, v)
			if !ps.isPunct(",") {
				break
			}
			ps.p++
		}
		return InExpr{col, vals, not}, ps.expectPunct(")")
	case ps.acceptKw("BETWEEN"):
		lo, err := ps.value()
		if err != nil {
			return nil, err
		}
		if err := ps.expectKw("AND"); err != nil {
			return nil, err
		}
		hi, err := ps.value()
		if err != nil {
			return nil, err
		}
		var e Expr = BetweenExpr{col, lo, hi}
		if not {
			e = NotExpr{e}
		}
		return e, nil
	}
	if op := ps.peek(); op.kind == tOp {
		ps.p++
		v, err := ps.value()
		if err != nil {
			return nil, err
		}
		o := op.text
		if o == "<>" {
			o = "!="
		}
		return CmpExpr{col, o, v}, nil
	}
	return BareExpr{col}, nil
}

func (ps *parser) rememberStmt(s *Stmt) error {
	s.Write = true
	var err error
	if s.Name, err = ps.word(); err != nil {
		return err
	}
	if s.Str, err = ps.str(); err != nil {
		return err
	}
	for !ps.atEnd() && err == nil {
		switch {
		case ps.acceptKw("IN"):
			var sec string
			sec, err = ps.word()
			ps.setOpt(s, "section", sec)
		case ps.acceptKw("CONCEPTS"):
			s.Strs, err = ps.strs()
		case ps.acceptKw("LINK"):
			if err = ps.expectKw("TO"); err != nil {
				return err
			}
			for {
				var r Ref
				if r, err = ps.ref(); err != nil {
					return err
				}
				l := LinkTo{Ref: r}
				if ps.acceptKw("WEIGHT") {
					var w float64
					if w, err = ps.number(); err != nil {
						return err
					}
					l.Weight = &w
				}
				s.Links = append(s.Links, l)
				if !ps.isPunct(",") {
					break
				}
				ps.p++
			}
		case ps.acceptKw("FIELDS"):
			s.Fields, err = ps.assigns(true)
		case ps.acceptKw("SUPERSEDES"):
			var r Ref
			r, err = ps.ref()
			s.Refs = []Ref{r}
		case ps.acceptKw("PIN"):
			ps.setFlag(s, "pin")
		case ps.acceptKw("FOCUS"):
			ps.setFlag(s, "focus")
		case ps.acceptKw("AS"):
			s.As, err = ps.word()
		case ps.acceptKw("LABELS"):
			s.Words, err = ps.parenList()
		case ps.acceptKw("OVERRIDE"):
			err = ps.overrideClause(s)
		default:
			err = ps.expectKw("IN", "CONCEPTS", "LINK", "FIELDS", "SUPERSEDES", "PIN", "FOCUS", "AS", "LABELS", "OVERRIDE")
		}
	}
	return err
}

func (ps *parser) assigns(paren bool) ([]Assign, error) {
	if paren {
		if err := ps.expectPunct("("); err != nil {
			return nil, err
		}
	}
	out := []Assign{}
	for {
		c, err := ps.word()
		if err != nil {
			return nil, err
		}
		if t := ps.peek(); !(t.kind == tOp && t.text == "=") {
			return nil, ps.errAt(t, "expected = after "+c)
		}
		ps.p++
		v, err := ps.value()
		if err != nil {
			return nil, err
		}
		out = append(out, Assign{c, v})
		if !ps.isPunct(",") {
			break
		}
		ps.p++
	}
	if paren {
		return out, ps.expectPunct(")")
	}
	return out, nil
}

func (ps *parser) insertStmt(s *Stmt) error {
	s.Write = true
	if err := ps.expectKw("INTO"); err != nil {
		return err
	}
	var err error
	if s.Name, err = ps.word(); err != nil {
		return err
	}
	if err := ps.expectPunct("("); err != nil {
		return err
	}
	if s.Cols, err = ps.strs(); err != nil {
		return err
	}
	if err := ps.expectPunct(")"); err != nil {
		return err
	}
	if err := ps.expectKw("VALUES"); err != nil {
		return err
	}
	for {
		start := ps.peek()
		if err := ps.expectPunct("("); err != nil {
			return err
		}
		row := []Value{}
		for {
			v, err := ps.value()
			if err != nil {
				return err
			}
			row = append(row, v)
			if !ps.isPunct(",") {
				break
			}
			ps.p++
		}
		if err := ps.expectPunct(")"); err != nil {
			return err
		}
		if len(row) != len(s.Cols) {
			return ps.errAt(start, fmt.Sprintf("%d values for %d columns", len(row), len(s.Cols)))
		}
		s.Rows = append(s.Rows, row)
		if !ps.isPunct(",") {
			for !ps.atEnd() {
				var err error
				switch {
				case ps.acceptKw("LABELS"):
					s.Words, err = ps.parenList()
				case ps.acceptKw("OVERRIDE"):
					err = ps.overrideClause(s)
				default:
					err = ps.expectKw("LABELS", "OVERRIDE")
				}
				if err != nil {
					return err
				}
			}
			return nil
		}
		ps.p++
	}
}

// parenList parses "(a, b, …)" or a bare comma list.
func (ps *parser) parenList() ([]string, error) {
	if ps.isPunct("(") {
		ps.p++
		if ps.isPunct(")") {
			ps.p++
			return []string{}, nil
		}
		l, err := ps.strs()
		if err != nil {
			return nil, err
		}
		return l, ps.expectPunct(")")
	}
	return ps.strs()
}

func (ps *parser) createStmt(s *Stmt) error {
	s.Write = true
	what, err := ps.word()
	if err != nil {
		return err
	}
	s.Sub = strings.ToUpper(what)
	if s.Sub != "USER" && s.Sub != "ROLE" {
		return ps.errAt(ps.toks[ps.p-1], "expected CREATE USER or CREATE ROLE")
	}
	if s.Name, err = ps.word(); err != nil {
		return err
	}
	for !ps.atEnd() && err == nil {
		switch {
		case ps.acceptKw("ROLES", "ROLE"):
			s.Strs, err = ps.parenList()
		case ps.acceptKw("LABELS"):
			s.Words, err = ps.parenList()
		case ps.acceptKw("DBS"):
			var l []string
			l, err = ps.parenList()
			ps.setOpt(s, "dbs", strings.Join(l, ","))
		case ps.acceptKw("TOKEN"):
			if err = ps.expectKw("ENV"); err != nil {
				return err
			}
			var v string
			v, err = ps.word()
			ps.setOpt(s, "tokenEnv", v)
		default:
			err = ps.expectKw("ROLES", "LABELS", "DBS", "TOKEN")
		}
	}
	return err
}

func (ps *parser) grantStmt(s *Stmt, kw string) error {
	s.Write = true
	acts, err := ps.strs()
	if err != nil {
		return err
	}
	for i := range acts {
		acts[i] = strings.ToLower(acts[i])
		if acts[i] != "read" && acts[i] != "write" && acts[i] != "admin" {
			return ps.errAt(ps.toks[ps.p-1], "actions are read, write or admin")
		}
	}
	s.Words = acts
	if err := ps.expectKw("ON"); err != nil {
		return err
	}
	if err := ps.expectKw("LABEL", "LABELS"); err != nil {
		return err
	}
	if ps.isPunct("*") {
		ps.p++
		s.Strs = []string{"*"}
	} else if s.Strs, err = ps.strs(); err != nil {
		return err
	}
	if kw == "REVOKE" {
		err = ps.expectKw("FROM")
	} else {
		err = ps.expectKw("TO")
	}
	if err != nil {
		return err
	}
	who, err := ps.word()
	if err != nil {
		return err
	}
	s.Sub = strings.ToUpper(who)
	if s.Sub != "ROLE" && s.Sub != "USER" {
		return ps.errAt(ps.toks[ps.p-1], "expected ROLE or USER")
	}
	if s.Name, err = ps.word(); err != nil {
		return err
	}
	for !ps.atEnd() && err == nil {
		if err = ps.expectKw("IN"); err != nil {
			return err
		}
		switch {
		case ps.acceptKw("SECTION"):
			var v []string
			v, err = ps.strs()
			ps.setOpt(s, "sections", strings.Join(v, ","))
		case ps.acceptKw("KIND"):
			var v []string
			v, err = ps.strs()
			ps.setOpt(s, "kinds", strings.Join(v, ","))
		case ps.acceptKw("DB"):
			var v []string
			v, err = ps.strs()
			ps.setOpt(s, "dbs", strings.Join(v, ","))
		default:
			err = ps.expectKw("SECTION", "KIND", "DB")
		}
	}
	return err
}

func (ps *parser) updateStmt(s *Stmt) error {
	s.Write = true
	tbl, err := ps.word()
	if err != nil {
		return err
	}
	s.Name = strings.ToLower(tbl)
	if err := ps.expectKw("SET"); err != nil {
		return err
	}
	if s.Fields, err = ps.assigns(false); err != nil {
		return err
	}
	if ps.acceptKw("WHERE") {
		if s.Where, err = ps.expr(); err != nil {
			return err
		}
	}
	if err := ps.dryConfirm(s); err != nil {
		return err
	}
	if s.Flags["dry"] {
		s.Write = false
	}
	return nil
}

func (ps *parser) fillStmt(s *Stmt) error {
	var err error
	if ps.acceptKw("TEMPLATE") {
		s.Sub = "TEMPLATE"
		if s.Str, err = ps.word(); err != nil {
			return err
		}
	} else if s.Str, err = ps.str(); err != nil {
		return err
	}
	for !ps.atEnd() && err == nil {
		switch {
		case ps.acceptKw("AS"):
			var f string
			f, err = ps.word()
			ps.setOpt(s, "as", strings.ToLower(f))
		case ps.acceptKw("OUTPUT"):
			var f string
			f, err = ps.word()
			ps.setOpt(s, "output", strings.ToLower(f))
		case ps.acceptKw("SCOPE"):
			var f string
			f, err = ps.word()
			ps.setOpt(s, "scope", strings.ToLower(f))
		case ps.acceptKw("WITH"):
			err = ps.expectKw("SOURCES")
			ps.setFlag(s, "sources")
		case ps.acceptKw("MIN"):
			var x float64
			x, err = ps.number()
			ps.setOpt(s, "min", strconv.FormatFloat(x, 'f', -1, 64))
		default:
			err = ps.expectKw("AS", "OUTPUT", "SCOPE", "WITH", "MIN")
		}
	}
	return err
}

func (ps *parser) sourceStmt(s *Stmt) error {
	var err error
	switch {
	case ps.acceptKw("ADD"):
		s.Sub, s.Write = "ADD", true
		if s.Name, err = ps.word(); err != nil {
			return err
		}
		if s.Str, err = ps.str(); err != nil {
			return err
		}
		for !ps.atEnd() && err == nil {
			switch {
			case ps.acceptKw("TOKEN"):
				if err = ps.expectKw("ENV"); err != nil {
					return err
				}
				var v string
				v, err = ps.word()
				ps.setOpt(s, "tokenEnv", v)
			case ps.acceptKw("WEIGHT"):
				var w float64
				w, err = ps.number()
				ps.setOpt(s, "weight", strconv.FormatFloat(w, 'f', -1, 64))
			case ps.acceptKw("DEPTH"):
				var n int
				n, err = ps.integer()
				ps.setOpt(s, "depth", strconv.Itoa(n))
			case ps.acceptKw("TTL"):
				var n int
				n, err = ps.integer()
				ps.setOpt(s, "ttl", strconv.Itoa(n))
			default:
				err = ps.expectKw("TOKEN", "WEIGHT", "DEPTH", "TTL")
			}
		}
	case ps.acceptKw("SYNC"):
		s.Sub = "SYNC"
		s.Name, err = ps.word()
	case ps.acceptKw("REMOVE", "RM"):
		s.Sub, s.Write = "REMOVE", true
		s.Name, err = ps.word()
	case ps.acceptKw("LS", "LIST"):
		s.Sub = "LS"
	default:
		err = ps.expectKw("ADD", "SYNC", "REMOVE", "LS")
	}
	return err
}

func (ps *parser) kindStmt(s *Stmt) error {
	if err := ps.expectKw("ADD"); err != nil {
		return err
	}
	s.Sub, s.Write = "ADD", true
	var err error
	if s.Name, err = ps.word(); err != nil {
		return err
	}
	for !ps.atEnd() && err == nil {
		switch {
		case ps.acceptKw("COLOR"):
			var c string
			c, err = ps.word()
			ps.setOpt(s, "color", c)
		case ps.acceptKw("HALF"):
			if err = ps.expectKw("LIFE"); err != nil {
				return err
			}
			var n float64
			n, err = ps.number()
			ps.setOpt(s, "halfLife", strconv.FormatFloat(n, 'f', -1, 64))
		case ps.acceptKw("BASE"):
			var n float64
			n, err = ps.number()
			ps.setOpt(s, "base", strconv.FormatFloat(n, 'f', -1, 64))
		case ps.acceptKw("PREFIX"):
			var p string
			p, err = ps.word()
			ps.setOpt(s, "prefix", p)
		case ps.acceptKw("SECTION"):
			var p string
			p, err = ps.word()
			ps.setOpt(s, "section", p)
		case ps.acceptKw("LABEL"):
			var p string
			p, err = ps.word()
			ps.setOpt(s, "label", p)
		default:
			err = ps.expectKw("COLOR", "HALF", "BASE", "PREFIX", "SECTION", "LABEL")
		}
	}
	return err
}

func (ps *parser) patternStmt(s *Stmt) error {
	var err error
	switch {
	case ps.acceptKw("ADD"):
		s.Sub, s.Write = "ADD", true
		if s.Name, err = ps.word(); err != nil {
			return err
		}
		if s.Str, err = ps.str(); err != nil {
			return err
		}
		for !ps.atEnd() && err == nil {
			switch {
			case ps.acceptKw("TYPE"):
				var t string
				t, err = ps.word()
				ps.setOpt(s, "type", t)
			case ps.acceptKw("NORMALIZE"):
				var t string
				t, err = ps.word()
				ps.setOpt(s, "normalize", strings.ToLower(t))
			case ps.acceptKw("PII"):
				var v Value
				v, err = ps.value()
				ps.setOpt(s, "pii", v.Text)
			default:
				err = ps.expectKw("TYPE", "NORMALIZE", "PII")
			}
		}
	case ps.acceptKw("TEST"):
		s.Sub = "TEST"
		if s.Name, err = ps.word(); err != nil {
			return err
		}
		s.Str, err = ps.str()
	case ps.acceptKw("REMOVE"):
		s.Sub, s.Write = "REMOVE", true
		s.Name, err = ps.word()
	default:
		err = ps.expectKw("ADD", "TEST", "REMOVE")
	}
	return err
}

func (ps *parser) overrideClause(s *Stmt) error {
	if err := ps.expectKw("PURPOSE"); err != nil {
		return err
	}
	if err := ps.expectKw("REASON"); err != nil {
		return err
	}
	r, err := ps.str()
	if err != nil {
		return err
	}
	if strings.TrimSpace(r) == "" {
		return ps.errAt(ps.toks[ps.p-1], "OVERRIDE PURPOSE needs a non-empty reason")
	}
	ps.setOpt(s, "override", r)
	return nil
}
