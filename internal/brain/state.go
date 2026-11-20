// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"fmt"
	"strings"
)

// NodeJSON is a node in State.
type NodeJSON struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`
	Section      string    `json:"section"`
	Text         string    `json:"text"`
	Base         float64   `json:"base"`
	Weight       float64   `json:"weight"`
	Age          float64   `json:"age"`
	Pinned       bool      `json:"pinned"`
	SupersededBy string    `json:"supersededBy,omitempty"`
	Parent       string    `json:"parent,omitempty"`
	Index        *int      `json:"index,omitempty"`
	File         *FileMeta `json:"file,omitempty"`
	Fields       Fields    `json:"fields,omitempty"`
	Source       string    `json:"source,omitempty"`
	External     bool      `json:"external,omitempty"`
	Activation   *float64  `json:"activation,omitempty"`
	Depth        *int      `json:"depth,omitempty"`
	Via          string    `json:"via,omitempty"`
	Route        []string  `json:"route,omitempty"`
	Labels       []string  `json:"labels,omitempty"`
}

// EdgeJSON is an edge in State.
type EdgeJSON struct {
	A    string  `json:"a"`
	B    string  `json:"b"`
	W    float64 `json:"w"`
	Type string  `json:"type"`
}

// EmbeddingJSON is the embedding block of State.
type EmbeddingJSON struct {
	Model             string  `json:"model"`
	Dim               int     `json:"dim"`
	Alpha             float64 `json:"alpha"`
	Hops              int     `json:"hops"`
	Use               string  `json:"use"`
	Lambda            float64 `json:"lambda"`
	CoherenceRaw      float64 `json:"coherenceRaw"`
	CoherenceEnriched float64 `json:"coherenceEnriched"`
}

// Stats summarises the lit slice.
type Stats struct {
	Lit            int     `json:"lit"`
	Memories       int     `json:"memories"`
	Concepts       int     `json:"concepts"`
	Tokens         int     `json:"tokens"`
	Reach          int     `json:"reach"`
	AvgWeight      float64 `json:"avgWeight"`
	Superseded     int     `json:"superseded"`
	NewLinks       int     `json:"newLinks"`
	ByDepth        []int   `json:"byDepth"`
	Connectors     int     `json:"connectors"`
	RoutesUsed     int     `json:"routesUsed"`
	ConnectorEdges int     `json:"connectorEdges"`
}

// State is the full brain state for the UI.
type State struct {
	Name       string          `json:"name"`
	Goal       string          `json:"goal"`
	Step       int             `json:"step"`
	Params     Params          `json:"params"`
	Embedding  EmbeddingJSON   `json:"embedding"`
	Kinds      []*Kind         `json:"kinds"`
	Sections   []string        `json:"sections"`
	Focus      []string        `json:"focus"`
	GoalFocus  []string        `json:"goalFocus"`
	Nodes      []NodeJSON      `json:"nodes"`
	Edges      []EdgeJSON      `json:"edges"`
	Stats      Stats           `json:"stats"`
	Status     string          `json:"status"`
	Sources    []SourceConfig  `json:"sources,omitempty"`
	Connectors []ConnectorJSON `json:"connectors"`
}

func (b *Brain) nodeJSON(n *Node) NodeJSON {
	j := NodeJSON{ID: n.ID, Kind: n.Kind, Section: n.Section, Text: n.Text, Base: n.Base, Weight: b.nodeW(n), Age: n.Age, Pinned: n.Pin,
		SupersededBy: n.Sup, Parent: n.Parent, File: n.Meta, Fields: n.Fields, Labels: n.Labels}
	if n.Parent != "" {
		j.Index = ip(n.Idx)
	}
	if n.Ext != nil {
		j.Source, j.External = n.Ext.Source, true
	}
	if a, ok := b.cur.act[n.ID]; ok {
		j.Activation = &a
		j.Depth = ip(b.cur.depth[n.ID])
		j.Via = b.cur.parent[n.ID]
		j.Route = b.cur.route[n.ID]
	}
	return j
}

func (b *Brain) stats() Stats {
	st := b.contextStats(b.cur)
	reach := b.cur.maxDepth()
	s := Stats{Lit: st.Mem, Memories: len(b.memories()), Concepts: st.Abs, Tokens: st.Tok, Reach: reach, AvgWeight: st.AvgW, ByDepth: make([]int, reach+1)}
	if len(b.cur.order) == 0 {
		s.ByDepth = []int{}
	}
	for _, id := range b.cur.litIDs(b) {
		s.ByDepth[b.cur.depth[id]]++
	}
	s.Connectors, s.ConnectorEdges = b.connStats()
	ro, _ := b.routesUsed(b.curLit())
	s.RoutesUsed = len(ro)
	for _, id := range b.order {
		if b.nodes[id].Sup != "" {
			s.Superseded++
		}
	}
	for _, e := range b.edges {
		if e.Type == EdgeNew {
			s.NewLinks++
		}
	}
	return s
}

// State returns the full state.
func (b *Brain) State() *State {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := &State{Name: b.Name, Goal: b.Goal, Step: b.Step, Params: b.P, Kinds: b.kinds, Sections: b.sectionOrder(),
		Focus: append([]string{}, b.seeds...), GoalFocus: append([]string{}, b.goal...), Nodes: []NodeJSON{}, Edges: []EdgeJSON{},
		Stats: b.stats(), Status: b.status("")}
	s.Embedding = EmbeddingJSON{Model: b.Emb.Model, Dim: b.Emb.Dim, Alpha: b.Emb.Alpha, Hops: b.Emb.Hops, Use: b.Emb.Use, Lambda: b.Emb.Lambda,
		CoherenceRaw: b.coherence(b.raw), CoherenceEnriched: b.coherence(b.enr)}
	kk := make([]*Kind, len(b.kinds))
	for i, k := range b.kinds {
		c := *k
		kk[i] = &c
	}
	s.Kinds = kk
	for _, id := range b.order {
		s.Nodes = append(s.Nodes, b.nodeJSON(b.nodes[id]))
	}
	for _, e := range b.edges {
		s.Edges = append(s.Edges, EdgeJSON{e.A, e.B, e.W, e.Type})
	}
	for _, src := range b.sources {
		s.Sources = append(s.Sources, src.Config())
	}
	s.Connectors = []ConnectorJSON{}
	for _, id := range b.connectors() {
		s.Connectors = append(s.Connectors, b.connectorJSON(b.nodes[id]))
	}
	return s
}

// ---------------------------------------------------------------- inspect

// InspectItem is the item part of Inspect.
type InspectItem struct {
	Node  NodeJSON `json:"node"`
	Decay float64  `json:"decay"`
	Chain []string `json:"chain"`
	XML   string   `json:"xml"`
}

// LinkJSON is one link of a node.
type LinkJSON struct {
	To   string  `json:"to"`
	W    float64 `json:"w"`
	Type string  `json:"type"`
}

// PathJSON is one knowledge path.
type PathJSON struct {
	Path     []string  `json:"path"`
	Weights  []float64 `json:"weights"`
	Strength float64   `json:"strength"`
}

// Discover is the discover result.
type Discover struct {
	Inner  []Suggestion `json:"inner"`
	Bridge []Suggestion `json:"bridge"`
}

// Inspect is the result of an inspection.
type Inspect struct {
	Item       *InspectItem          `json:"item,omitempty"`
	Links      []LinkJSON            `json:"links,omitempty"`
	Near       []Near                `json:"near,omitempty"`
	Test       *LinkTest             `json:"test,omitempty"`
	Suggest    []Suggestion          `json:"suggest,omitempty"`
	Paths      []PathJSON            `json:"paths,omitempty"`
	Discover   *Discover             `json:"discover,omitempty"`
	Connectors []ConnectorSuggestion `json:"connectors,omitempty"`
	Text       string                `json:"-"`
}

// Inspect inspects one node (what ⊂ item, links, near, test, suggest, paths) or runs discover.
func (b *Brain) Inspect(id string, what []string, to string) (*Inspect, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.inspect(id, what, to)
}

func (b *Brain) inspect(id string, what []string, to string) (*Inspect, error) {
	out := &Inspect{}
	if contains(what, "connectors") {
		out.Connectors = b.suggestConnectors(10)
		L := []string{"suggested connectors:"}
		for _, c := range out.Connectors {
			L = append(L, fmt.Sprintf("  ⧉ %s %q%s · %s · members %s", c.Type, c.Label, map[bool]string{true: " key " + c.Key, false: ""}[c.Key != ""], c.Evidence, strings.Join(firstN(c.Members, 8), ",")))
		}
		if len(out.Connectors) == 0 {
			L = append(L, "  —")
		}
		out.Text = strings.Join(L, "\n")
		if id == "" && !contains(what, "discover") {
			return out, nil
		}
	}
	if id == "" || contains(what, "discover") {
		in, br := b.discover()
		out.Discover = &Discover{in, br}
		fmtS := func(s Suggestion) string {
			v := strings.Join(firstN(s.Via, 3), ",")
			if v == "" {
				v = "—"
			}
			return fmt.Sprintf("  %s %s ↔ %s", f2(s.Score), s.U, b.line(s.V, "via "+v))
		}
		L := []string{"missing synapses inside the context:"}
		for _, s := range in {
			L = append(L, fmtS(s))
		}
		if len(in) == 0 {
			L = append(L, "  —")
		}
		L = append(L, "bridges context ↔ faded memory:")
		for _, s := range br {
			L = append(L, fmtS(s))
		}
		if len(br) == 0 {
			L = append(L, "  —")
		}
		if out.Text != "" {
			L = append([]string{out.Text}, L...)
		}
		out.Text = strings.Join(L, "\n")
		return out, nil
	}
	n := b.nodes[id]
	if n == nil {
		return nil, errf("no such item %s", id)
	}
	if len(what) == 0 {
		if to != "" {
			what = []string{"item", "paths"}
		} else {
			what = []string{"item", "links", "near"}
		}
	}
	L := []string{}
	if contains(what, "item") {
		decay := 1.0
		if n.Kind != Concept && !n.Pin && n.Ext == nil {
			decay = b.kind(n.Kind).decayAt(n.Age)
		}
		out.Item = &InspectItem{Node: b.nodeJSON(n), Decay: decay, XML: b.itemXML(n, b.curLit(), false), Chain: []string{}}
		L = append(L, out.Item.XML)
		hl := "∞"
		if k := b.kind(n.Kind); k.HalfLife != nil && n.Kind != Concept {
			hl = fmtNum(*k.HalfLife)
		}
		fl := ""
		if f := b.kind(n.Kind).Floor; f > 0 && n.Kind != Concept {
			fl = ", floor " + toFixed(f, 2)
		}
		s := fmt.Sprintf("weight %s = base %s × decay %s (age %s d, half-life %s d%s)", f2(b.nodeW(n)), fmtNum(n.Base), toFixed(decay, 3), fmtNum(n.Age), hl, fl)
		if n.Sup != "" {
			s += " × 0.3 (superseded)"
		}
		if n.Pin {
			s += " · pinned"
		}
		L = append(L, s)
		if b.cur.has(id) {
			out.Item.Chain = b.chainTo(id)
			L = append(L, "path from focus: "+strings.Join(out.Item.Chain, " → "))
		}
	}
	if contains(what, "links") {
		out.Links = []LinkJSON{}
		es := append([]*Edge(nil), b.adj[id]...)
		stableSortDesc(es, func(e *Edge) float64 { return e.W })
		L = append(L, "\nlinks:")
		for _, e := range es {
			out.Links = append(out.Links, LinkJSON{other(e, id), e.W, e.Type})
			L = append(L, fmt.Sprintf("  %s%s %s", edgeMark(e.Type), f2(e.W), b.line(other(e, id), "")))
		}
	}
	if contains(what, "near") {
		out.Near = b.nearest(id, 6)
		L = append(L, "\nsemantic neighbours (enriched vectors):")
		for _, x := range out.Near {
			lk := ""
			if x.Linked {
				lk = " (linked)"
			}
			L = append(L, fmt.Sprintf("  cos %s%s %s", f2(x.Cos), lk, b.line(x.ID, "")))
		}
	}
	if contains(what, "test") {
		t := b.testLinks(id)
		out.Test = &t
		L = append(L, fmt.Sprintf("\ntest: propagation from %s alone lights %d nodes:", id, len(t.Reached)))
		for _, x := range firstN(t.Reached, 8) {
			L = append(L, fmt.Sprintf("  d%d %s %s", x.Depth, f2(x.Act), b.line(x.ID, "")))
		}
		L = append(L, "synapse ablation (Δ lit / Δ activation sum; ! = the neighbour drops out of the context):")
		for _, x := range t.Ablation {
			c, sg := " ", ""
			if x.Critical {
				c = "!"
			}
			if x.DLit >= 0 {
				sg = "+"
			}
			L = append(L, fmt.Sprintf("  %s %s%d / %s  %s w%s → %s", c, sg, x.DLit, f2(x.DSum), x.Type, f2(x.W), b.line(x.To, "")))
		}
	}
	if contains(what, "suggest") {
		out.Suggest = b.suggestFor(id, nil, 6)
		L = append(L, "\nproposed new synapses (Adamic-Adar + words + cos):")
		for _, s := range out.Suggest {
			v := strings.Join(firstN(s.Via, 3), ",")
			if v == "" {
				v = "—"
			}
			extra := "via " + v
			if s.Cos > .2 {
				extra += " cos " + f2(s.Cos)
			}
			L = append(L, fmt.Sprintf("  %s → %s", f2(s.Score), b.line(s.V, extra)))
		}
	}
	if contains(what, "paths") {
		if to == "" || b.nodes[to] == nil {
			return nil, errf("paths needs a valid \"to\"")
		}
		ps := b.kPaths(id, to, 3)
		out.Paths = []PathJSON{}
		h := fmt.Sprintf("\nknowledge paths %s → %s:", id, to)
		if len(ps) == 0 {
			h += " no connection"
		}
		L = append(L, h)
		for i, p := range ps {
			pj := PathJSON{Path: p.Path, Strength: p.Strength, Weights: []float64{}}
			parts := []string{}
			for j, x := range p.Path {
				if j == 0 {
					parts = append(parts, x)
				} else {
					pj.Weights = append(pj.Weights, p.Edges[j-1].W)
					parts = append(parts, fmt.Sprintf("—%s→ %s", f2(p.Edges[j-1].W), x))
				}
			}
			out.Paths = append(out.Paths, pj)
			beyond := ""
			if len(p.Path)-1 > b.P.Depth {
				beyond = " (beyond the context depth)"
			}
			L = append(L, fmt.Sprintf("  #%d strength %s · %d hops%s: %s", i+1, toFixed(p.Strength, 3), len(p.Path)-1, beyond, strings.Join(parts, " ")))
		}
	}
	out.Text = strings.Join(L, "\n")
	return out, nil
}

func firstN[T any](xs []T, n int) []T {
	if len(xs) > n {
		return xs[:n]
	}
	return xs
}

func edgeMark(t string) string {
	switch t {
	case EdgeConcept:
		return "◆"
	case EdgeSupersede:
		return "⇄"
	case EdgeSimilar:
		return "≈"
	case EdgePart:
		return "⊂"
	case EdgeExternal:
		return "⇢"
	}
	return "~"
}

// Line renders one node as a single line for tool output.
func (b *Brain) Line(id, right string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.line(id, right)
}

func (b *Brain) line(id, right string) string {
	n := b.nodes[id]
	if n == nil {
		return id + " (removed)"
	}
	k := n.Kind
	if k == Concept {
		k = "◆"
	}
	s := fmt.Sprintf("%s [%s] w=%s", id, k, f2(b.nodeW(n)))
	if a, ok := b.cur.act[id]; ok {
		s += fmt.Sprintf(" act=%s d%d", f2(a), b.cur.depth[id])
	} else {
		s += " (faded)"
	}
	if n.Sup != "" {
		s += " ⇄" + n.Sup
	}
	if right != "" {
		s += " · " + right
	}
	return s + " — " + short(collapseSpace(n.Text), 110)
}

// ---------------------------------------------------------------- layout & vectors

// Layout is the result of the layout endpoint.
type Layout struct {
	Positions map[string][3]float64 `json:"positions"`
	TreeEdges [][2]string           `json:"treeEdges,omitempty"`
	Root      string                `json:"root,omitempty"`
}

// Layout computes positions for the "embed" (PCA) or "tree" (spanning tree) views.
func (b *Brain) Layout(view, root string) (*Layout, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch view {
	case "embed", "vectors", "":
		return &Layout{Positions: b.pcaLayout()}, nil
	case "tree":
		r, pos, edges := b.treeLayout(root)
		if edges == nil {
			edges = [][2]string{}
		}
		return &Layout{Positions: pos, TreeEdges: edges, Root: r}, nil
	}
	return nil, errf("view must be embed or tree")
}

// VectorInfo is the result of the vector endpoint.
type VectorInfo struct {
	Raw      []float32 `json:"raw"`
	Enriched []float32 `json:"enriched"`
	Dim      int       `json:"dim"`
	Drift    float64   `json:"drift"`
}

// Vector returns the first 64 dimensions of a node's raw and enriched vectors.
func (b *Brain) Vector(id string, full bool) (*VectorInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, e := b.raw[id], b.enr[id]
	if r == nil {
		return nil, errf("no vector for %s", id)
	}
	n := 64
	if full {
		n = len(r)
	}
	return &VectorInfo{Raw: append([]float32(nil), r[:n]...), Enriched: append([]float32(nil), e[:n]...), Dim: len(r), Drift: 1 - vcos(r, e)}, nil
}

// Has reports whether a node exists.
func (b *Brain) Has(id string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.nodes[id] != nil
}

// Counts returns node and edge counts.
func (b *Brain) Counts() (nodes, edges int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.order), len(b.edges)
}

// Focus returns the current focus.
func (b *Brain) Focus() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.seeds...)
}

// CurrentStep returns the step counter.
func (b *Brain) CurrentStep() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Step
}

// GoalText returns the goal.
func (b *Brain) GoalText() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Goal
}
