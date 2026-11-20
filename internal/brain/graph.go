// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"math"
	"sort"
	"strings"
)

// actResult is an insertion-ordered activation map (order matters for ties, as with JS Map).
type actResult struct {
	order  []string
	act    map[string]float64
	depth  map[string]int
	parent map[string]string // "" = seed (null)
	route  map[string][]string
}

func newAct() *actResult {
	return &actResult{act: map[string]float64{}, depth: map[string]int{}, parent: map[string]string{}, route: map[string][]string{}}
}

// lit returns the lit knowledge ids (connectors excluded) in activation order.
func (r *actResult) litIDs(b *Brain) []string { return filterStr(r.order, b.knowledge) }

func (r *actResult) has(id string) bool { _, ok := r.act[id]; return ok }

func (r *actResult) sum() float64 {
	s := 0.0
	for _, id := range r.order {
		s += r.act[id]
	}
	return s
}

func (r *actResult) maxDepth() int {
	m := 0
	for _, id := range r.order {
		if r.depth[id] > m {
			m = r.depth[id]
		}
	}
	return m
}

// activate is spreading activation from the seeds, level by level up to P.Depth:
//
//	a(v) = act(u) · w(u,v) · hop · (0.35 + 0.65 · weight(v)); superseded v (hidden): ×0.15 and supersede edges skipped
//
// v is kept when a ≥ threshold and a beats its current activation. Connectors: entering one gives
// a = act(u) · w · gain (no hop); a transparent connector is expanded within the same level, an opaque one consumes
// a level. Nodes reached through connectors record via = the knowledge node before them and route = the connectors.
func (b *Brain) activate(seeds []string, P Params, skip *Edge) *actResult {
	r := newAct()
	for _, id := range seeds {
		if b.nodes[id] == nil || isQuarantined(b.nodes[id]) {
			continue
		}
		if !r.has(id) {
			r.order = append(r.order, id)
		}
		r.act[id], r.depth[id], r.parent[id] = 1, 0, ""
	}
	frontier := append([]string(nil), r.order...)
	for d := 1; d <= P.Depth && len(frontier) > 0; d++ {
		next := []string{}
		inNext := map[string]bool{}
		push := func(v string) {
			if !inNext[v] {
				inNext[v] = true
				next = append(next, v)
			}
		}
		var expand func(u string, au float64, viaK string, route []string, guard map[string]bool)
		expand = func(u string, au float64, viaK string, route []string, guard map[string]bool) {
			allowed := b.fanout(u)
			for _, e := range b.adj[u] {
				if e == skip {
					continue
				}
				if e.Type == EdgeSupersede && !P.ShowSuperseded {
					continue
				}
				v := other(e, u)
				if allowed != nil && !allowed[v] {
					continue
				}
				nv := b.nodes[v]
				if isQuarantined(nv) {
					continue
				}
				if nv.Conn != nil {
					if guard[v] {
						continue
					}
					a := au * e.W * nv.Conn.Gain
					if a < P.Threshold {
						continue
					}
					cur, ok := r.act[v]
					if ok && a <= cur+1e-9 {
						continue
					}
					if !ok {
						r.order = append(r.order, v)
						r.depth[v] = d
						if nv.Conn.Transparent {
							r.depth[v] = d - 1
						}
					}
					r.act[v], r.parent[v] = a, viaK
					r.setRoute(v, route)
					if nv.Conn.Transparent {
						g := map[string]bool{v: true}
						for k := range guard {
							g[k] = true
						}
						expand(v, a, viaK, append(append([]string(nil), route...), v), g)
					} else {
						push(v)
					}
					continue
				}
				a := au * e.W * P.Hop * (0.35 + 0.65*b.nodeW(nv))
				if nv.Sup != "" && !P.ShowSuperseded {
					a *= 0.15
				}
				if a < P.Threshold {
					continue
				}
				cur, ok := r.act[v]
				if !ok || a > cur+1e-9 {
					if !ok {
						r.depth[v] = d
						r.order = append(r.order, v)
					}
					r.act[v] = a
					r.parent[v] = viaK
					r.setRoute(v, route)
					push(v)
				}
			}
		}
		for _, u := range frontier {
			if isConn(b.nodes[u]) {
				expand(u, r.act[u], r.parent[u], append(append([]string(nil), r.route[u]...), u), map[string]bool{u: true})
			} else {
				expand(u, r.act[u], u, nil, nil)
			}
		}
		frontier = next
	}
	return r
}

func (r *actResult) setRoute(v string, route []string) {
	if len(route) == 0 {
		delete(r.route, v)
		return
	}
	r.route[v] = append([]string(nil), route...)
}

// fanout returns the allowed neighbours of a connector with more than MaxFanout neighbours (nil = all).
func (b *Brain) fanout(u string) map[string]bool {
	n := b.nodes[u]
	if !isConn(n) || len(b.adj[u]) <= MaxFanout {
		return nil
	}
	es := append([]*Edge(nil), b.adj[u]...)
	stableSortDesc(es, func(e *Edge) float64 { return e.W * b.nodeW(b.nodes[other(e, u)]) })
	m := map[string]bool{}
	for _, e := range es[:MaxFanout] {
		m[other(e, u)] = true
	}
	return m
}

// knowledge reports whether id is a knowledge node (not a connector).
func (b *Brain) knowledge(id string) bool { return !isConn(b.nodes[id]) }

func (b *Brain) refresh() *actResult {
	b.cur = b.activate(b.seeds, b.P, nil)
	return b.cur
}

// ctxStats mirrors contextStats().
type ctxStats struct {
	Mem, Abs, Tok int
	Sum, AvgW     float64
}

func (b *Brain) contextStats(r *actResult) ctxStats {
	var st ctxStats
	sw := 0.0
	for _, id := range r.order {
		n := b.nodes[id]
		if n.Conn != nil {
			continue
		}
		if n.Kind == Concept {
			st.Abs++
		} else {
			st.Mem++
			st.Tok += tokensOf(n)
			sw += b.nodeW(n)
		}
	}
	for _, id := range r.order {
		if isConn(b.nodes[id]) {
			st.Sum -= r.act[id]
		}
	}
	st.Sum += r.sum()
	if st.Mem > 0 {
		st.AvgW = sw / float64(st.Mem)
	}
	return st
}

// ---------------------------------------------------------------- paths

type kpath struct {
	Path     []string
	Edges    []*Edge
	Strength float64
}

func (b *Brain) bestPath(from, to string, banned *Edge) *kpath {
	distK := []string{from}
	dist := map[string]float64{from: 0}
	type pe struct {
		p string
		e *Edge
	}
	prev := map[string]pe{}
	done := map[string]bool{}
	for {
		u, best := "", math.Inf(1)
		for _, k := range distK {
			if v := dist[k]; !done[k] && v < best {
				best, u = v, k
			}
		}
		if u == "" {
			return nil
		}
		if u == to {
			break
		}
		done[u] = true
		for _, e := range b.adj[u] {
			if e == banned {
				continue
			}
			v := other(e, u)
			nv := b.nodes[v]
			sup := 1.0
			if nv.Sup != "" {
				sup = .3
			}
			c := best - math.Log(e.W*(0.35+0.65*b.nodeW(nv))*sup)
			old, ok := dist[v]
			if !ok {
				old = math.Inf(1)
			}
			if c < old {
				if !ok {
					distK = append(distK, v)
				}
				dist[v] = c
				prev[v] = pe{u, e}
			}
		}
	}
	p := &kpath{Path: []string{to}}
	for c := to; c != from; {
		x := prev[c]
		p.Edges = append([]*Edge{x.e}, p.Edges...)
		p.Path = append([]string{x.p}, p.Path...)
		c = x.p
	}
	p.Strength = math.Exp(-dist[to])
	return p
}

func (b *Brain) kPaths(from, to string, k int) []*kpath {
	first := b.bestPath(from, to, nil)
	if first == nil {
		return nil
	}
	out := []*kpath{first}
	seen := map[string]bool{strings.Join(first.Path, ">"): true}
	for _, e := range first.Edges {
		p := b.bestPath(from, to, e)
		if p != nil && !seen[strings.Join(p.Path, ">")] {
			seen[strings.Join(p.Path, ">")] = true
			out = append(out, p)
		}
	}
	stableSortDesc(out, func(p *kpath) float64 { return p.Strength })
	if len(out) > k {
		out = out[:k]
	}
	return out
}

// ---------------------------------------------------------------- suggestions

// Suggestion is a predicted missing synapse.
type Suggestion struct {
	U           string   `json:"u"`
	V           string   `json:"v"`
	Score       float64  `json:"score"`
	Via         []string `json:"via"`
	CommonStems int      `json:"commonStems"`
	Cos         float64  `json:"cos"`
}

func (b *Brain) linkScore(u, v string) Suggestion {
	nu := map[string]float64{}
	for _, e := range b.adj[u] {
		nu[other(e, u)] = e.W
	}
	s := 0.0
	via := []string{}
	for _, e := range b.adj[v] {
		z := other(e, v)
		if w, ok := nu[z]; ok {
			s += w * e.W / math.Log(2+float64(len(b.adj[z])))
			via = append(via, z)
		}
	}
	su, sv := stems(b.nodes[u].Text), stems(b.nodes[v].Text)
	common := 0
	for x := range su {
		if sv[x] {
			common++
		}
	}
	s += float64(common) * .12
	cs := vcos(b.vecOf(u), b.vecOf(v))
	if cs > .2 {
		s += (cs - .2) * 1.2
	}
	return Suggestion{U: u, V: v, Score: s, Via: via, CommonStems: common, Cos: cs}
}

func (b *Brain) suggestFor(u string, pool []string, n int) []Suggestion {
	if pool == nil {
		pool = b.order
	}
	pu := b.nodes[u].Parent
	out := []Suggestion{}
	for _, v := range pool {
		nv := b.nodes[v]
		if v == u || nv.Conn != nil || b.edgeBetween(u, v) != nil || nv.Sup != "" || (pu != "" && nv.Parent == pu) || nv.Parent == u || pu == v {
			continue
		}
		if s := b.linkScore(u, v); s.Score > .05 {
			out = append(out, s)
		}
	}
	stableSortDesc(out, func(s Suggestion) float64 { return s.Score })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func (b *Brain) discover() (inner, bridge []Suggestion) {
	pool := filterStr(b.order, func(id string) bool { n := b.nodes[id]; return n.Kind != Concept && n.Sup == "" && n.Conn == nil })
	seen := map[string]bool{}
	inner, bridge = []Suggestion{}, []Suggestion{}
	for _, u := range b.cur.order {
		if b.nodes[u].Kind == Concept || isConn(b.nodes[u]) {
			continue
		}
		for _, s := range b.suggestFor(u, pool, 8) {
			k := []string{s.U, s.V}
			sort.Strings(k)
			key := k[0] + "|" + k[1]
			if seen[key] {
				continue
			}
			seen[key] = true
			if b.cur.has(s.V) {
				inner = append(inner, s)
			} else {
				bridge = append(bridge, s)
			}
		}
	}
	stableSortDesc(inner, func(s Suggestion) float64 { return s.Score })
	stableSortDesc(bridge, func(s Suggestion) float64 { return s.Score })
	if len(inner) > 8 {
		inner = inner[:8]
	}
	if len(bridge) > 8 {
		bridge = bridge[:8]
	}
	return
}

// ---------------------------------------------------------------- link test

// Reached is a node lit from a single source.
type Reached struct {
	ID    string  `json:"id"`
	Act   float64 `json:"act"`
	Depth int     `json:"depth"`
}

// Ablation reports the effect of removing one edge.
type Ablation struct {
	To       string  `json:"to"`
	W        float64 `json:"w"`
	Type     string  `json:"type"`
	DLit     int     `json:"dLit"`
	DSum     float64 `json:"dSum"`
	Critical bool    `json:"critical"`
}

// LinkTest is the result of testLinks.
type LinkTest struct {
	Reached  []Reached  `json:"reached"`
	Ablation []Ablation `json:"ablation"`
}

func (b *Brain) testLinks(id string) LinkTest {
	solo := b.activate([]string{id}, b.P, nil)
	base, baseSum := len(b.cur.order), b.cur.sum()
	t := LinkTest{Reached: []Reached{}, Ablation: []Ablation{}}
	for _, v := range solo.order {
		if v != id {
			t.Reached = append(t.Reached, Reached{v, solo.act[v], solo.depth[v]})
		}
	}
	stableSortDesc(t.Reached, func(r Reached) float64 { return r.Act })
	for _, e := range b.adj[id] {
		r := b.activate(b.seeds, b.P, e)
		v := other(e, id)
		t.Ablation = append(t.Ablation, Ablation{To: v, W: e.W, Type: e.Type, DLit: len(r.order) - base, DSum: r.sum() - baseSum,
			Critical: b.cur.has(v) && !r.has(v)})
	}
	sort.SliceStable(t.Ablation, func(i, j int) bool { return t.Ablation[i].DSum < t.Ablation[j].DSum })
	return t
}

func (b *Brain) chainTo(id string) []string {
	ch := []string{}
	c, ok := id, true
	for ok && len(ch) < 12 {
		ch = append([]string{c}, ch...)
		p, has := b.cur.parent[c]
		if !has || p == "" {
			break
		}
		c = p
	}
	return ch
}

// Near is a semantic neighbour.
type Near struct {
	ID     string  `json:"id"`
	Cos    float64 `json:"cos"`
	Linked bool    `json:"linked"`
}

func (b *Brain) nearest(id string, k int) []Near {
	out := []Near{}
	for _, x := range b.order {
		if x == id || b.raw[x] == nil || isConn(b.nodes[x]) {
			continue
		}
		out = append(out, Near{x, vcos(b.vecOf(id), b.vecOf(x)), b.edgeBetween(id, x) != nil})
	}
	stableSortDesc(out, func(n Near) float64 { return n.Cos })
	if len(out) > k {
		out = out[:k]
	}
	return out
}

// ---------------------------------------------------------------- spanning tree layout

type spanT struct {
	root   string
	parent map[string]string
	order  []string
	depth  map[string]int
	edges  [][2]string
}

func (b *Brain) spanTree(root string) *spanT {
	t := &spanT{root: root, parent: map[string]string{root: ""}, depth: map[string]int{root: 0}, order: []string{root}}
	type cand struct {
		s float64
		u string
	}
	bestK := []string{}
	best := map[string]cand{}
	push := func(u string) {
		for _, e := range b.adj[u] {
			if e.Type == EdgeSupersede {
				continue
			}
			v := other(e, u)
			if _, ok := t.parent[v]; ok {
				continue
			}
			s := e.W * (.4 + .6*b.nodeW(b.nodes[v]))
			c, ok := best[v]
			if !ok || s > c.s {
				if !ok {
					bestK = append(bestK, v)
				}
				best[v] = cand{s, u}
			}
		}
	}
	push(root)
	for len(bestK) > 0 {
		bv, bs, bi := "", -1.0, -1
		for i, v := range bestK {
			if c := best[v]; c.s > bs {
				bs, bv, bi = c.s, v, i
			}
		}
		u := best[bv].u
		delete(best, bv)
		bestK = append(bestK[:bi:bi], bestK[bi+1:]...)
		t.parent[bv] = u
		t.order = append(t.order, bv)
		t.depth[bv] = t.depth[u] + 1
		t.edges = append(t.edges, [2]string{u, bv})
		push(bv)
	}
	return t
}

func (b *Brain) treeLayout(root string) (string, map[string][3]float64, [][2]string) {
	if b.nodes[root] == nil {
		root = ""
		for _, id := range b.seeds {
			if b.nodes[id] != nil {
				root = id
				break
			}
		}
		if root == "" && len(b.concepts()) > 0 {
			root = b.concepts()[0]
		}
		if root == "" && len(b.order) > 0 {
			root = b.order[0]
		}
	}
	out := map[string][3]float64{}
	if root == "" {
		return root, out, nil
	}
	T := b.spanTree(root)
	kids := map[string][]string{}
	for _, v := range T.order {
		if p := T.parent[v]; p != "" {
			kids[p] = append(kids[p], v)
		}
	}
	leaves := map[string]int{}
	var cnt func(u string) int
	cnt = func(u string) int {
		c := 0
		for _, v := range kids[u] {
			c += cnt(v)
		}
		if len(kids[u]) == 0 {
			c = 1
		}
		leaves[u] = c
		return c
	}
	cnt(root)
	maxD := 1
	for _, d := range T.depth {
		if d > maxD {
			maxD = d
		}
	}
	var place func(u string, a0, a1 float64)
	place = func(u string, a0, a1 float64) {
		d := float64(T.depth[u])
		a, r := (a0+a1)/2, d*26
		out[u] = [3]float64{math.Cos(a) * r, 90 - d*(180/float64(maxD)), math.Sin(a) * r}
		s := a0
		ks := append([]string(nil), kids[u]...)
		sort.SliceStable(ks, func(i, j int) bool { return leaves[ks[i]] > leaves[ks[j]] })
		for _, v := range ks {
			span := (a1 - a0) * float64(leaves[v]) / float64(leaves[u])
			place(v, s, s+span)
			s += span
		}
	}
	place(root, 0, math.Pi*2)
	rest := filterStr(b.order, func(id string) bool { _, ok := out[id]; return !ok })
	for i, id := range rest {
		a := float64(i) / float64(len(rest)) * math.Pi * 2
		out[id] = [3]float64{math.Cos(a) * 170, -115, math.Sin(a) * 170}
	}
	return root, out, T.edges
}
