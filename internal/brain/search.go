// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"encoding/json"
	"math"
	"strings"
)

// SearchRow is one search hit.
type SearchRow struct {
	ID       string   `json:"id"`
	Score    float64  `json:"score"`
	Raw      *float64 `json:"raw,omitempty"`
	Enriched *float64 `json:"enriched,omitempty"`
	Lift     *int     `json:"lift,omitempty"`
	Vector   *float64 `json:"vector,omitempty"`
	Graph    *float64 `json:"graph,omitempty"`
	Depth    *int     `json:"depth,omitempty"`
	Via      string   `json:"via,omitempty"`
	Parent   *string  `json:"parent,omitempty"`
	Source   string   `json:"source,omitempty"`
	root     bool     // tree root: parent is rendered as null
}

// MarshalJSON renders tree roots with "parent": null.
func (r SearchRow) MarshalJSON() ([]byte, error) {
	type alias SearchRow
	d, err := json.Marshal(alias(r))
	if err != nil || !r.root {
		return d, err
	}
	return append(d[:len(d)-1], []byte(`,"parent":null}`)...), nil
}

// SearchResult is the result of Search.
type SearchResult struct {
	Mode   string      `json:"mode"`
	Lambda *float64    `json:"lambda,omitempty"`
	Rows   []SearchRow `json:"rows"`
	Via    []SearchRow `json:"via,omitempty"`
	Roots  []string    `json:"roots,omitempty"`
	Warn   []string    `json:"warnings,omitempty"`
}

func ip(i int) *int { return &i }

// Search runs one of the four search modes over the whole memory.
func (b *Brain) Search(q, mode string, k int) (*SearchResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.search(q, mode, k)
}

func (b *Brain) searchParams() Params {
	P := b.P
	if P.Depth < 2 {
		P.Depth = 2
	}
	return P
}

type vecScorer struct {
	b      *Brain
	qv     []float32
	ranked []string
	sc     map[string]float64
}

func (b *Brain) scorer(q string) *vecScorer {
	s := &vecScorer{b: b, qv: b.embed(b.expandQuery(q)), sc: map[string]float64{}}
	for _, id := range b.rawOr {
		if isQuarantined(b.nodes[id]) {
			continue
		}
		v := vcos(s.qv, b.vecOf(id))
		if b.nodes[id].Sup != "" {
			v *= .5
		}
		s.sc[id] = v
	}
	s.ranked = filterStr(b.rawOr, func(id string) bool { return !isQuarantined(b.nodes[id]) })
	stableSortDesc(s.ranked, func(id string) float64 { return s.sc[id] })
	return s
}

// hybrid computes the hybrid score for every node (reference formula generalised to all ids).
type hybridRes struct {
	seeds  []string
	top    float64
	r      *actResult
	scores map[string]float64
	vec    map[string]float64
	graph  map[string]float64
	cand   []string
	s      *vecScorer
}

func (b *Brain) hybrid(q string) *hybridRes {
	s := b.scorer(q)
	h := &hybridRes{s: s, scores: map[string]float64{}, vec: map[string]float64{}, graph: map[string]float64{}}
	if len(s.ranked) == 0 {
		h.r = newAct()
		return h
	}
	h.top = math.Max(1e-6, s.sc[s.ranked[0]])
	for i, id := range s.ranked {
		if i >= 4 {
			break
		}
		if s.sc[id] >= h.top*.5 {
			h.seeds = append(h.seeds, id)
		}
	}
	h.r = b.activate(h.seeds, b.searchParams(), nil)
	L := b.Emb.Lambda
	for _, id := range s.ranked {
		v := math.Max(0, s.sc[id])
		g := h.r.act[id]
		if contains(h.seeds, id) {
			g = v / h.top
		}
		h.vec[id], h.graph[id] = v, g
		h.scores[id] = L*v + (1-L)*g
	}
	top20 := s.ranked
	if len(top20) > 20 {
		top20 = top20[:20]
	}
	h.cand = uniq(append(append(append([]string(nil), h.seeds...), h.r.litIDs(b)...), top20...))
	return h
}

func (b *Brain) search(q, mode string, k int) (*SearchResult, error) {
	q = jsTrim(q)
	if q == "" {
		return nil, errf("empty query")
	}
	if k <= 0 {
		k = 8
	}
	if mode == "" {
		mode = "hybrid"
	}
	cut := func(n int) int {
		if n > k {
			return k
		}
		return n
	}
	switch mode {
	case "graph":
		qs := stems(b.expandQuery(q))
		type sc struct {
			id string
			s  float64
		}
		direct := []sc{}
		for _, id := range b.order {
			n := b.nodes[id]
			if n.Conn != nil || isQuarantined(n) {
				continue
			}
			ns := stems(n.Text)
			c := 0
			for x := range qs {
				if ns[x] {
					c++
				}
			}
			if c == 0 {
				continue
			}
			s := float64(c) / float64(len(qs)) * (.4 + .6*b.nodeW(n))
			if n.Sup != "" {
				s *= .3
			}
			if s > 0 {
				direct = append(direct, sc{id, s})
			}
		}
		stableSortDesc(direct, func(x sc) float64 { return x.s })
		direct = direct[:cut(len(direct))]
		seeds := []string{}
		dset := map[string]bool{}
		res := &SearchResult{Mode: mode, Rows: []SearchRow{}, Via: []SearchRow{}}
		for i, x := range direct {
			if i < 4 {
				seeds = append(seeds, x.id)
			}
			dset[x.id] = true
			res.Rows = append(res.Rows, SearchRow{ID: x.id, Score: x.s})
		}
		r := b.activate(seeds, b.searchParams(), nil)
		via := filterStr(r.litIDs(b), func(id string) bool { return !dset[id] })
		stableSortDesc(via, func(id string) float64 { return r.act[id] })
		for _, id := range via[:cut(len(via))] {
			res.Via = append(res.Via, SearchRow{ID: id, Score: r.act[id], Depth: ip(r.depth[id]), Via: r.parent[id]})
		}
		return res, nil
	case "vector":
		s := b.scorer(q)
		byRaw := append([]string(nil), b.rawOr...)
		rawSc := map[string]float64{}
		for _, id := range byRaw {
			rawSc[id] = vcos(s.qv, b.raw[id])
		}
		stableSortDesc(byRaw, func(id string) float64 { return rawSc[id] })
		rankR := map[string]int{}
		for i, id := range byRaw {
			rankR[id] = i
		}
		res := &SearchResult{Mode: mode, Rows: []SearchRow{}}
		for i, id := range s.ranked[:cut(len(s.ranked))] {
			raw, enr := rawSc[id], vcos(s.qv, b.enr[id])
			res.Rows = append(res.Rows, SearchRow{ID: id, Score: s.sc[id], Raw: &raw, Enriched: &enr, Lift: ip(rankR[id] - i)})
		}
		return res, nil
	case "hybrid":
		h := b.hybrid(q)
		L := b.Emb.Lambda
		rows := []SearchRow{}
		for _, id := range h.cand {
			v, g := h.vec[id], h.graph[id]
			row := SearchRow{ID: id, Score: h.scores[id], Vector: &v, Graph: &g, Via: h.r.parent[id]}
			if h.r.has(id) {
				row.Depth = ip(h.r.depth[id])
			}
			rows = append(rows, row)
		}
		stableSortDesc(rows, func(r SearchRow) float64 { return r.Score })
		return &SearchResult{Mode: mode, Lambda: &L, Rows: rows[:cut(len(rows))]}, nil
	case "tree":
		s := b.scorer(q)
		start := s.ranked
		if len(start) > 2 {
			start = start[:2]
		}
		type tn struct {
			p string
			s float64
			d int
		}
		Tk := append([]string(nil), start...)
		T := map[string]tn{}
		for _, id := range start {
			T[id] = tn{"", math.Max(0, s.sc[id]), 0}
		}
		frontier := append([]string(nil), start...)
		maxD := b.searchParams().Depth + 1
		type cand struct {
			v, u string
			s    float64
		}
		for d := 1; d <= maxD && len(frontier) > 0; d++ {
			ck := []string{}
			cands := map[string]cand{}
			for _, u := range frontier {
				ks := []cand{}
				for _, e := range b.adj[u] {
					if e.Type == EdgeSupersede {
						continue
					}
					v := other(e, u)
					if _, ok := T[v]; ok {
						continue
					}
					if nv := b.nodes[v]; nv.Conn != nil {
						// expand through the connector (one junction, weight = w · gain · w')
						for _, e2 := range b.adj[v] {
							v2 := other(e2, v)
							if _, ok := T[v2]; ok || v2 == u || isConn(b.nodes[v2]) || e2.Type == EdgeSupersede {
								continue
							}
							ks = append(ks, cand{v2, u, .55*math.Max(0, s.sc[v2]) + .45*T[u].s*e.W*nv.Conn.Gain*e2.W})
						}
						continue
					}
					ks = append(ks, cand{v, u, .55*math.Max(0, s.sc[v]) + .45*T[u].s*e.W})
				}
				stableSortDesc(ks, func(c cand) float64 { return c.s })
				if len(ks) > 3 {
					ks = ks[:3]
				}
				for _, x := range ks {
					c, ok := cands[x.v]
					if !ok || c.s < x.s {
						if !ok {
							ck = append(ck, x.v)
						}
						cands[x.v] = x
					}
				}
			}
			pick := []cand{}
			for _, v := range ck {
				pick = append(pick, cands[v])
			}
			stableSortDesc(pick, func(c cand) float64 { return c.s })
			if len(pick) > 6 {
				pick = pick[:6]
			}
			frontier = nil
			for _, x := range pick {
				if _, ok := T[x.v]; !ok {
					Tk = append(Tk, x.v)
				}
				T[x.v] = tn{x.u, x.s, d}
				frontier = append(frontier, x.v)
			}
		}
		res := &SearchResult{Mode: mode, Roots: start, Rows: []SearchRow{}}
		for _, id := range Tk {
			x := T[id]
			row := SearchRow{ID: id, Score: x.s, Depth: ip(x.d), root: x.p == ""}
			if x.p != "" {
				p := x.p
				row.Parent = &p
			}
			res.Rows = append(res.Rows, row)
		}
		return res, nil
	}
	return nil, errf("mode must be one of: hybrid | vector | tree | graph")
}

// SearchFocus runs a search and, like the reference brain_search focus=true, lights the top results.
func (b *Brain) SearchFocus(r *SearchResult) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if r.Mode == "tree" {
		b.seeds = append([]string(nil), r.Roots...)
	} else {
		b.seeds = nil
		for i, x := range r.Rows {
			if i >= 4 {
				break
			}
			if b.nodes[x.ID] != nil {
				b.seeds = append(b.seeds, x.ID)
			}
		}
	}
	b.refresh()
}

func lowerDeacc(s string) string { return deacc(strings.ToLower(s)) }
