// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"encoding/json"
	"math"
	"os"
	"sort"
)

// Seed is the seed.json format.
type Seed struct {
	Name         string           `json:"name"`
	Goal         string           `json:"goal"`
	Sections     []string         `json:"sections"`
	Kinds        []SeedKind       `json:"kinds"`
	Concepts     []SeedConcept    `json:"concepts"`
	ConceptLinks [][]any          `json:"conceptLinks"`
	Memories     []SeedMemory     `json:"memories"`
	Links        [][]any          `json:"links"`
	Archive      *OMap            `json:"archive"`
	Focus        []string         `json:"focus"`
	Context      *SeedContext     `json:"context"`
	Sources      []SourceConfig   `json:"sources"`
	External     [][]any          `json:"externalLinks"`
	Embedding    *EmbeddingChange `json:"embedding"`
	Connectors   []SeedConnector  `json:"connectors"`
	Extra        map[string]any   `json:"-"`
}

// SeedConnector is a connector in a seed.
type SeedConnector struct {
	ConnectorSpec
	Connect []string `json:"connect"`
}

// SeedKind is a custom kind in a seed.
type SeedKind struct {
	Name     string  `json:"name"`
	Label    string  `json:"label"`
	Color    string  `json:"color"`
	HalfLife float64 `json:"halfLife"`
	Floor    float64 `json:"floor"`
	Base     float64 `json:"base"`
	Prefix   string  `json:"prefix"`
	Section  string  `json:"section"`
}

// SeedConcept is a concept in a seed.
type SeedConcept struct {
	ID    string   `json:"id"`
	Label string   `json:"label"`
	Base  *float64 `json:"base"`
}

// SeedMemory is a memory in a seed.
type SeedMemory struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Section      string   `json:"section"`
	Text         string   `json:"text"`
	Concepts     []string `json:"concepts"`
	AgeDays      float64  `json:"ageDays"`
	Pinned       bool     `json:"pinned"`
	Base         *float64 `json:"base"`
	SupersededBy string   `json:"supersededBy"`
	Fields       Fields   `json:"fields"`
	Labels       []string `json:"labels"`
}

// SeedContext holds activation parameters.
type SeedContext struct {
	Depth          *int     `json:"depth"`
	Threshold      *float64 `json:"threshold"`
	Hop            *float64 `json:"hop"`
	ShowSuperseded bool     `json:"showSuperseded"`
}

func linkTriple(l []any) (string, string, float64, bool) {
	if len(l) < 2 {
		return "", "", 0, false
	}
	a, _ := l[0].(string)
	c, _ := l[1].(string)
	w := .6
	if len(l) > 2 {
		if x, ok := l[2].(float64); ok {
			w = x
		}
	}
	return a, c, w, a != "" && c != ""
}

func cid(id string) string {
	if len(id) > 2 && id[:2] == "A:" {
		return id
	}
	return "A:" + id
}

// LoadSeedFile reads a seed.json and builds the brain from it.
func (b *Brain) LoadSeedFile(path string) error {
	d, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var s Seed
	if err := json.Unmarshal(d, &s); err != nil {
		return errf("seed %s: %v", path, err)
	}
	return b.LoadSeed(&s)
}

// LoadSeed builds the brain from a seed, mirroring the reference buildDefault().
func (b *Brain) LoadSeed(s *Seed) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	fed := b.fed
	b.reset()
	b.fed = fed
	b.Name, b.Goal, b.Step = s.Name, s.Goal, 1
	if len(s.Sections) > 0 {
		b.secs = uniq(s.Sections)
	}
	for _, k := range s.Kinds {
		hl := k.HalfLife
		if hl <= 0 {
			hl = 14
		}
		b.addKind(Kind{Name: k.Name, Label: k.Label, Color: k.Color, HalfLife: fp(hl), Floor: clamp01(k.Floor), Base: k.Base, Prefix: k.Prefix, Section: k.Section})
		b.addSection(b.kindM[k.Name].Section)
	}
	for _, sc := range s.Sources {
		src := sc.toSource()
		if err := src.validate(); err != nil {
			return err
		}
		b.sources = append(b.sources, src)
	}
	r := mulberry32(7)
	for _, c := range s.Concepts {
		base := .8
		if c.Base != nil {
			base = *c.Base
		}
		label := c.Label
		if label == "" {
			label = c.ID
		}
		b.addNode(&Node{ID: cid(c.ID), Kind: Concept, Section: "concepts", Text: label, Base: base})
	}
	for _, l := range s.ConceptLinks {
		if a, c, w, ok := linkTriple(l); ok {
			b.addEdge(cid(a), cid(c), w, EdgeConcept)
		}
	}
	for _, m := range s.Memories {
		k, ok := b.kindM[m.Kind]
		if !ok || m.Kind == Concept {
			return errf("seed memory %s: unknown kind %q", m.ID, m.Kind)
		}
		if m.ID == "" {
			return errf("seed memory without id: %q", m.Text)
		}
		base := k.Base
		if m.Base != nil {
			base = *m.Base
		}
		sec := m.Section
		if sec == "" {
			sec = k.Section
		}
		b.addNode(&Node{ID: m.ID, Kind: m.Kind, Section: sec, Text: m.Text, Age: m.AgeDays, Pin: m.Pinned, Base: base, Sup: m.SupersededBy, Fields: m.Fields,
			Labels: uniqSorted(m.Labels)})
		b.addSection(sec)
		for i, a := range m.Concepts {
			w := .8
			if i > 0 {
				w = .6
			}
			A := cid(a)
			if b.nodes[A] == nil {
				b.addNode(&Node{ID: A, Kind: Concept, Section: "concepts", Text: a, Base: .8})
			}
			b.addEdge(m.ID, A, clamp(w+(r()-.5)*.2), EdgeConcept)
		}
	}
	// archive: older, mostly faded memories
	kinds := []string{"fact", "note", "task", "question", "fact", "summary", "note", "decision"}
	i := 200
	archived := map[string]bool{}
	if s.Archive != nil {
		for _, ak := range s.Archive.Keys {
			list, _ := s.Archive.Vals[ak].([]any)
			A := cid(ak)
			if b.nodes[A] == nil {
				b.addNode(&Node{ID: A, Kind: Concept, Section: "concepts", Text: ak, Base: .8})
			}
			for _, t := range list {
				txt, _ := t.(string)
				kind := kinds[int(math.Floor(r()*float64(len(kinds))))]
				k := b.kindM[kind]
				id := k.Prefix + itoa(i)
				i++
				for b.nodes[id] != nil {
					id = k.Prefix + itoa(i)
					i++
				}
				b.addNode(&Node{ID: id, Kind: kind, Section: k.Section, Text: txt, Age: math.Floor(25 + r()*280 + .5), Base: k.Base})
				archived[id] = true
				b.addSection(k.Section)
				b.addEdge(id, A, .55+r()*.35, EdgeConcept)
				if r() < .45 {
					nb := [][]any{}
					for _, l := range s.ConceptLinks {
						if x, y, _, ok := linkTriple(l); ok && (cid(x) == A || cid(y) == A) {
							nb = append(nb, l)
						}
					}
					if len(nb) > 0 {
						l := nb[int(math.Floor(r()*float64(len(nb))))]
						x, y, _, _ := linkTriple(l)
						o := cid(x)
						if o == A {
							o = cid(y)
						}
						b.addEdge(id, o, .3+r()*.3, EdgeConcept)
					}
				}
			}
		}
	}
	for _, l := range s.Links {
		if a, c, w, ok := linkTriple(l); ok {
			b.addEdge(a, c, w, EdgeAssoc)
		}
	}
	// random associations between memories sharing a concept
	mems := b.memories()
	for _, mid := range mems {
		abs := []string{}
		for _, e := range b.adj[mid] {
			if e.Type == EdgeConcept {
				abs = append(abs, other(e, mid))
			}
		}
		for k := 0; k < 2; k++ {
			if r() > .55 || len(abs) == 0 {
				continue
			}
			A := abs[int(math.Floor(r()*float64(len(abs))))]
			cands := []string{}
			for _, e := range b.adj[A] {
				x := other(e, A)
				if b.nodes[x].Kind != Concept && x != mid {
					cands = append(cands, x)
				}
			}
			if len(cands) > 0 {
				b.addEdge(mid, cands[int(math.Floor(r()*float64(len(cands))))], .25+r()*.35, EdgeAssoc)
			}
		}
	}
	for _, id := range b.order {
		if n := b.nodes[id]; n.Sup != "" && b.nodes[n.Sup] != nil {
			b.addEdge(id, n.Sup, .5, EdgeSupersede)
		}
	}
	for _, mid := range mems {
		m := b.nodes[mid]
		// only archive memories may be superseded at random (the numeric-id test alone would also hit seed ids ≥ 200)
		if v, ok := jsNumber(mid[1:]); !(ok && v >= 200) || !archived[mid] {
			continue
		}
		if r() >= .1 {
			continue
		}
		var A *Edge
		for _, e := range b.adj[mid] {
			if e.Type == EdgeConcept {
				A = e
				break
			}
		}
		if A == nil {
			continue
		}
		ac := other(A, mid)
		for _, e := range b.adj[ac] {
			x := b.nodes[other(e, ac)]
			if x.Kind != Concept && x.Age < m.Age && x.Sup == "" {
				m.Sup = x.ID
				b.addEdge(mid, x.ID, .5, EdgeSupersede)
				break
			}
		}
	}
	for _, l := range s.External {
		if a, c, w, ok := linkTriple(l); ok && b.nodes[a] != nil {
			if b.nodes[c] == nil && b.isExternalID(c) {
				b.addStub(c, "", "", 0)
			}
			b.addEdge(a, c, w, EdgeExternal)
		}
	}
	for _, c := range s.Connectors {
		h, err := b.addConnector(c.ConnectorSpec)
		if err != nil {
			return errf("seed connector %q: %v", c.Label, err)
		}
		for _, id := range c.Connect {
			if b.nodes[id] == nil && b.isExternalID(id) {
				b.addStub(id, "", "", 0)
			}
			if b.nodes[id] != nil {
				b.addEdge(id, h, .9, EdgeConnector)
			}
		}
	}
	b.seeds = filterStr(s.Focus, func(id string) bool { return b.nodes[id] != nil })
	b.goal = append([]string(nil), b.seeds...)
	if c := s.Context; c != nil {
		if c.Depth != nil {
			b.P.Depth = *c.Depth
		}
		if c.Threshold != nil {
			b.P.Threshold = *c.Threshold
		}
		if c.Hop != nil {
			b.P.Hop = *c.Hop
		}
		b.P.ShowSuperseded = c.ShowSuperseded
	}
	if s.Embedding != nil {
		if s.Embedding.Alpha != nil {
			b.Emb.Alpha = *s.Embedding.Alpha
		}
		if s.Embedding.Hops != nil {
			b.Emb.Hops = *s.Embedding.Hops
		}
		if s.Embedding.Lambda != nil {
			b.Emb.Lambda = *s.Embedding.Lambda
		}
		if s.Embedding.Use == "raw" {
			b.Emb.Use = "raw"
		}
	}
	b.recomputeNextID()
	b.embedAll()
	b.refresh()
	return nil
}

var _ = sort.Strings
