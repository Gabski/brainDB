// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

// Template fill: deterministic slot filling from memory (no LLM). Two template styles:
//
//   - schema templates (JSON / YAML / XML): every string leaf is a slot "description[:type][|option…]";
//     list slots are written as a key "deals[]" with item fields "deals[].title", or (JSON) as an array holding one
//     object whose optional "_each" key is the list description (default: the key name), or (XML) an element with a
//     repeat="…" attribute;
//   - text templates (Markdown or any text): {{ description[:type][|options] }} and {{#each …}} … {{/each}}.
//
// Inside a list item, ".id .kind .text .section .weight .activation .fields.<name>" read the item itself and other
// descriptions are searched in the item and its neighbourhood (2 hops).

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Gabski/brainDB/internal/patterns"
)

// FillRequest is a template fill request.
type FillRequest struct {
	Template       string   `json:"template"`
	TemplateFormat string   `json:"templateFormat,omitempty"`
	Output         string   `json:"output,omitempty"`
	Scope          string   `json:"scope,omitempty"`
	WithSources    bool     `json:"withSources,omitempty"`
	Min            *float64 `json:"min,omitempty"`
}

// SlotValue reports how one slot was filled.
type SlotValue struct {
	Path        string  `json:"path"`
	Description string  `json:"description"`
	Type        string  `json:"type"`
	Value       any     `json:"value"`
	Source      string  `json:"source,omitempty"`
	Score       float64 `json:"score"`
	Snippet     string  `json:"snippet,omitempty"`
}

// FillResult is the result of a fill.
type FillResult struct {
	Output  string      `json:"output"`
	Format  string      `json:"format"`
	Values  []SlotValue `json:"values"`
	Missing []string    `json:"missing"`
}

// SlotTypes are the supported slot types.
var SlotTypes = map[string]bool{"text": true, "name": true, "number": true, "money": true, "percent": true, "date": true, "email": true,
	"phone": true, "url": true, "bool": true, "list": true, "id": true, "org": true, "domain": true, "duration": true, "iban": true,
	"vat": true, "address": true, "version": true}

type slot struct {
	raw, desc, typ string
	opts           map[string]string
	itemField      string // ".text" etc.
}

func parseSlot(s string) slot {
	parts := strings.Split(s, "|")
	head := strings.TrimSpace(parts[0])
	sl := slot{raw: s, opts: map[string]string{}, typ: "text"}
	for _, p := range parts[1:] {
		k, v, _ := strings.Cut(strings.TrimSpace(p), "=")
		sl.opts[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	if strings.HasPrefix(head, ".") {
		sl.itemField = head
		sl.desc = head
		return sl
	}
	if i := strings.LastIndex(head, ":"); i >= 0 && SlotTypes[strings.TrimSpace(head[i+1:])] {
		sl.typ = strings.TrimSpace(head[i+1:])
		head = strings.TrimSpace(head[:i])
	}
	sl.desc = head
	return sl
}

type filler struct {
	b       *Brain
	req     FillRequest
	min     float64
	values  []SlotValue
	missing []string
	sources *OMap
	notes   []string
	L       lit
}

type cand struct {
	id    string
	score float64
}

func (f *filler) neighbourhood(item string, hops int) map[string]bool {
	seen := map[string]bool{item: true}
	fr := []string{item}
	for h := 0; h < hops; h++ {
		nx := []string{}
		for _, u := range fr {
			for _, e := range f.b.adj[u] {
				v := other(e, u)
				if !seen[v] {
					seen[v] = true
					nx = append(nx, v)
				}
			}
		}
		fr = nx
	}
	return seen
}

func (f *filler) kindsMentioned(desc string) []string {
	ws := map[string]bool{}
	for _, w := range asciiWords(desc) {
		ws[w] = true
	}
	out := []string{}
	for _, k := range f.b.kinds {
		if k.Name == Concept || k.Name == ConnectorKind {
			continue
		}
		for _, n := range []string{k.Name, k.Label} {
			ln := lowerDeacc(n)
			if ws[ln] || ws[ln+"s"] || ws[ln+"es"] || (strings.HasSuffix(ln, "y") && ws[ln[:len(ln)-1]+"ies"]) {
				out = append(out, k.Name)
				break
			}
		}
	}
	return out
}

// candidates ranks memories for a slot description.
func (f *filler) candidates(sl slot, pathCtx, item string) []cand {
	b := f.b
	q := sl.desc
	if pathCtx != "" {
		q += " " + pathCtx
	}
	h := b.hybrid(q)
	var pool map[string]bool
	if item != "" {
		pool = f.neighbourhood(item, 2)
	}
	kinds := splitIDs(sl.opts["kind"])
	if len(kinds) == 0 && item == "" {
		kinds = f.kindsMentioned(sl.desc)
	}
	scope := sl.opts["scope"]
	if scope == "" {
		scope = f.req.Scope
	}
	out := []cand{}
	for _, id := range h.s.ranked {
		n := b.nodes[id]
		if n.Kind == Concept || n.Conn != nil {
			continue
		}
		if n.Ext != nil && scope != "federated" {
			continue
		}
		if pool != nil && !pool[id] {
			continue
		}
		if s := sl.opts["section"]; s != "" && n.Section != s {
			continue
		}
		if c := sl.opts["concept"]; c != "" && !b.hasConcept(id, c) {
			continue
		}
		if scope == "context" && !f.L.r.has(id) {
			continue
		}
		sc := h.scores[id]
		if f.L.r.has(id) {
			sc *= 1.15
		}
		if id == item {
			sc *= 1.2
		}
		out = append(out, cand{id, sc})
	}
	if len(kinds) > 0 {
		k2 := []cand{}
		for _, c := range out {
			if contains(kinds, b.nodes[c.id].Kind) {
				k2 = append(k2, c)
			}
		}
		if len(k2) > 0 || sl.opts["kind"] != "" {
			out = k2
		}
	}
	stableSortDesc(out, func(c cand) float64 { return c.score })
	return out
}

var patType = map[string]string{"date": "date", "money": "money", "percent": "percent", "email": "email", "phone": "phone", "url": "url",
	"name": "person", "org": "org", "domain": "domain", "duration": "duration", "iban": "iban", "vat": "vat", "address": "address",
	"version": "version", "id": "id_code"}

var (
	// A full stop ends a clause only when whitespace follows it, so "brain.xml", "0.0.2" and "MRR 0.57"
	// survive the split instead of being cut in half. Go RE2 has no lookarounds, hence the alternation.
	clauseSplit = regexp.MustCompile(`(?:\.+\s+|[;·—]+\s*|\n+)`)
	plainNum    = regexp.MustCompile(`(?:^|[^\w-])(-?\d+(?:[.,]\d+)?)`)
	capSeq      = regexp.MustCompile(`\p{Lu}[\p{L}'-]+(?:\s+\p{Lu}[\p{L}'-]+)+`)
	boolYes     = regexp.MustCompile(`(?i)\b(yes|true|required|mandatory|enabled|approved|signed|confirmed|done)\b`)
	boolNo      = regexp.MustCompile(`(?i)\b(no|not|false|optional|disabled|rejected|declined|missing|none)\b`)
)

// fieldsFor ranks field names that a slot description refers to (explicit field= first; then by matched words,
// preferring later words in the description, which usually name the attribute: "deal value" → value).
func (f *filler) fieldsFor(sl slot, cs []cand) []string {
	if fl := sl.opts["field"]; fl != "" {
		return []string{fl}
	}
	dw := asciiWords(sl.desc)
	kindWords := map[string]bool{}
	for _, k := range f.kindsMentioned(sl.desc) {
		kindWords[lowerDeacc(k)] = true
	}
	type fc struct {
		name  string
		words int
		pos   int
	}
	out := []fc{}
	seen := map[string]bool{}
	for i, c := range cs {
		if i >= 8 {
			break
		}
		for _, fl := range f.b.nodes[c.id].Fields {
			if seen[fl.Name] {
				continue
			}
			seen[fl.Name] = true
			fw := asciiWords(strings.ReplaceAll(fl.Name, "_", " "))
			if len(fw) == 0 || (len(fw) == 1 && kindWords[fw[0]]) {
				continue
			}
			all, last := true, -1
			for _, w := range fw {
				hit := -1
				for j, d := range dw {
					st, sd := w, d
					if len(st) > 5 {
						st = st[:5]
					}
					if len(sd) > 5 {
						sd = sd[:5]
					}
					if d == w || (len(st) >= 4 && st == sd) {
						hit = j
					}
				}
				if hit < 0 {
					all = false
					break
				}
				if hit > last {
					last = hit
				}
			}
			if all {
				out = append(out, fc{fl.Name, len(fw), last})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].words != out[j].words {
			return out[i].words > out[j].words
		}
		return out[i].pos > out[j].pos
	})
	names := []string{}
	for _, x := range out {
		names = append(names, x.name)
	}
	return names
}

// typedOK checks that a field value fits a typed slot.
func (f *filler) typedOK(typ, v string) bool {
	switch typ {
	case "number", "money", "percent":
		if _, ok := patterns.ParseAmount(strings.Trim(v, " %€$£"), false); ok {
			return true
		}
		return len(f.b.patOpts().ExtractTypes(v, []string{"money", "percent", "number_unit"})) > 0
	case "date":
		_, ok := patterns.NormalizeDate(v)
		return ok
	case "bool":
		_, ok := f.convert("bool", v)
		return ok
	}
	if pt := patType[typ]; pt != "" && typ != "name" && typ != "id" {
		return len(f.b.patOpts().ExtractTypes(v, []string{pt})) > 0
	}
	return true
}

func (f *filler) convert(typ, raw string) (any, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	switch typ {
	case "number":
		if v, ok := patterns.ParseAmount(raw, false); ok {
			return v, true
		}
		if m := plainNum.FindStringSubmatch(raw); m != nil {
			v, _ := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64)
			return v, true
		}
		return nil, false
	case "date":
		if d, ok := patterns.NormalizeDate(raw); ok {
			return d, true
		}
		return raw, true
	case "bool":
		if boolYes.MatchString(raw) {
			return true, true
		}
		if boolNo.MatchString(raw) {
			return false, true
		}
		return nil, false
	case "list":
		parts := []any{}
		for _, p := range regexp.MustCompile(`\s*[,;]\s*|\s+and\s+`).Split(raw, -1) {
			if p = strings.TrimSpace(p); p != "" {
				parts = append(parts, p)
			}
		}
		return parts, len(parts) > 0
	}
	return raw, true
}

// extract computes a value from one candidate.
func (f *filler) extract(sl slot, id string) (any, string, bool) {
	n := f.b.nodes[id]
	texts := []string{n.Text}
	for _, fl := range n.Fields {
		texts = append(texts, fl.Value)
	}
	all := strings.Join(texts, " · ")
	switch sl.typ {
	case "text":
		return f.bestClause(sl.desc, n.Text), n.Text, true
	case "list":
		v, ok := f.convert("list", f.bestClause(sl.desc, n.Text))
		return v, n.Text, ok
	case "bool":
		v, ok := f.convert("bool", all)
		return v, n.Text, ok
	case "number":
		for _, e := range f.b.patOpts().ExtractTypes(all, []string{"number_unit", "money", "percent"}) {
			return e.Amount, e.Raw, true
		}
		if m := plainNum.FindStringSubmatch(all); m != nil {
			v, _ := f.convert("number", m[1])
			return v, m[1], true
		}
		return nil, "", false
	}
	pt := patType[sl.typ]
	if pt != "" {
		for _, e := range f.b.patOpts().ExtractTypes(all, []string{pt}) {
			switch sl.typ {
			case "money", "percent":
				return e.Raw, e.Raw, true
			}
			return e.Value, e.Raw, true
		}
	}
	switch sl.typ {
	case "id":
		return id, n.Text, true
	case "name":
		for _, e := range f.b.patOpts().ExtractTypes(all, []string{"org"}) {
			return e.Value, e.Raw, true
		}
		if m := capSeq.FindString(n.Text); m != "" {
			return m, m, true
		}
	}
	return nil, "", false
}

func (f *filler) bestClause(desc, text string) string {
	cand := []string{}
	for _, p := range clauseSplit.Split(text, -1) {
		if p = strings.TrimSpace(p); jsLen(p) >= 2 {
			cand = append(cand, p)
		}
	}
	if len(cand) == 0 {
		return flat(text)
	}
	// one batch: the description plus every candidate clause, so a remote embedder costs a single round trip
	vs := f.b.embedBatch(append([]string{desc}, cand...))
	qv := vs[0]
	best, bs := "", -2.0
	for i, p := range cand {
		if s := vcos(qv, vs[i+1]); s > bs+1e-12 {
			best, bs = p, s
		}
	}
	if best == "" {
		return flat(text)
	}
	return best
}

// itemValue reads ".text", ".fields.x" etc. from an item.
func (f *filler) itemValue(field, id string) any {
	n := f.b.nodes[id]
	if n == nil {
		return nil
	}
	v, _ := f.b.column(n, strings.TrimPrefix(field, "."), f.L, nil)
	return v
}

// fillSlot fills one scalar slot.
func (f *filler) fillSlot(path, s, pathCtx, item string) any {
	sl := parseSlot(s)
	sv := SlotValue{Path: path, Description: sl.desc, Type: sl.typ}
	if sl.itemField != "" {
		if item == "" {
			sv.Type = "field"
			f.record(sv, true)
			return nil
		}
		v := f.itemValue(sl.itemField, item)
		sv.Type, sv.Value, sv.Source, sv.Score, sv.Snippet = "field", v, item, 1, short(flat(f.b.nodes[item].Text), 120)
		f.record(sv, v == nil)
		return f.mask(v)
	}
	min := f.min
	if m, ok := sl.opts["min"]; ok {
		min = atof(m, min)
	}
	cs := f.candidates(sl, pathCtx, item)
	fmin := min
	if sl.opts["field"] != "" {
		fmin = 0
	}
	type pick struct {
		val          any
		src, snippet string
		score        float64
		ok           bool
	}
	var fp, xp pick
	for _, fl := range f.fieldsFor(sl, cs) {
		for _, c := range cs {
			if v, ok := f.b.nodes[c.id].Fields.Get(fl); ok && v != "" && c.score >= fmin && f.typedOK(sl.typ, v) {
				if val, ok := f.convertField(sl.typ, v); ok {
					fp = pick{val, c.id, short(flat(f.b.nodes[c.id].Text), 120), c.score, true}
					break
				}
			}
		}
		if fp.ok {
			break
		}
	}
	if !(fp.ok && sl.opts["field"] != "") {
		for i, c := range cs {
			if i >= 8 || c.score < min || (fp.ok && c.score <= fp.score) {
				break
			}
			if v, raw, ok := f.extract(sl, c.id); ok && v != nil {
				xp = pick{v, c.id, short(flat(raw), 120), c.score, true}
				break
			}
		}
	}
	best := fp
	if xp.ok && (!fp.ok || xp.score > fp.score) {
		best = xp
	}
	if best.ok {
		sv.Value, sv.Source, sv.Score, sv.Snippet = best.val, best.src, best.score, best.snippet
		f.record(sv, false)
		return f.mask(best.val)
	}
	if len(cs) > 0 {
		sv.Score = cs[0].score
	}
	f.record(sv, true)
	return nil
}

func (f *filler) convertField(typ, v string) (any, bool) {
	switch typ {
	case "text", "name", "id", "email", "phone", "url", "org", "domain", "address", "version", "iban", "vat", "duration":
		return v, true
	case "money", "percent":
		return v, true
	}
	return f.convert(typ, v)
}

func (f *filler) mask(v any) any {
	s, ok := v.(string)
	if !ok {
		return v
	}
	return f.b.maskText(s)
}

func (f *filler) record(sv SlotValue, missing bool) {
	if missing {
		sv.Value = nil
		f.missing = append(f.missing, sv.Path)
	} else if sv.Source != "" && f.req.WithSources {
		f.sources.Set(sv.Path, sv.Source)
	}
	f.values = append(f.values, sv)
}

// listItems picks the items of a list slot.
func (f *filler) listItems(path, spec, pathCtx, item string) []string {
	sl := parseSlot(spec)
	limit := 5
	if l, ok := sl.opts["limit"]; ok {
		limit = atoi(l, 5)
	}
	min := f.min / 2
	if sl.opts["kind"] != "" || sl.opts["section"] != "" || sl.opts["concept"] != "" {
		min = 0 // an explicit filter lists every match (up to limit)
	}
	if m, ok := sl.opts["min"]; ok {
		min = atof(m, min)
	}
	cs := f.candidates(sl, pathCtx, item)
	out := []string{}
	for _, c := range cs {
		if len(out) >= limit || c.score < min {
			break
		}
		if c.id == item {
			continue
		}
		out = append(out, c.id)
	}
	sv := SlotValue{Path: path, Description: sl.desc, Type: "list", Value: len(out)}
	if len(out) > 0 {
		sv.Source, sv.Score = out[0], cs[0].score
	}
	f.record(sv, len(out) == 0)
	return out
}

// ---------------------------------------------------------------- schema templates (JSON/YAML)

func joinPath(p, k string) string {
	if p == "" {
		return k
	}
	return p + "." + k
}

func ctxWords(p string) string {
	r := strings.NewReplacer(".", " ", "[]", " ", "_", " ", "-", " ")
	return strings.TrimSpace(r.Replace(regexp.MustCompile(`\[\d+\]`).ReplaceAllString(p, " ")))
}

func (f *filler) fillValue(path string, v any, item string) any {
	switch x := v.(type) {
	case string:
		return f.fillSlot(path, x, ctxWords(parentPath(path)), item)
	case *OMap:
		return f.fillMap(path, x, item)
	case []any:
		if len(x) == 1 {
			if m, ok := x[0].(*OMap); ok {
				spec := lastKey(path)
				tpl := NewOMap()
				for _, k := range m.Keys {
					if k == "_each" {
						spec = scalarString(m.Vals[k])
						continue
					}
					tpl.Set(k, m.Vals[k])
				}
				return f.fillList(path, spec, tpl, item)
			}
			if s, ok := x[0].(string); ok && !strings.HasPrefix(s, ".") {
				ids := f.listItems(path, s, ctxWords(parentPath(path)), item)
				out := []any{}
				for i, id := range ids {
					out = append(out, f.fillSlot(fmt.Sprintf("%s[%d]", path, i), ".text", "", id))
				}
				return out
			}
		}
		out := []any{}
		for i, it := range x {
			out = append(out, f.fillValue(fmt.Sprintf("%s[%d]", path, i), it, item))
		}
		return out
	}
	return v
}

func parentPath(p string) string {
	if i := strings.LastIndexAny(p, ".["); i >= 0 {
		return p[:i]
	}
	return ""
}

func lastKey(p string) string {
	if i := strings.LastIndex(p, "."); i >= 0 {
		return p[i+1:]
	}
	return p
}

func (f *filler) fillList(path, spec string, tpl *OMap, item string) []any {
	ids := f.listItems(path, spec, ctxWords(parentPath(path)), item)
	out := []any{}
	for i, id := range ids {
		out = append(out, f.fillMap(fmt.Sprintf("%s[%d]", path, i), tpl, id))
	}
	return out
}

func (f *filler) fillMap(path string, m *OMap, item string) *OMap {
	out := NewOMap()
	for _, k := range m.Keys {
		v := m.Vals[k]
		if strings.Contains(k, "[].") {
			continue
		}
		if strings.HasSuffix(k, "[]") {
			name := strings.TrimSuffix(k, "[]")
			tpl := NewOMap()
			for _, k2 := range m.Keys {
				if strings.HasPrefix(k2, k+".") {
					tpl.Set(strings.TrimPrefix(k2, k+"."), m.Vals[k2])
				}
			}
			spec, _ := v.(string)
			if spec == "" {
				spec = name
			}
			if len(tpl.Keys) == 0 {
				tpl.Set("text", ".text")
			}
			out.Set(name, f.fillList(joinPath(path, name), spec, tpl, item))
			continue
		}
		out.Set(k, f.fillValue(joinPath(path, k), v, item))
	}
	return out
}

// ---------------------------------------------------------------- XML templates

func (f *filler) fillXML(path string, el *XMLEl, item string) []*XMLEl {
	if spec, ok := el.Attr("repeat"); ok {
		ids := f.listItems(joinPath(path, el.Name), spec, ctxWords(path), item)
		out := []*XMLEl{}
		tpl := &XMLEl{Name: el.Name, Text: el.Text, Children: el.Children}
		for _, a := range el.Attrs {
			if a.Name != "repeat" {
				tpl.Attrs = append(tpl.Attrs, a)
			}
		}
		for i, id := range ids {
			out = append(out, f.fillXMLOne(fmt.Sprintf("%s[%d]", joinPath(path, el.Name), i), tpl, id)...)
		}
		return out
	}
	return f.fillXMLOne(joinPath(path, el.Name), el, item)
}

func (f *filler) fillXMLOne(path string, el *XMLEl, item string) []*XMLEl {
	o := &XMLEl{Name: el.Name}
	src := ""
	for _, a := range el.Attrs {
		v := f.fillSlot(path+"@"+a.Name, a.Value, ctxWords(path), item)
		if v != nil {
			o.Attrs = append(o.Attrs, XMLAttr{a.Name, asString(v)})
			if src == "" && len(f.values) > 0 {
				src = f.values[len(f.values)-1].Source
			}
		}
	}
	if t := strings.TrimSpace(el.Text); t != "" {
		v := f.fillSlot(path, t, ctxWords(parentPath(path)), item)
		if v != nil {
			o.Text = asString(v)
			if len(f.values) > 0 {
				src = f.values[len(f.values)-1].Source
			}
		}
	}
	for _, c := range el.Children {
		o.Children = append(o.Children, f.fillXML(path, c, item)...)
	}
	if f.req.WithSources && src != "" {
		o.Attrs = append(o.Attrs, XMLAttr{"source", src})
	}
	return []*XMLEl{o}
}

func renderXMLEl(b *strings.Builder, e *XMLEl, ind int) {
	pad := strings.Repeat("  ", ind)
	b.WriteString(pad + "<" + e.Name)
	for _, a := range e.Attrs {
		b.WriteString(" " + a.Name + `="` + esc(a.Value) + `"`)
	}
	if len(e.Children) == 0 && e.Text == "" {
		b.WriteString("/>\n")
		return
	}
	b.WriteString(">")
	if len(e.Children) == 0 {
		b.WriteString(esc(e.Text) + "</" + e.Name + ">\n")
		return
	}
	b.WriteString("\n")
	if e.Text != "" {
		b.WriteString(pad + "  " + esc(e.Text) + "\n")
	}
	for _, c := range e.Children {
		renderXMLEl(b, c, ind+1)
	}
	b.WriteString(pad + "</" + e.Name + ">\n")
}

func xmlToValue(e *XMLEl) any {
	if len(e.Attrs) == 0 && len(e.Children) == 0 {
		if e.Text == "" {
			return nil
		}
		return e.Text
	}
	m := NewOMap()
	for _, a := range e.Attrs {
		m.Set(a.Name, a.Value)
	}
	counts := map[string]int{}
	for _, c := range e.Children {
		counts[c.Name]++
	}
	for _, c := range e.Children {
		v := xmlToValue(c)
		if counts[c.Name] > 1 {
			l, _ := m.Vals[c.Name].([]any)
			m.Set(c.Name, append(l, v))
		} else {
			m.Set(c.Name, v)
		}
	}
	if e.Text != "" {
		m.Set("text", e.Text)
	}
	return m
}

// valueToXML renders a filled schema value as XML.
func valueToXML(b *strings.Builder, name string, v any, ind int) {
	pad := strings.Repeat("  ", ind)
	name = xmlName(name)
	switch x := v.(type) {
	case *OMap:
		b.WriteString(pad + "<" + name + ">\n")
		for _, k := range x.Keys {
			valueToXML(b, k, x.Vals[k], ind+1)
		}
		b.WriteString(pad + "</" + name + ">\n")
	case []any:
		b.WriteString(pad + "<" + name + ">\n")
		for _, it := range x {
			valueToXML(b, "item", it, ind+1)
		}
		b.WriteString(pad + "</" + name + ">\n")
	case nil:
		b.WriteString(pad + "<" + name + "/>\n")
	default:
		b.WriteString(pad + "<" + name + ">" + esc(asString(v)) + "</" + name + ">\n")
	}
}

func valueToMD(b *strings.Builder, key string, v any, ind int) {
	pad := strings.Repeat("  ", ind)
	switch x := v.(type) {
	case *OMap:
		if key != "" {
			b.WriteString(pad + "- **" + key + "**\n")
			ind++
		}
		for _, k := range x.Keys {
			valueToMD(b, k, x.Vals[k], ind)
		}
	case []any:
		b.WriteString(pad + "- **" + key + "**\n")
		for i, it := range x {
			valueToMD(b, fmt.Sprintf("%d", i+1), it, ind+1)
		}
	case nil:
		b.WriteString(pad + "- **" + key + "**: _missing_\n")
	default:
		b.WriteString(pad + "- **" + key + "**: " + asString(v) + "\n")
	}
}

// ---------------------------------------------------------------- text templates

type tnode struct {
	text  string
	slot  string
	each  string
	kids  []*tnode
	isTxt bool
}

var tplTok = regexp.MustCompile(`\{\{\s*(#each\s+[^}]*?|/each|[^}]*?)\s*\}\}`)

func parseTextTemplate(s string) ([]*tnode, error) {
	root := &tnode{}
	stack := []*tnode{root}
	last := 0
	for _, m := range tplTok.FindAllStringSubmatchIndex(s, -1) {
		top := stack[len(stack)-1]
		if m[0] > last {
			top.kids = append(top.kids, &tnode{text: s[last:m[0]], isTxt: true})
		}
		last = m[1]
		inner := strings.TrimSpace(s[m[2]:m[3]])
		switch {
		case strings.HasPrefix(inner, "#each"):
			n := &tnode{each: strings.TrimSpace(strings.TrimPrefix(inner, "#each"))}
			top.kids = append(top.kids, n)
			stack = append(stack, n)
		case inner == "/each":
			if len(stack) == 1 {
				return nil, errf("{{/each}} without {{#each}}")
			}
			stack = stack[:len(stack)-1]
		case inner == "":
		default:
			top.kids = append(top.kids, &tnode{slot: inner})
		}
	}
	if len(stack) > 1 {
		return nil, errf("unclosed {{#each}}")
	}
	if last < len(s) {
		root.kids = append(root.kids, &tnode{text: s[last:], isTxt: true})
	}
	return root.kids, nil
}

func (f *filler) renderText(b *strings.Builder, nodes []*tnode, path, item string) {
	for _, n := range nodes {
		switch {
		case n.isTxt:
			b.WriteString(n.text)
		case n.each != "":
			ids := f.listItems(joinPath(path, "each"), n.each, "", item)
			for i, id := range ids {
				f.renderText(b, n.kids, fmt.Sprintf("%s[%d]", joinPath(path, "each"), i), id)
			}
		default:
			p := joinPath(path, parseSlot(n.slot).desc)
			v := f.fillSlot(p, n.slot, "", item)
			if v == nil {
				b.WriteString("[missing: " + parseSlot(n.slot).desc + "]")
				continue
			}
			b.WriteString(asString(listString(v)))
			if f.req.WithSources {
				sv := f.values[len(f.values)-1]
				f.notes = append(f.notes, fmt.Sprintf("[^%d]: %s — %s", len(f.notes)+1, sv.Source, sv.Snippet))
				b.WriteString(fmt.Sprintf("[^%d]", len(f.notes)))
			}
		}
	}
}

func listString(v any) any {
	if l, ok := v.([]any); ok {
		ps := []string{}
		for _, x := range l {
			ps = append(ps, asString(x))
		}
		return strings.Join(ps, ", ")
	}
	return v
}

// ---------------------------------------------------------------- entry point

func detectTemplateFormat(t string) string {
	s := strings.TrimSpace(t)
	switch {
	case strings.HasPrefix(s, "{") || strings.HasPrefix(s, "["):
		if _, err := DecodeOrderedJSON([]byte(s)); err == nil {
			return "json"
		}
	case strings.HasPrefix(s, "<"):
		return "xml"
	}
	if strings.Contains(s, "{{") {
		return "md"
	}
	if v, err := ParseYAML(s); err == nil {
		if _, ok := v.(*OMap); ok {
			return "yaml"
		}
	}
	return "md"
}

// Fill fills a template from memory.
func (b *Brain) Fill(req FillRequest) (*FillResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.fill(req)
}

func (b *Brain) fill(req FillRequest) (*FillResult, error) {
	if strings.TrimSpace(req.Template) == "" {
		return nil, errf("empty template")
	}
	f := &filler{b: b, req: req, min: .2, sources: NewOMap(), values: []SlotValue{}, missing: []string{}, L: b.curLit()}
	if req.Min != nil {
		f.min = *req.Min
	}
	tf := req.TemplateFormat
	if tf == "" || tf == "auto" {
		tf = detectTemplateFormat(req.Template)
	}
	out := req.Output
	if out == "" || out == "same" {
		out = tf
	}
	res := &FillResult{Format: out}
	var filled any
	switch tf {
	case "json":
		v, err := DecodeOrderedJSON([]byte(req.Template))
		if err != nil {
			return nil, errf("template JSON: %v", err)
		}
		filled = f.fillValue("", v, "")
	case "yaml":
		v, err := ParseYAML(req.Template)
		if err != nil {
			return nil, errf("template YAML: %v", err)
		}
		filled = f.fillValue("", v, "")
	case "xml":
		root := ParseXML(req.Template)
		if root == nil {
			return nil, errf("template XML: no root element")
		}
		els := f.fillXML("", root, "")
		if out == "xml" {
			var x strings.Builder
			for _, e := range els {
				renderXMLEl(&x, e, 0)
			}
			res.Output = x.String()
		} else if len(els) > 0 {
			m := NewOMap()
			m.Set(els[0].Name, xmlToValue(els[0]))
			filled = m
		}
	case "md", "text", "markdown":
		nodes, err := parseTextTemplate(req.Template)
		if err != nil {
			return nil, err
		}
		var x strings.Builder
		f.renderText(&x, nodes, "", "")
		if len(f.notes) > 0 {
			x.WriteString("\n\n" + strings.Join(f.notes, "\n") + "\n")
		}
		res.Output, res.Format = x.String(), "md"
	default:
		return nil, errf("templateFormat must be json, yaml, xml, md or auto")
	}
	if filled != nil {
		if req.WithSources && len(f.sources.Keys) > 0 {
			if m, ok := filled.(*OMap); ok && out != "xml" && out != "md" {
				m.Set("_sources", f.sources)
			}
		}
		switch out {
		case "json":
			res.Output = jsonIndent(filled)
		case "yaml":
			res.Output = EmitYAML(filled)
		case "xml":
			var x strings.Builder
			if m, ok := filled.(*OMap); ok && len(m.Keys) == 1 && tf == "xml" {
				valueToXML(&x, m.Keys[0], m.Vals[m.Keys[0]], 0)
			} else {
				valueToXML(&x, "fill", filled, 0)
			}
			res.Output = x.String()
		case "md", "markdown":
			var x strings.Builder
			valueToMD(&x, "", filled, 0)
			if req.WithSources {
				x.WriteString("\n")
				for i, k := range f.sources.Keys {
					x.WriteString(fmt.Sprintf("[^%d]: %s → %s\n", i+1, k, f.sources.Vals[k]))
				}
			}
			res.Output, res.Format = x.String(), "md"
		default:
			return nil, errf("output must be same, json, yaml, xml or md")
		}
	}
	res.Output = b.maskText(res.Output)
	res.Values, res.Missing = f.values, f.missing
	return res, nil
}
