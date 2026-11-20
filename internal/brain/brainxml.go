// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
)

const dayMs = 864e5

func isoMs(ms int64) string { return time.UnixMilli(ms).UTC().Format("2006-01-02T15:04:05.000Z") }

func parseISO(s string) (int64, bool) {
	for _, l := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(l, s); err == nil {
			return t.UnixMilli(), true
		}
	}
	return 0, false
}

func (b *Brain) touchOf(n *Node) int64 {
	if n.Touched == 0 {
		n.Touched = int64(float64(b.nowMs()) - n.Age*dayMs)
	}
	return n.Touched
}

func (b *Brain) ageNow(n *Node) float64 { return round2(float64(b.nowMs()-b.touchOf(n)) / dayMs) }

func atof(s string, def float64) float64 {
	if s == "" {
		return def
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return def
	}
	return v
}

func atoi(s string, def int) int { return int(atof(s, float64(def))) }

func splitIDs(s string) []string {
	out := []string{}
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

var legacyEdge = map[string]string{"abstr": EdgeConcept, "sup": EdgeSupersede, "sim": EdgeSimilar}

// LoadXML replaces the brain with the content of a brain XML document. It also accepts the legacy dialect
// (<abstractions>, <context seeds=…>, kind "abstraction", edge types abstr/sup/sim).
func (b *Brain) LoadXML(src string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.loadXML(src)
}

func (b *Brain) loadXML(src string) error {
	root := ParseXML(src)
	if root == nil || root.Name != "brain" {
		return errf("expected a <brain> root element")
	}
	fed := b.fed
	b.reset()
	b.fed = fed
	b.Name = root.A("name")
	if g := root.One("goal"); g != nil {
		b.Goal = jsTrim(g.Text)
	}
	b.Step = atoi(root.A("step"), 1)
	for _, k := range root.One("kinds").Kids("kind") {
		kk := Kind{Name: k.A("name"), Label: k.A("label"), Color: k.A("color"), Base: atof(k.A("base"), .5),
			Prefix: k.A("prefix"), Section: k.A("section")}
		kk.HalfLife = fp(atof(k.A("half-life"), 14))
		kk.Floor = atof(k.A("floor"), 0)
		kk.Floor = atof(k.A("floor"), 0)
		if kk.Prefix == "" {
			kk.Prefix = "k"
		}
		b.addKind(kk)
	}
	if em := root.One("embedding"); em != nil {
		b.Emb.Alpha = atof(em.A("alpha"), .5)
		b.Emb.Hops = atoi(em.A("hops"), 2)
		b.Emb.Lambda = atof(em.A("lambda"), .5)
		switch em.A("use") {
		case "raw":
			b.Emb.Use = "raw"
		default:
			b.Emb.Use = "enriched"
		}
	}
	for _, s := range root.One("sources").Kids("source") {
		b.sources = append(b.sources, sourceFromXML(s))
	}
	cs := root.One("concepts")
	if cs == nil {
		cs = root.One("abstractions")
	}
	for _, c := range cs.Kids("concept") {
		b.addNode(&Node{ID: c.A("id"), Kind: Concept, Section: "concepts", Text: jsTrim(c.Text), Base: atof(c.A("base"), .8), Labels: splitIDs(c.A("labels"))})
	}
	now := b.nowMs()
	secs := []string{}
	for _, sec := range root.One("memory").Kids("section") {
		secs = append(secs, sec.A("name"))
		for _, it := range sec.Kids("item") {
			kind := it.A("kind")
			if kind == "abstraction" {
				kind = Concept
			}
			if _, ok := b.kindM[kind]; !ok {
				kind = "note"
			}
			n := &Node{ID: it.A("id"), Kind: kind, Section: sec.A("name"), Text: jsTrim(it.Text), Base: atof(it.A("base"), b.kind(kind).Base),
				Pin: it.A("pinned") == "1", Sup: it.A("superseded-by"), Parent: it.A("parent"), Idx: atoi(it.A("idx"), 0)}
			if t, ok := parseISO(it.A("touched")); ok && it.A("touched") != "" {
				n.Touched = t
				n.Age = math.Max(0, round2(float64(now-t)/dayMs))
			} else {
				n.Age = atof(it.A("age"), 0)
			}
			if name, ok := it.Attr("name"); ok && name != "" {
				n.Meta = &FileMeta{Name: name, Mime: it.A("mime"), Size: int64(atof(it.A("size"), 0))}
			}
			n.Labels = splitIDs(it.A("labels"))
			n.Src, n.Hash = it.A("src"), it.A("hash")
			if d := it.A("purpose-decision"); d != "" || it.A("purpose-flag") != "" {
				n.Purpose = &PurposeMark{Score: atof(it.A("purpose-score"), 0), Decision: d, Flag: it.A("purpose-flag"), Section: it.A("purpose-section"), Reasons: it.A("purpose-reasons")}
			}
			if d := it.A("data"); d != "" {
				var f Fields
				if json.Unmarshal([]byte(d), &f) == nil {
					n.Fields = f
				}
			}
			b.addNode(n)
		}
	}
	if len(secs) > 0 {
		b.secs = uniq(secs)
	}
	for _, c := range root.One("connectors").Kids("connector") {
		n := connFromXML(c)
		n.Labels = splitIDs(c.A("labels"))
		b.addNode(n)
	}
	for _, r := range root.One("externals").Kids("ref") {
		id := r.A("id")
		src := id
		if i := strings.Index(id, ":"); i > 0 {
			src = id[:i]
		}
		b.addNode(&Node{ID: id, Kind: r.A("kind"), Section: "external", Text: jsTrim(r.Text), Base: atof(r.A("w"), .5),
			Ext: &ExtRef{Source: src, W: atof(r.A("w"), .5), Fetched: r.A("fetched")}, Labels: splitIDs(r.A("labels"))})
	}
	for _, l := range root.One("synapses").Kids("link") {
		t := l.A("type")
		if t == "" {
			t = EdgeAssoc
		}
		if m, ok := legacyEdge[t]; ok {
			t = m
		}
		b.addEdge(l.A("a"), l.A("b"), atof(l.A("w"), 0), t)
	}
	if c := root.One("context"); c != nil {
		f, ok := c.Attr("focus")
		if !ok {
			f = c.A("seeds")
		}
		known := func(id string) bool { return b.nodes[id] != nil }
		b.seeds = filterStr(splitIDs(f), known)
		if g, ok := c.Attr("goal-focus"); ok {
			b.goal = filterStr(splitIDs(g), known)
		} else {
			b.goal = append([]string(nil), b.seeds...)
		}
		b.P.Depth = atoi(c.A("depth"), 2)
		b.P.Threshold = atof(c.A("threshold"), .16)
		b.P.Hop = atof(c.A("hop"), .85)
		b.P.ShowSuperseded = c.A("show-superseded") == "1"
	}
	b.recomputeNextID()
	b.embedAll()
	b.refresh()
	return nil
}

// XML serialises the brain (lossless: LoadXML(XML()) then XML() gives the same text for the same clock).
func (b *Brain) XML() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.brainXML()
}

func (b *Brain) brainXML() string {
	var x strings.Builder
	x.WriteString(`<brain version="1" step="` + strconv.Itoa(b.Step) + `"` + xa("name", b.Name) + ">\n")
	x.WriteString("<goal>" + esc(b.Goal) + "</goal>\n")
	x.WriteString(`<embedding model="` + esc(b.Emb.Model) + `" dim="` + strconv.Itoa(b.Emb.Dim) + `" alpha="` + fmtNum(b.Emb.Alpha) +
		`" hops="` + strconv.Itoa(b.Emb.Hops) + `" use="` + b.Emb.Use + `" lambda="` + fmtNum(b.Emb.Lambda) + "\"/>\n")
	custom := []*Kind{}
	for _, k := range b.kinds {
		if !k.Builtin {
			custom = append(custom, k)
		}
	}
	if len(custom) > 0 {
		x.WriteString("<kinds>\n")
		for _, k := range custom {
			x.WriteString(`  <kind name="` + esc(k.Name) + `" label="` + esc(k.Label) + `" color="` + esc(k.Color) + `" half-life="` + fmtNum(k.hl()) +
				`" floor="` + fmtNum(k.Floor) + `" floor="` + fmtNum(k.Floor) + `" base="` + fmtNum(k.Base) + `" prefix="` + esc(k.Prefix) + `" section="` + esc(k.Section) + "\"/>\n")
		}
		x.WriteString("</kinds>\n")
	}
	ss := "0"
	if b.P.ShowSuperseded {
		ss = "1"
	}
	x.WriteString(`<context focus="` + esc(strings.Join(b.seeds, ",")) + `" goal-focus="` + esc(strings.Join(b.goal, ",")) + `" depth="` + strconv.Itoa(b.P.Depth) +
		`" threshold="` + fmtNum(b.P.Threshold) + `" hop="` + fmtNum(b.P.Hop) + `" show-superseded="` + ss + "\"/>\n")
	if len(b.sources) > 0 {
		x.WriteString("<sources>\n")
		for _, s := range b.sources {
			x.WriteString("  " + s.xml() + "\n")
		}
		x.WriteString("</sources>\n")
	}
	x.WriteString("<concepts>\n")
	for _, id := range b.concepts() {
		n := b.nodes[id]
		x.WriteString(`  <concept id="` + esc(id) + `" base="` + fmtNum(n.Base) + `"` + xa("labels", strings.Join(n.Labels, ",")) + `>` + esc(n.Text) + "</concept>\n")
	}
	x.WriteString("</concepts>\n<memory>\n")
	bySec := map[string][]*Node{}
	secs := append([]string(nil), b.secs...)
	for _, id := range b.order {
		n := b.nodes[id]
		if n.Kind == Concept || n.Ext != nil || n.Conn != nil {
			continue
		}
		if !contains(secs, n.Section) {
			secs = append(secs, n.Section)
		}
		bySec[n.Section] = append(bySec[n.Section], n)
	}
	for _, sec := range secs {
		x.WriteString(`<section name="` + esc(sec) + "\">\n")
		for _, n := range bySec[sec] {
			x.WriteString(`  <item id="` + esc(n.ID) + `" kind="` + esc(n.Kind) + `" base="` + fmtNum(n.Base) + `" age="` + fmtNum(b.ageNow(n)) +
				`" touched="` + isoMs(b.touchOf(n)) + `"`)
			if n.Pin {
				x.WriteString(` pinned="1"`)
			}
			if n.Sup != "" {
				x.WriteString(` superseded-by="` + esc(n.Sup) + `"`)
			}
			if n.Meta != nil {
				x.WriteString(` name="` + esc(n.Meta.Name) + `" mime="` + esc(n.Meta.Mime) + `" size="` + strconv.FormatInt(n.Meta.Size, 10) + `"`)
			}
			if n.Parent != "" {
				x.WriteString(` parent="` + esc(n.Parent) + `" idx="` + strconv.Itoa(n.Idx) + `"`)
			}
			if len(n.Fields) > 0 {
				d, _ := n.Fields.MarshalJSON()
				x.WriteString(` data="` + esc(string(d)) + `"`)
			}
			x.WriteString(xa("labels", strings.Join(n.Labels, ",")) + xa("src", n.Src) + xa("hash", n.Hash))
			if pm := n.Purpose; pm != nil {
				x.WriteString(` purpose-score="` + fmtNum(pm.Score) + `"` + xa("purpose-decision", pm.Decision) + xa("purpose-flag", pm.Flag) + xa("purpose-section", pm.Section) + xa("purpose-reasons", pm.Reasons))
			}
			x.WriteString(">" + esc(n.Text) + "</item>\n")
		}
		x.WriteString("</section>\n")
	}
	x.WriteString("</memory>\n")
	x.WriteString(b.connectorsXML())
	x.WriteString("<synapses>\n")
	for _, e := range b.edges {
		x.WriteString(`  <link a="` + esc(e.A) + `" b="` + esc(e.B) + `" w="` + fmtNum(e.W) + `" type="` + e.Type + "\"/>\n")
	}
	x.WriteString("</synapses>\n")
	ext := false
	for _, id := range b.order {
		n := b.nodes[id]
		if n.Ext == nil {
			continue
		}
		if !ext {
			x.WriteString("<externals>\n")
			ext = true
		}
		x.WriteString(`  <ref id="` + esc(n.ID) + `" kind="` + esc(n.Kind) + `" w="` + fmtNum(round2(n.Ext.W)) + `"` + xa("fetched", n.Ext.Fetched) + xa("labels", strings.Join(n.Labels, ",")) + ">" + esc(n.Text) + "</ref>\n")
	}
	if ext {
		x.WriteString("</externals>\n")
	}
	x.WriteString("</brain>\n")
	return x.String()
}
