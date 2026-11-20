// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// FieldFilter is a field (or column) comparison.
type FieldFilter struct {
	Name  string `json:"name"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

// UnmarshalJSON accepts any scalar as value.
func (f *FieldFilter) UnmarshalJSON(d []byte) error {
	var raw struct {
		Name  string `json:"name"`
		Op    string `json:"op"`
		Value any    `json:"value"`
	}
	if err := json.Unmarshal(d, &raw); err != nil {
		return err
	}
	f.Name, f.Op, f.Value = raw.Name, raw.Op, scalarString(raw.Value)
	if f.Op == "" {
		f.Op = "="
	}
	return nil
}

// Where holds query filters.
type Where struct {
	ID         StrList       `json:"id,omitempty"`
	Kind       StrList       `json:"kind,omitempty"`
	Section    StrList       `json:"section,omitempty"`
	Concept    string        `json:"concept,omitempty"`
	Text       string        `json:"text,omitempty"`
	Lit        *bool         `json:"lit,omitempty"`
	Pinned     *bool         `json:"pinned,omitempty"`
	Superseded *bool         `json:"superseded,omitempty"`
	MinWeight  *float64      `json:"minWeight,omitempty"`
	MaxAgeDays *float64      `json:"maxAgeDays,omitempty"`
	Fields     []FieldFilter `json:"fields,omitempty"`
}

// Query is a raw data query.
type Query struct {
	From    string  `json:"from,omitempty"`
	Where   Where   `json:"where"`
	Near    string  `json:"near,omitempty"`
	OrderBy string  `json:"orderBy,omitempty"`
	Limit   int     `json:"limit,omitempty"`
	Offset  int     `json:"offset,omitempty"`
	Select  StrList `json:"select,omitempty"`
	Sources StrList `json:"sources,omitempty"`
	Format  string  `json:"format,omitempty"`
}

// QueryResult is a raw query result.
type QueryResult struct {
	Columns  []string `json:"columns"`
	Rows     []*OMap  `json:"rows"`
	Count    int      `json:"count"`
	Total    int      `json:"total"`
	Rendered string   `json:"rendered"`
	Format   string   `json:"format"`
}

var cmpOps = []string{">=", "<=", "!=", "=", ">", "<", "~", ":"}

var compactKeys = map[string]bool{"kind": true, "section": true, "concept": true, "lit": true, "pinned": true, "superseded": true,
	"near": true, "sort": true, "orderby": true, "order": true, "limit": true, "offset": true, "select": true, "from": true, "text": true,
	"id": true, "source": true, "format": true}

// tokenize splits on whitespace outside double/single quotes and strips the quotes.
func tokenize(s string) ([]string, error) {
	out := []string{}
	var cur strings.Builder
	has := false
	var q rune
	for _, r := range s {
		switch {
		case q != 0:
			if r == q {
				q = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			q, has = r, true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if has {
				out = append(out, cur.String())
				cur.Reset()
				has = false
			}
		default:
			cur.WriteRune(r)
			has = true
		}
	}
	if q != 0 {
		return nil, errf("unterminated quote in query")
	}
	if has {
		out = append(out, cur.String())
	}
	return out, nil
}

var termRe = regexp.MustCompile(`^([A-Za-z_@][\w.@-]*)(>=|<=|!=|=|>|<|~|:)(.*)$`)

func parseBool(s string) (bool, error) {
	switch lower(s) {
	case "true", "yes", "1", "y":
		return true, nil
	case "false", "no", "0", "n":
		return false, nil
	}
	return false, errf("expected true or false, got %q", s)
}

// ParseQuery parses a JSON object or the compact string syntax.
func ParseQuery(src string) (*Query, error) {
	src = strings.TrimSpace(src)
	q := &Query{}
	if strings.HasPrefix(src, "{") {
		if err := json.Unmarshal([]byte(src), q); err != nil {
			return nil, errf("query JSON: %v", err)
		}
		return q, nil
	}
	// note: a quoted string keeps its spaces; detect quoted bare terms by re-tokenising with markers
	toks, err := tokenizeKeepQuoted(src)
	if err != nil {
		return nil, err
	}
	text := []string{}
	for _, t := range toks {
		if t.quoted && !strings.Contains(t.prefix, ":") {
			text = append(text, t.s)
			continue
		}
		m := termRe.FindStringSubmatch(t.s)
		if m == nil {
			text = append(text, t.s)
			continue
		}
		k, op, v := m[1], m[2], m[3]
		lk := lower(k)
		if op == ":" && compactKeys[lk] {
			switch lk {
			case "kind":
				q.Where.Kind = append(q.Where.Kind, splitIDs(v)...)
			case "section":
				q.Where.Section = append(q.Where.Section, splitIDs(v)...)
			case "id":
				q.Where.ID = append(q.Where.ID, splitIDs(v)...)
			case "concept":
				q.Where.Concept = v
			case "text":
				text = append(text, v)
			case "lit", "pinned", "superseded":
				bv, err := parseBool(v)
				if err != nil {
					return nil, errf("%s: %v", k, err)
				}
				switch lk {
				case "lit":
					q.Where.Lit = &bv
				case "pinned":
					q.Where.Pinned = &bv
				default:
					q.Where.Superseded = &bv
				}
			case "near":
				q.Near = v
			case "sort", "orderby", "order":
				q.OrderBy = v
			case "limit", "offset":
				n, err := strconv.Atoi(v)
				if err != nil || n < 0 {
					return nil, errf("%s must be a non-negative integer", k)
				}
				if lk == "limit" {
					q.Limit = n
				} else {
					q.Offset = n
				}
			case "select":
				q.Select = splitIDs(v)
			case "from":
				q.From = v
			case "source":
				q.Sources = splitIDs(v)
			case "format":
				q.Format = v
			}
			continue
		}
		if op == ":" {
			op = "="
		}
		switch {
		case (lk == "w" || lk == "weight") && (op == ">=" || op == ">"):
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, errf("%s: expected a number", k)
			}
			q.Where.MinWeight = &f
			continue
		case lk == "age" && (op == "<=" || op == "<"):
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, errf("%s: expected a number", k)
			}
			q.Where.MaxAgeDays = &f
			continue
		}
		q.Where.Fields = append(q.Where.Fields, FieldFilter{Name: k, Op: op, Value: v})
	}
	q.Where.Text = strings.Join(text, " ")
	return q, nil
}

type qtok struct {
	s, prefix string
	quoted    bool
}

func tokenizeKeepQuoted(s string) ([]qtok, error) {
	out := []qtok{}
	var cur strings.Builder
	has, quoted := false, false
	prefix := ""
	var q rune
	for _, r := range s {
		switch {
		case q != 0:
			if r == q {
				q = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			if !quoted {
				prefix = cur.String()
			}
			q, has, quoted = r, true, true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if has {
				out = append(out, qtok{cur.String(), prefix, quoted})
				cur.Reset()
				has, quoted, prefix = false, false, ""
			}
		default:
			cur.WriteRune(r)
			has = true
		}
	}
	if q != 0 {
		return nil, errf("unterminated quote in query")
	}
	if has {
		out = append(out, qtok{cur.String(), prefix, quoted})
	}
	return out, nil
}

var numStrip = regexp.MustCompile(`[\s$€£¥₹%,]|zł|PLN|USD|EUR|GBP|CHF`)

// parseNum parses numbers with currency symbols, thousands separators and %.
func parseNum(s string) (float64, bool) {
	t := strings.TrimSpace(s)
	if t == "" {
		return 0, false
	}
	mult := 1.0
	if l := lower(t); strings.HasSuffix(l, "k") && len(l) > 1 && l[len(l)-2] >= '0' && l[len(l)-2] <= '9' {
		mult, t = 1e3, t[:len(t)-1]
	} else if strings.HasSuffix(l, "m") && len(l) > 1 && l[len(l)-2] >= '0' && l[len(l)-2] <= '9' {
		mult, t = 1e6, t[:len(t)-1]
	}
	t = numStrip.ReplaceAllString(t, "")
	v, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return 0, false
	}
	return v * mult, true
}

func compare(a, op, b string) bool {
	if op == "~" {
		return strings.Contains(lowerDeacc(a), lowerDeacc(b))
	}
	x, ok1 := parseNum(a)
	y, ok2 := parseNum(b)
	c := 0
	if ok1 && ok2 {
		switch {
		case x < y:
			c = -1
		case x > y:
			c = 1
		}
	} else {
		if op == "=" || op == "!=" {
			eq := strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
			return eq == (op == "=")
		}
		c = strings.Compare(lower(a), lower(b))
	}
	switch op {
	case "=":
		return c == 0
	case "!=":
		return c != 0
	case ">":
		return c > 0
	case ">=":
		return c >= 0
	case "<":
		return c < 0
	case "<=":
		return c <= 0
	}
	return false
}

// column returns a selectable value of a node.
func (b *Brain) column(n *Node, col string, L lit, score map[string]float64) (any, bool) {
	if strings.HasPrefix(col, "fields.") {
		v, ok := n.Fields.Get(col[7:])
		if !ok {
			return nil, true
		}
		return v, true
	}
	switch col {
	case "id":
		return n.ID, true
	case "kind":
		return n.Kind, true
	case "section":
		return n.Section, true
	case "text", "label":
		return n.Text, true
	case "weight", "w":
		return roundN(b.nodeW(n), 4), true
	case "base":
		return n.Base, true
	case "age":
		return n.Age, true
	case "touched":
		// Absolute last-touch timestamp, same value and ISO-8601 form the XML carries, so
		// "WHERE touched < '2026-11-01'" compares lexicographically and correctly.
		return isoMs(b.touchOf(n)), true
	case "pinned":
		return n.Pin, true
	case "superseded":
		return n.Sup != "", true
	case "supersededBy":
		return n.Sup, true
	case "parent":
		return n.Parent, true
	case "activation", "act":
		if a, ok := L.r.act[n.ID]; ok {
			return roundN(a, 4), true
		}
		return nil, true
	case "depth":
		if d, ok := L.r.depth[n.ID]; ok {
			return d, true
		}
		return nil, true
	case "via":
		return L.r.parent[n.ID], true
	case "lit":
		return L.r.has(n.ID), true
	case "score":
		if score != nil {
			return roundN(score[n.ID], 4), true
		}
		return roundN(b.nodeW(n), 4), true
	case "source":
		if n.Ext != nil {
			return n.Ext.Source, true
		}
		return "", true
	case "file":
		if n.Meta != nil {
			return n.Meta.Name, true
		}
		return "", true
	case "concepts":
		cs := []string{}
		for _, e := range b.adj[n.ID] {
			if e.Type == EdgeConcept {
				cs = append(cs, b.nodes[other(e, n.ID)].Text)
			}
		}
		return strings.Join(cs, ", "), true
	case "members", "degree":
		return len(b.adj[n.ID]), true
	case "connector":
		return n.Conn != nil, true
	case "labels":
		return strings.Join(n.Labels, ","), true
	case "src":
		return n.Src, true
	case "purpose.score":
		if n.Purpose != nil {
			return n.Purpose.Score, true
		}
		return nil, true
	case "purpose.decision":
		if n.Purpose != nil {
			return n.Purpose.Decision, true
		}
		return "", true
	case "purpose.flag":
		if n.Purpose != nil {
			return n.Purpose.Flag, true
		}
		return "", true
	case "purpose.reasons":
		if n.Purpose != nil {
			return n.Purpose.Reasons, true
		}
		return "", true
	case "purpose.section":
		if n.Purpose != nil {
			return n.Purpose.Section, true
		}
		return "", true
	case "route":
		return strings.Join(L.r.route[n.ID], ","), true
	case "type":
		if n.Conn != nil {
			return n.Conn.Type, true
		}
		return n.Kind, true
	case "key":
		if n.Conn != nil {
			return n.Conn.Key, true
		}
		return "", true
	case "gain":
		if n.Conn != nil {
			return n.Conn.Gain, true
		}
		return nil, true
	case "transparent":
		return n.Conn != nil && n.Conn.Transparent, true
	case "terms":
		if n.Conn != nil {
			return strings.Join(n.Conn.Terms, ", "), true
		}
		return "", true
	}
	if v, ok := n.Fields.Get(col); ok {
		return v, true
	}
	return nil, false
}

func asString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return fmtNum(x)
	case int:
		return strconv.Itoa(x)
	case bool:
		return strconv.FormatBool(x)
	}
	return fmt.Sprint(v)
}

// RunQuery executes a raw data query.
func (b *Brain) RunQuery(q *Query, format string) (*QueryResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.runQuery(q, format)
}

func (b *Brain) runQuery(q *Query, format string) (*QueryResult, error) {
	if format == "" {
		format = q.Format
	}
	if format == "" {
		format = "json"
	}
	from := q.From
	if i := strings.Index(from, "@"); i >= 0 {
		q.Sources = append(q.Sources, from[i+1:])
		from = from[:i]
	}
	if from == "" {
		from = "memory"
	}
	L := b.curLit()
	var score map[string]float64
	if q.Near != "" {
		score = b.hybrid(q.Near).scores
	}
	var cols []string
	rows := []*OMap{}
	// windowed is the match count before offset/limit were applied to the ids. Building an OMap per
	// match and cutting afterwards costs the same whether the caller wants 50 rows or 50000; with no
	// ordering the window can be taken first, which is where almost all of a query's time went.
	windowed := -1
	switch from {
	case "links", "edges", "synapses":
		cols = []string{"a", "b", "w", "type"}
		if len(q.Select) > 0 {
			cols = q.Select
		}
		for _, e := range b.edges {
			vals := map[string]string{"a": e.A, "b": e.B, "w": fmtNum(e.W), "type": e.Type}
			ok := true
			for _, f := range q.Where.Fields {
				if v, has := vals[f.Name]; !has || !compare(v, f.Op, f.Value) {
					ok = false
				}
			}
			if len(q.Where.Kind) > 0 && !contains(q.Where.Kind, e.Type) {
				ok = false
			}
			if len(q.Where.ID) > 0 && !contains(q.Where.ID, e.A) && !contains(q.Where.ID, e.B) {
				ok = false
			}
			if q.Where.MinWeight != nil && e.W < *q.Where.MinWeight {
				ok = false
			}
			if !ok {
				continue
			}
			r := NewOMap()
			for _, c := range cols {
				switch c {
				case "w":
					r.Set(c, e.W)
				case "a", "b", "type":
					r.Set(c, vals[c])
				default:
					r.Set(c, nil)
				}
			}
			rows = append(rows, r)
		}
	case "memory", "concepts", "nodes", "all", "connectors":
		ids := []string{}
		// F4: when the SQLite backend is attached and current, let it narrow the candidates. The
		// predicate below still decides, so this can only change how many nodes it has to look at.
		scan := b.order
		// Ask SQL for only as many candidates as the page needs when nothing has to be ranked first.
		// That keeps an unselective predicate from materialising a third of the brain through the
		// driver, which measured slower than scanning it in memory.
		cand, pushed := b.sqlCandidates(&q.Where, from)
		if pushed {
			scan = cand
		}
		for _, id := range scan {
			n := b.nodes[id]
			if n == nil {
				continue
			}
			isC := n.Kind == Concept
			if (from == "connectors") != (n.Conn != nil) {
				continue
			}
			if (from == "memory" && isC) || (from == "concepts" && !isC) {
				continue
			}
			if n.Ext != nil && len(q.Sources) == 0 {
				continue
			}
			if !b.matchWhere(n, q, L) {
				continue
			}
			ids = append(ids, id)
		}
		if len(q.Select) > 0 {
			cols = q.Select
		} else if from == "concepts" {
			cols = []string{"id", "text", "weight", "activation", "degree"}
		} else if from == "connectors" {
			cols = []string{"id", "type", "label", "key", "gain", "transparent", "degree"}
		} else {
			cols = []string{"id", "kind", "section", "text", "weight", "activation"}
			seen := map[string]bool{}
			for _, id := range ids {
				for _, f := range b.nodes[id].Fields {
					if !seen[f.Name] {
						seen[f.Name] = true
						cols = append(cols, "fields."+f.Name)
					}
				}
			}
		}
		if q.Near != "" && !contains(cols, "score") {
			cols = append(cols, "score")
		}
		for _, c := range cols {
			if _, ok := b.column(&Node{}, c, L, score); !ok && !strings.HasPrefix(c, "fields.") {
				// unknown column: allowed if some row has the field
				found := false
				for _, id := range ids {
					if _, ok := b.nodes[id].Fields.Get(c); ok {
						found = true
						break
					}
				}
				if !found {
					return nil, errf("unknown column %q (try id, kind, section, text, weight, activation, score, age, pinned, fields.<name>)", c)
				}
			}
		}
		ob := q.OrderBy
		if ob == "" && q.Near != "" {
			ob = "-score"
		}
		if ob == "" {
			// No ordering: the rows that survive are the first ones, so materialise only those.
			windowed = len(ids)
			lim := q.Limit
			if lim <= 0 {
				lim = 100
			}
			if q.Offset > 0 {
				if q.Offset >= len(ids) {
					ids = nil
				} else {
					ids = ids[q.Offset:]
				}
			}
			if len(ids) > lim {
				ids = ids[:lim]
			}
		}
		for _, id := range ids {
			n := b.nodes[id]
			r := NewOMap()
			for _, c := range cols {
				v, _ := b.column(n, c, L, score)
				r.Set(c, v)
			}
			r.Set("\x00id", id)
			rows = append(rows, r)
		}
		if ob != "" {
			desc := strings.HasPrefix(ob, "-")
			key := strings.TrimPrefix(strings.TrimPrefix(ob, "-"), "+")
			kv := func(r *OMap) any {
				if v, ok := r.Vals[key]; ok {
					return v
				}
				id := r.Vals["\x00id"].(string)
				v, _ := b.column(b.nodes[id], key, L, score)
				if v == nil {
					if fv, ok := b.nodes[id].Fields.Get(key); ok {
						return fv
					}
				}
				return v
			}
			sort.SliceStable(rows, func(i, j int) bool {
				c := cmpAny(kv(rows[i]), kv(rows[j]))
				if desc {
					return c > 0
				}
				return c < 0
			})
		}
		for _, r := range rows {
			r.Delete("\x00id")
		}
	default:
		return nil, errf("from must be memory, concepts, connectors or links")
	}
	total := len(rows)
	if windowed >= 0 {
		total = windowed // the cut already happened, on the ids
	} else {
		if q.Offset > 0 {
			if q.Offset >= len(rows) {
				rows = rows[:0]
			} else {
				rows = rows[q.Offset:]
			}
		}
		limit := q.Limit
		if limit <= 0 {
			limit = 100
		}
		if len(rows) > limit {
			rows = rows[:limit]
		}
	}
	res := &QueryResult{Columns: cols, Rows: rows, Count: len(rows), Total: total, Format: format}
	out, err := RenderRows(cols, rows, format)
	if err != nil {
		return nil, err
	}
	res.Rendered = b.maskText(out)
	return res, nil
}

func cmpAny(a, c any) int {
	if a == nil && c == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if c == nil {
		return 1
	}
	x, ok1 := parseNum(asString(a))
	y, ok2 := parseNum(asString(c))
	if ok1 && ok2 {
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
		return 0
	}
	if ba, ok := a.(bool); ok {
		bc, _ := c.(bool)
		switch {
		case ba == bc:
			return 0
		case !ba:
			return -1
		}
		return 1
	}
	return strings.Compare(lower(asString(a)), lower(asString(c)))
}

func (b *Brain) hasConcept(id, concept string) bool {
	A := b.findConcept(concept, false)
	if A == "" {
		return false
	}
	return b.edgeBetween(id, A) != nil
}

func (b *Brain) matchWhere(n *Node, q *Query, L lit) bool {
	w := q.Where
	if len(w.ID) > 0 && !contains(w.ID, n.ID) {
		return false
	}
	if len(w.Kind) > 0 && !contains(w.Kind, n.Kind) {
		return false
	}
	if len(w.Section) > 0 && !contains(w.Section, n.Section) {
		return false
	}
	if w.Concept != "" && !b.hasConcept(n.ID, w.Concept) {
		return false
	}
	if w.Text != "" {
		t := lowerDeacc(n.Text + " " + fieldsText(n.Fields))
		for _, word := range strings.Fields(lowerDeacc(w.Text)) {
			if !strings.Contains(t, word) {
				return false
			}
		}
	}
	if w.Lit != nil && L.r.has(n.ID) != *w.Lit {
		return false
	}
	if w.Pinned != nil && n.Pin != *w.Pinned {
		return false
	}
	if w.Superseded != nil && (n.Sup != "") != *w.Superseded {
		return false
	}
	if w.MinWeight != nil && b.nodeW(n) < *w.MinWeight {
		return false
	}
	if w.MaxAgeDays != nil && n.Age > *w.MaxAgeDays {
		return false
	}
	if len(q.Sources) > 0 && n.Ext != nil && !contains(q.Sources, "*") && !contains(q.Sources, n.Ext.Source) {
		return false
	}
	for _, f := range w.Fields {
		v, ok := n.Fields.Get(f.Name)
		if !ok {
			cv, known := b.column(n, f.Name, L, nil)
			if !known || cv == nil {
				// No value: it satisfies no comparison, "!=" included, the same rule SELECT follows.
				return false
			}
			v = asString(cv)
		}
		if f.Name == "route" && (f.Op == "=" || f.Op == "!=") {
			if contains(L.r.route[n.ID], f.Value) != (f.Op == "=") {
				return false
			}
			continue
		}
		if !compare(v, f.Op, f.Value) {
			return false
		}
	}
	return true
}

func fieldsText(f Fields) string {
	parts := []string{}
	for _, x := range f {
		parts = append(parts, x.Value)
	}
	return strings.Join(parts, " ")
}

// ---------------------------------------------------------------- renderers

// RenderRows renders rows as json | yaml | xml | md | csv.
func RenderRows(cols []string, rows []*OMap, format string) (string, error) {
	switch format {
	case "json":
		l := make([]any, len(rows))
		for i, r := range rows {
			l[i] = r
		}
		return jsonIndent(l), nil
	case "yaml":
		l := make([]any, len(rows))
		for i, r := range rows {
			l[i] = r
		}
		return EmitYAML(l), nil
	case "xml":
		var x strings.Builder
		fmt.Fprintf(&x, "<rows count=\"%d\">\n", len(rows))
		for _, r := range rows {
			x.WriteString("  <row")
			for _, c := range cols {
				if v := r.Vals[c]; v != nil {
					x.WriteString(" " + xmlName(c) + `="` + esc(asString(v)) + `"`)
				}
			}
			x.WriteString("/>\n")
		}
		x.WriteString("</rows>\n")
		return x.String(), nil
	case "md", "markdown":
		var x strings.Builder
		x.WriteString("| " + strings.Join(cols, " | ") + " |\n|")
		for range cols {
			x.WriteString("---|")
		}
		x.WriteString("\n")
		for _, r := range rows {
			cells := []string{}
			for _, c := range cols {
				s := strings.ReplaceAll(flat(asString(r.Vals[c])), "|", "\\|")
				cells = append(cells, s)
			}
			x.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		}
		return x.String(), nil
	case "csv":
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		w.UseCRLF = true
		_ = w.Write(cols)
		for _, r := range rows {
			rec := []string{}
			for _, c := range cols {
				rec = append(rec, asString(r.Vals[c]))
			}
			_ = w.Write(rec)
		}
		w.Flush()
		return buf.String(), nil
	}
	return "", errf("format must be json, yaml, xml, md or csv")
}

var xmlNameBad = regexp.MustCompile(`[^\w.-]`)

func xmlName(s string) string {
	s = xmlNameBad.ReplaceAllString(s, "_")
	if s == "" || (s[0] >= '0' && s[0] <= '9') || s[0] == '-' || s[0] == '.' {
		s = "_" + s
	}
	return s
}

// ---------------------------------------------------------------- context query

// ContextQuery is an ad-hoc lit slice request (does not change the shared focus).
type ContextQuery struct {
	Focus          StrList  `json:"focus,omitempty"`
	Query          string   `json:"query,omitempty"`
	Depth          *int     `json:"depth,omitempty"`
	Threshold      *float64 `json:"threshold,omitempty"`
	Hop            *float64 `json:"hop,omitempty"`
	ShowSuperseded *bool    `json:"showSuperseded,omitempty"`
	Format         string   `json:"format,omitempty"`
	Sources        StrList  `json:"sources,omitempty"`
}

// ContextQueryResult is the result of a context query.
type ContextQueryResult struct {
	Format string   `json:"format"`
	Output string   `json:"output"`
	Focus  []string `json:"focus"`
	Lit    int      `json:"lit"`
	Tokens int      `json:"tokens"`
}

func (b *Brain) adhocLit(c ContextQuery) (lit, error) {
	P := b.P
	if c.Depth != nil {
		P.Depth = *c.Depth
	}
	if c.Threshold != nil {
		P.Threshold = *c.Threshold
	}
	if c.Hop != nil {
		P.Hop = *c.Hop
	}
	if c.ShowSuperseded != nil {
		P.ShowSuperseded = *c.ShowSuperseded
	}
	seeds := []string(c.Focus)
	if len(seeds) == 1 && seeds[0] == "goal" {
		seeds = append([]string(nil), b.goal...)
	}
	for _, id := range seeds {
		if b.nodes[id] == nil {
			return lit{}, errf("no such item %s", id)
		}
	}
	if len(seeds) == 0 && c.Query != "" {
		seeds = b.hybrid(c.Query).seeds
	}
	if len(seeds) == 0 && c.Query == "" {
		seeds = b.seeds
	}
	return lit{seeds: seeds, P: P, r: b.activate(seeds, P, nil)}, nil
}

// QueryContext renders an ad-hoc lit slice in xml | md | json | yaml.
func (b *Brain) QueryContext(c ContextQuery) (*ContextQueryResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	L, err := b.adhocLit(c)
	if err != nil {
		return nil, err
	}
	f := c.Format
	if f == "" {
		f = "xml"
	}
	st := b.contextStats(L.r)
	res := &ContextQueryResult{Format: f, Focus: L.seeds, Lit: len(L.r.order), Tokens: st.Tok}
	switch f {
	case "xml":
		res.Output = b.contextXML(L)
	case "md", "markdown":
		res.Output = b.contextMD(L)
	case "json":
		res.Output = jsonIndent(b.contextDoc(L))
	case "yaml":
		res.Output = EmitYAML(toOrdered(b.contextDoc(L)))
	default:
		return nil, errf("format must be xml, md, json or yaml")
	}
	res.Output = b.maskText(res.Output)
	return res, nil
}

type ctxDocItem struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Text         string   `json:"text"`
	W            float64  `json:"w"`
	Act          float64  `json:"act"`
	Depth        int      `json:"depth"`
	Via          string   `json:"via,omitempty"`
	Age          float64  `json:"age"`
	Pinned       bool     `json:"pinned,omitempty"`
	SupersededBy string   `json:"supersededBy,omitempty"`
	Refs         []string `json:"refs,omitempty"`
	Concepts     []string `json:"concepts,omitempty"`
	Fields       Fields   `json:"fields,omitempty"`
	Source       string   `json:"source,omitempty"`
}

type ctxDoc struct {
	Now       string   `json:"now"`
	Step      int      `json:"step"`
	Tokens    int      `json:"tokens"`
	Depth     int      `json:"depth"`
	Threshold float64  `json:"threshold"`
	Hop       float64  `json:"hop"`
	Goal      string   `json:"goal"`
	Focus     []string `json:"focus"`
	Concepts  []struct {
		ID    string  `json:"id"`
		Text  string  `json:"text"`
		Act   float64 `json:"act"`
		Depth int     `json:"depth"`
	} `json:"concepts"`
	Sections []struct {
		Name  string       `json:"name"`
		Items []ctxDocItem `json:"items"`
	} `json:"sections"`
	Memory struct {
		Total           int `json:"total"`
		Lit             int `json:"lit"`
		SupersededFaded int `json:"supersededFaded"`
	} `json:"memory"`
}

func (b *Brain) contextDoc(L lit) *ctxDoc {
	st := b.contextStats(L.r)
	d := &ctxDoc{Now: b.stamp(), Step: b.Step, Tokens: st.Tok, Depth: L.P.Depth, Threshold: L.P.Threshold, Hop: L.P.Hop, Goal: b.Goal, Focus: append([]string{}, L.seeds...)}
	for _, id := range b.litConcepts(L) {
		d.Concepts = append(d.Concepts, struct {
			ID    string  `json:"id"`
			Text  string  `json:"text"`
			Act   float64 `json:"act"`
			Depth int     `json:"depth"`
		}{id, b.nodes[id].Text, roundN(L.r.act[id], 4), L.r.depth[id]})
	}
	for _, sec := range b.sectionOrder() {
		items := b.litItems(L, sec, nil)
		if len(items) == 0 {
			continue
		}
		s := struct {
			Name  string       `json:"name"`
			Items []ctxDocItem `json:"items"`
		}{Name: sec}
		for _, n := range items {
			it := ctxDocItem{ID: n.ID, Kind: n.Kind, Text: n.Text, W: roundN(b.nodeW(n), 4), Act: roundN(L.r.act[n.ID], 4), Depth: L.r.depth[n.ID],
				Via: L.r.parent[n.ID], Age: n.Age, Pinned: n.Pin, SupersededBy: n.Sup, Refs: b.refsOf(n.ID, L), Concepts: b.conceptTags(n.ID), Fields: n.Fields}
			if n.Ext != nil {
				it.Source = n.Ext.Source
			}
			s.Items = append(s.Items, it)
		}
		d.Sections = append(d.Sections, s)
	}
	d.Memory.Total, d.Memory.Lit = len(b.order), len(L.r.order)
	for _, id := range b.order {
		if b.nodes[id].Sup != "" && !L.r.has(id) {
			d.Memory.SupersededFaded++
		}
	}
	return d
}

var _ = math.Abs
