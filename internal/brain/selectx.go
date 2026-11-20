// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/Gabski/brainDB/internal/patterns"
)

// EntVals is the value of an "entities.<type>" column: all normalised values of that type in a node.
type EntVals struct {
	Type string
	Vals []string
}

// SelCol is one selected column: a column name, "*", "fields.*", or an aggregate.
type SelCol struct {
	Col string // column name or "*" (COUNT(*))
	Agg string // COUNT | SUM | AVG | MIN | MAX | ""
}

// Name is the result column name.
func (c SelCol) Name() string {
	if c.Agg != "" {
		return c.Agg + "(" + c.Col + ")"
	}
	return c.Col
}

// OrderKey is an ORDER BY key.
type OrderKey struct {
	Col  string
	Desc bool
}

// SelectSpec is a SELECT statement after parsing.
type SelectSpec struct {
	From    string
	Sources []string
	Where   func(get func(col string) any) (bool, error)
	Near    string
	Mode    string
	Columns []SelCol
	GroupBy []string
	Order   []OrderKey
	Limit   int
	Offset  int
	MaxRows int
	IDsOnly bool
}

// SelectResult is the result of Select.
type SelectResult struct {
	Columns []string
	Rows    []*OMap
	IDs     []string
	Total   int
}

// entities returns the (cached) entities of a node.
func (b *Brain) entities(id string) []patterns.Entity {
	n := b.nodes[id]
	if n == nil || n.Conn != nil {
		return nil
	}
	key := n.Text + "\x00" + fieldsText(n.Fields) + "\x00" + func() string {
		if n.Meta != nil {
			return n.Meta.Name
		}
		return ""
	}()
	if b.entCache == nil {
		b.entCache = map[string]entCacheItem{}
	}
	if c, ok := b.entCache[id]; ok && c.key == key {
		return c.es
	}
	txt := n.Text
	for _, f := range n.Fields {
		txt += "\n" + f.Value
	}
	es := b.patOpts().Extract(txt)
	b.entCache[id] = entCacheItem{key, es}
	return es
}

type entCacheItem struct {
	key string
	es  []patterns.Entity
}

// Entities returns the entities of one node.
func (b *Brain) Entities(id string) ([]patterns.Entity, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.nodes[id] == nil {
		return nil, errf("no such item %s", id)
	}
	es := append([]patterns.Entity(nil), b.entities(id)...)
	for i := range es {
		if es[i].Type == "card" || contains(b.Privacy, es[i].Type) {
			es[i].Raw = patterns.Mask(es[i].Type, es[i].Raw)
			es[i].Value = patterns.Mask(es[i].Type, es[i].Value)
		}
	}
	return es, nil
}

func (b *Brain) defaultCols(from string, ids []string) []string {
	switch from {
	case "concepts":
		return []string{"id", "text", "weight", "activation", "degree"}
	case "connectors":
		return []string{"id", "type", "label", "key", "gain", "transparent", "degree"}
	case "links":
		return []string{"a", "b", "w", "type"}
	case "entities":
		return []string{"id", "node", "type", "value", "raw", "confidence"}
	}
	cols := []string{"id", "kind", "section", "text", "weight", "activation"}
	return append(cols, b.fieldCols(ids)...)
}

func (b *Brain) fieldCols(ids []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		if n := b.nodes[id]; n != nil {
			for _, f := range n.Fields {
				if !seen[f.Name] {
					seen[f.Name] = true
					out = append(out, "fields."+f.Name)
				}
			}
		}
	}
	return out
}

type selRow struct {
	id   string
	edge *Edge
	ent  *patterns.Entity
}

// Select runs a SELECT over memory, concepts, connectors, links or entities.
func (b *Brain) Select(s *SelectSpec) (*SelectResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.selectx(s)
}

func (b *Brain) nearScores(q, mode string) map[string]float64 {
	switch mode {
	case "vector":
		sc := b.scorer(q)
		return sc.sc
	case "graph":
		qs := stems(b.expandQuery(q))
		out := map[string]float64{}
		for _, id := range b.order {
			n := b.nodes[id]
			ns := stems(n.Text)
			c := 0
			for x := range qs {
				if ns[x] {
					c++
				}
			}
			if c > 0 && len(qs) > 0 {
				out[id] = float64(c) / float64(len(qs)) * (.4 + .6*b.nodeW(n))
			}
		}
		return out
	}
	return b.hybrid(q).scores
}

func (b *Brain) selectx(s *SelectSpec) (*SelectResult, error) {
	from := s.From
	if from == "" {
		from = "memory"
	}
	if i := strings.Index(from, "@"); i >= 0 {
		s.Sources = append(s.Sources, from[i+1:])
		from = from[:i]
	}
	L := b.curLit()
	var score map[string]float64
	if s.Near != "" {
		score = b.nearScores(s.Near, s.Mode)
	}
	rows := []selRow{}
	switch from {
	case "memory", "concepts", "connectors", "nodes", "all", "quarantine":
		for _, id := range b.order {
			n := b.nodes[id]
			switch from {
			case "quarantine":
				if !isQuarantined(n) {
					continue
				}
			case "memory":
				if n.Kind == Concept || n.Conn != nil || isQuarantined(n) {
					continue
				}
				if n.Ext != nil && !(contains(s.Sources, "*") || contains(s.Sources, n.Ext.Source)) {
					continue
				}
			case "concepts":
				if n.Kind != Concept {
					continue
				}
			case "connectors":
				if n.Conn == nil {
					continue
				}
			}
			rows = append(rows, selRow{id: id})
		}
	case "links", "edges", "synapses":
		from = "links"
		for _, e := range b.edges {
			rows = append(rows, selRow{edge: e})
		}
	case "entities":
		for _, id := range b.memories() {
			for _, e := range b.entities(id) {
				e := e
				rows = append(rows, selRow{id: id, ent: &e})
			}
		}
	default:
		return nil, errf("unknown table %q (memory, concepts, connectors, links, entities, memory@source)", s.From)
	}
	// Every column name the statement touches, so a name that exists nowhere in this table can be
	// reported as a typo instead of silently reading as NULL ("WHERE banana < '2026-01-01'" used to
	// match every row). Validity is decided against the table's schema afterwards, never against the
	// rows that happened to be probed: short-circuit evaluation visits different rows depending on the
	// order of the conditions, and the verdict must not depend on that.
	referenced := map[string]bool{}
	get := func(r selRow) func(string) any {
		return func(col string) any {
			referenced[col] = true
			if r.edge != nil {
				switch col {
				case "a":
					return r.edge.A
				case "b":
					return r.edge.B
				case "w", "weight":
					return r.edge.W
				case "type":
					return r.edge.Type
				}
				return nil
			}
			if r.ent != nil {
				switch col {
				case "id", "node":
					if col == "id" {
						return r.id + "#" + fmt.Sprint(r.ent.Start)
					}
					return r.id
				case "type":
					return r.ent.Type
				case "value":
					if r.ent.Type == "card" || contains(b.Privacy, r.ent.Type) {
						return patterns.Mask(r.ent.Type, r.ent.Value)
					}
					return r.ent.Value
				case "raw":
					if r.ent.Type == "card" || contains(b.Privacy, r.ent.Type) {
						return patterns.Mask(r.ent.Type, r.ent.Raw)
					}
					return r.ent.Raw
				case "confidence":
					return r.ent.Confidence
				case "pii":
					return r.ent.PII
				}
				n := b.nodes[r.id]
				v, _ := b.column(n, col, L, score)
				return v
			}
			n := b.nodes[r.id]
			if strings.HasPrefix(col, "entities.") {
				t := col[9:]
				ev := EntVals{Type: t}
				for _, e := range b.entities(r.id) {
					if e.Type == t {
						ev.Vals = append(ev.Vals, e.Value)
					}
				}
				return ev
			}
			if strings.HasPrefix(col, "has:") {
				for _, e := range b.entities(r.id) {
					if e.Type == col[4:] {
						return true
					}
				}
				return false
			}
			switch col {
			case "external":
				return n.Ext != nil
			case "label":
				return n.Text
			case "concept":
				return ConceptRef{b, r.id}
			}
			if name, isField := strings.CutPrefix(col, "fields."); isField {
				fv, has := n.Fields.Get(name)
				if !has {
					return nil // untyped nil, not the "" that Fields.Get returns: this node has no such field
				}
				return fv
			}
			v, ok := b.column(n, col, L, score)
			if !ok {
				if fv, has := n.Fields.Get(col); has {
					return fv
				}
			}
			return v
		}
	}
	kept := []selRow{}
	for _, r := range rows {
		if s.Where != nil {
			ok, err := s.Where(get(r))
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
		}
		kept = append(kept, r)
	}
	ids := []string{}
	for _, r := range kept {
		if r.id != "" && r.ent == nil {
			ids = append(ids, r.id)
		}
	}
	// columns
	cols := []SelCol{}
	hasAgg := false
	for _, c := range s.Columns {
		switch {
		case c.Agg != "":
			hasAgg = true
			cols = append(cols, c)
		case c.Col == "*":
			for _, d := range b.defaultCols(from, ids) {
				cols = append(cols, SelCol{Col: d})
			}
		case c.Col == "fields.*":
			for _, d := range b.fieldCols(ids) {
				cols = append(cols, SelCol{Col: d})
			}
		default:
			cols = append(cols, c)
		}
	}
	if len(cols) == 0 {
		for _, d := range b.defaultCols(from, ids) {
			cols = append(cols, SelCol{Col: d})
		}
	}
	if s.Near != "" && !hasAgg {
		has := false
		for _, c := range cols {
			if c.Col == "score" {
				has = true
			}
		}
		if !has {
			cols = append(cols, SelCol{Col: "score"})
		}
	}
	names := []string{}
	for _, c := range cols {
		names = append(names, c.Name())
	}
	type outRow struct {
		m  *OMap
		id string
		g  func(string) any
	}
	out := []outRow{}
	if hasAgg || len(s.GroupBy) > 0 {
		groups := map[string][]selRow{}
		gorder := []string{}
		for _, r := range kept {
			g := get(r)
			parts := []string{}
			for _, k := range s.GroupBy {
				parts = append(parts, asString(g(k)))
			}
			key := strings.Join(parts, "\x00")
			if _, ok := groups[key]; !ok {
				gorder = append(gorder, key)
			}
			groups[key] = append(groups[key], r)
		}
		if len(s.GroupBy) == 0 && len(gorder) == 0 {
			gorder = []string{""}
		}
		for _, key := range gorder {
			grp := groups[key]
			m := NewOMap()
			var g func(string) any
			if len(grp) > 0 {
				g = get(grp[0])
			}
			for _, c := range cols {
				if c.Agg == "" {
					if g != nil {
						m.Set(c.Name(), plain(g(c.Col)))
					} else {
						m.Set(c.Name(), nil)
					}
					continue
				}
				m.Set(c.Name(), aggregate(c, grp, get))
			}
			out = append(out, outRow{m: m, g: func(k string) any {
				if v, ok := m.Vals[k]; ok {
					return v
				}
				if g != nil {
					return g(k)
				}
				return nil
			}})
		}
	} else {
		for _, r := range kept {
			g := get(r)
			m := NewOMap()
			for _, c := range cols {
				m.Set(c.Name(), plain(g(c.Col)))
			}
			out = append(out, outRow{m: m, id: r.id, g: g})
		}
	}
	order := s.Order
	if len(order) == 0 && s.Near != "" && !hasAgg {
		order = []OrderKey{{Col: "score", Desc: true}}
	}
	if len(order) > 0 {
		sort.SliceStable(out, func(i, j int) bool {
			for _, k := range order {
				c := cmpAny(plain(out[i].g(k.Col)), plain(out[j].g(k.Col)))
				if c == 0 {
					continue
				}
				if k.Desc {
					return c > 0
				}
				return c < 0
			}
			return false
		})
	}
	res := &SelectResult{Columns: names, Total: len(out), Rows: []*OMap{}, IDs: []string{}}
	if s.Offset > 0 {
		if s.Offset >= len(out) {
			out = nil
		} else {
			out = out[s.Offset:]
		}
	}
	max := s.MaxRows
	if max <= 0 {
		max = 1000
	}
	lim := s.Limit
	if lim <= 0 || lim > max {
		lim = max
	}
	if len(out) > lim {
		out = out[:lim]
	}
	for _, r := range out {
		for _, k := range r.m.Keys {
			if str, ok := r.m.Vals[k].(string); ok {
				r.m.Vals[k] = b.maskText(str)
			}
		}
		res.Rows = append(res.Rows, r.m)
		if r.id != "" {
			res.IDs = append(res.IDs, r.id)
		}
	}
	if err := b.unknownCols(referenced, from, rows, L, score); err != nil {
		return nil, err
	}
	return res, nil
}

// unknownCols reports referenced column names that this table has no definition for. A name is valid
// when it is a built-in column of the table, or a field carried by at least one of its rows; entity
// columns are open-ended (custom patterns define the types), so they are always accepted. Scanning all
// candidate rows rather than the filtered ones keeps the verdict independent of condition order.
func (b *Brain) unknownCols(referenced map[string]bool, from string, rows []selRow, L lit, score map[string]float64) error {
	if len(rows) == 0 {
		return nil // nothing was scanned, so nothing can be judged
	}
	fields := map[string]bool{}
	var probe *Node
	for _, r := range rows {
		n := b.nodes[r.id]
		if n == nil {
			continue
		}
		if probe == nil {
			probe = n
		}
		for _, f := range n.Fields {
			fields[f.Name] = true
		}
	}
	valid := func(col string) bool {
		if strings.HasPrefix(col, "entities.") || strings.HasPrefix(col, "has:") {
			return true
		}
		if name, isField := strings.CutPrefix(col, "fields."); isField {
			return fields[name]
		}
		if fields[col] {
			return true
		}
		switch from {
		case "links":
			switch col {
			case "a", "b", "w", "weight", "type":
				return true
			}
			return false
		case "entities":
			switch col {
			case "id", "node", "type", "value", "raw", "confidence", "pii":
				return true
			}
		}
		switch col {
		case "external", "label", "concept":
			return true
		}
		if probe == nil {
			return true // cannot tell without a node to ask
		}
		_, ok := b.column(probe, col, L, score)
		return ok
	}
	bad := []string{}
	for col := range referenced {
		if !valid(col) {
			bad = append(bad, col)
		}
	}
	if len(bad) == 0 {
		return nil
	}
	sort.Strings(bad)
	return errf("unknown column %s in %s (has: %s)", quoteList(bad), from, strings.Join(b.tableCols(from, rows), ", "))
}

// tableCols lists the column names a table offers, for the "unknown column" hint.
func (b *Brain) tableCols(from string, rows []selRow) []string {
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.id != "" {
			ids = append(ids, r.id)
		}
	}
	out := b.defaultCols(from, ids)
	if from == "links" || from == "entities" {
		return out
	}
	return append(out, "age", "touched", "base", "pinned", "superseded", "labels", "degree")
}

func quoteList(ss []string) string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = strconv.Quote(s)
	}
	return strings.Join(out, ", ")
}

// plain converts column values to output scalars.
func plain(v any) any {
	switch x := v.(type) {
	case EntVals:
		if len(x.Vals) == 0 {
			return nil
		}
		return strings.Join(x.Vals, ", ")
	case []string:
		return strings.Join(x, ", ")
	}
	return v
}

func aggregate(c SelCol, grp []selRow, get func(selRow) func(string) any) any {
	if c.Agg == "COUNT" {
		if c.Col == "*" {
			return len(grp)
		}
		n := 0
		for _, r := range grp {
			if v := plain(get(r)(c.Col)); v != nil && v != "" {
				n++
			}
		}
		return n
	}
	vals := []float64{}
	for _, r := range grp {
		if v, ok := parseNum(asString(plain(get(r)(c.Col)))); ok {
			vals = append(vals, v)
		}
	}
	if len(vals) == 0 {
		return nil
	}
	switch c.Agg {
	case "SUM", "AVG":
		s := 0.0
		for _, v := range vals {
			s += v
		}
		if c.Agg == "AVG" {
			return roundN(s/float64(len(vals)), 4)
		}
		return s
	case "MIN":
		m := math.Inf(1)
		for _, v := range vals {
			m = math.Min(m, v)
		}
		return m
	case "MAX":
		m := math.Inf(-1)
		for _, v := range vals {
			m = math.Max(m, v)
		}
		return m
	}
	return nil
}

// CompareValues compares a column value with a literal (typed for entities, numeric when both parse).
// It is called from inside Select's Where callback, so it does not lock.
//
// A missing value (NULL: a field the node does not carry) matches no comparison at all, not even "!=".
// Without that rule an absent value would compare as the empty string, and every "<" would be true for
// every node that lacks the field.
func (b *Brain) CompareValues(v any, op string, lit string) bool {
	if v == nil {
		return false
	}
	if cr, ok := v.(ConceptRef); ok {
		m := cr.b.hasConcept(cr.id, lit)
		if op == "!=" || op == "<>" {
			return !m
		}
		return m
	}
	if ev, ok := v.(EntVals); ok {
		for _, x := range ev.Vals {
			if op == "~" {
				if strings.Contains(lowerDeacc(x), lowerDeacc(lit)) {
					return true
				}
				continue
			}
			norm := lit
			if es := b.patOpts().ExtractTypes(lit, []string{ev.Type}); len(es) > 0 {
				norm = es[0].Value
			}
			c, ok := b.patOpts().CompareValues(ev.Type, x, norm)
			if !ok {
				continue
			}
			if cmpOK(c, op) {
				return true
			}
		}
		return op == "!=" && len(ev.Vals) == 0
	}
	if bv, ok := v.(bool); ok {
		lb, err := parseBool(lit)
		if err == nil {
			return cmpOK(map[bool]int{true: 0, false: 1}[bv == lb], op) && (op == "=" || op == "!=")
		}
	}
	return compare(asString(plain(v)), op, lit)
}

func cmpOK(c int, op string) bool {
	switch op {
	case "=":
		return c == 0
	case "!=", "<>":
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
