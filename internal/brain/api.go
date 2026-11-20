// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Gabski/brainDB/internal/patterns"
)

// ConceptRef is the value of the "concept" pseudo-column: compare it with = to test membership.
type ConceptRef struct {
	b  *Brain
	id string
}

// Matches reports whether the node is linked to a concept with that label or id.
func (c ConceptRef) Matches(label string) bool { return c.b.hasConcept(c.id, label) }

// AddSource mounts (or updates) an external source.
func (b *Brain) AddSource(c SourceConfig) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := c.toSource()
	if err := s.validate(); err != nil {
		return err
	}
	for i, x := range b.sources {
		if x.Name == s.Name {
			b.sources[i] = s
			return nil
		}
	}
	b.sources = append(b.sources, s)
	return nil
}

// RemoveSource unmounts a source and drops its stubs and external edges.
func (b *Brain) RemoveSource(name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.source(name) == nil {
		return errf("no source %s", name)
	}
	for _, id := range append([]string(nil), b.order...) {
		if n := b.nodes[id]; n != nil && n.Ext != nil && n.Ext.Source == name {
			b.removeNode(id, false)
		}
	}
	b.sources = func() []*Source {
		out := []*Source{}
		for _, s := range b.sources {
			if s.Name != name {
				out = append(out, s)
			}
		}
		return out
	}()
	b.embedAll()
	b.refresh()
	return nil
}

// SuggestConnectors proposes connectors with evidence.
func (b *Brain) SuggestConnectors(limit int) []ConnectorSuggestion {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.suggestConnectors(limit)
}

// Resolve creates anchors from shared field values (fields named in by).
func (b *Brain) Resolve(name string, by []string, min int, dry bool) (*ResolveResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !dry {
		snap := b.clone()
		r, err := b.resolve(name, by, min, false)
		if err != nil {
			b.restore(snap)
			return nil, err
		}
		b.Step++
		b.embedAll()
		b.refresh()
		return r, nil
	}
	return b.resolve(name, by, min, true)
}

// ResolveEntities creates anchors for nodes sharing a normalised entity value of one type.
func (b *Brain) ResolveEntities(typ string, min int, dry bool) (*ResolveResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if min < 2 {
		min = 2
	}
	groups := map[string][]string{}
	raw := map[string]string{}
	order := []string{}
	for _, id := range b.memories() {
		for _, e := range b.entities(id) {
			if e.Type != typ {
				continue
			}
			if _, ok := groups[e.Value]; !ok {
				order = append(order, e.Value)
				raw[e.Value] = e.Raw
			}
			if !contains(groups[e.Value], id) {
				groups[e.Value] = append(groups[e.Value], id)
			}
		}
	}
	res := &ResolveResult{Anchors: []ConnectorSuggestion{}, Created: []string{}}
	snap := b.clone()
	for _, v := range order {
		ids := groups[v]
		if len(ids) < min {
			continue
		}
		label := raw[v]
		if typ == "card" || contains(b.Privacy, typ) {
			label = patterns.Mask(typ, label)
		}
		s := ConnectorSuggestion{Type: "anchor", Label: label, Key: typ + ":" + v, Members: ids, Evidence: fmt.Sprintf("%d items mention %s %s", len(ids), typ, label), Score: float64(len(ids))}
		res.Anchors = append(res.Anchors, s)
		if dry {
			continue
		}
		h, err := b.addConnector(ConnectorSpec{Type: "anchor", Label: label, Key: s.Key})
		if err != nil {
			b.restore(snap)
			return nil, err
		}
		for _, id := range ids {
			b.addEdge(id, h, .9, EdgeConnector)
		}
		res.Created = append(res.Created, h)
	}
	if !dry && len(res.Created) > 0 {
		b.Step++
		b.embedAll()
		b.refresh()
	}
	return res, nil
}

// AddPattern registers a custom pattern (validated).
func (b *Brain) AddPattern(c *patterns.Custom) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := c.Compile(); err != nil {
		return err
	}
	o := b.patOpts()
	for i, x := range o.Custom {
		if x.Name == c.Name {
			o.Custom[i] = c
			b.entCache = nil
			return nil
		}
	}
	o.Custom = append(o.Custom, c)
	b.entCache = nil
	return nil
}

// RemovePattern removes a custom pattern.
func (b *Brain) RemovePattern(name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	o := b.patOpts()
	for i, x := range o.Custom {
		if x.Name == name {
			o.Custom = append(o.Custom[:i], o.Custom[i+1:]...)
			b.entCache = nil
			return nil
		}
	}
	return errf("no custom pattern %s", name)
}

// CustomPattern returns a custom pattern by name.
func (b *Brain) CustomPattern(name string) *patterns.Custom {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, x := range b.patOpts().Custom {
		if x.Name == name {
			return x
		}
	}
	return nil
}

// PIICounts counts PII entities per type across memory.
func (b *Brain) PIICounts() map[string]int {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := map[string]int{}
	for _, id := range b.memories() {
		for _, e := range b.entities(id) {
			if e.PII {
				out[e.Type]++
			}
		}
	}
	return out
}

// Sections returns section names with item counts.
func (b *Brain) Sections() []*OMap {
	b.mu.Lock()
	defer b.mu.Unlock()
	cnt := map[string]int{}
	for _, id := range b.memories() {
		cnt[b.nodes[id].Section]++
	}
	out := []*OMap{}
	for _, s := range b.sectionOrder() {
		out = append(out, NewOMap().Set("name", s).Set("items", cnt[s]))
	}
	return out
}

// ClosestID suggests an existing id close to a misspelled one ("" if none within distance 2).
func (b *Brain) ClosestID(id string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return Closest(id, b.order, 2)
}

// Closest returns the candidate with the smallest edit distance ≤ max ("" if none).
func Closest(s string, cands []string, max int) string {
	best, bd := "", max+1
	ls := strings.ToLower(s)
	for _, c := range cands {
		if d := levenshtein(ls, strings.ToLower(c)); d < bd {
			best, bd = c, d
		}
	}
	return best
}

func levenshtein(a, c string) int {
	ra, rc := []rune(a), []rune(c)
	if len(ra) == 0 {
		return len(rc)
	}
	prev := make([]int, len(rc)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rc)+1)
		cur[0] = i
		for j := 1; j <= len(rc); j++ {
			cost := 1
			if ra[i-1] == rc[j-1] {
				cost = 0
			}
			cur[j] = min3(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rc)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// FieldNames lists all field names used by records.
func (b *Brain) FieldNames() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	seen := map[string]bool{}
	for _, id := range b.order {
		for _, f := range b.nodes[id].Fields {
			seen[f.Name] = true
		}
	}
	out := []string{}
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Brief returns the kind and text of a node ("", id when missing).
func (b *Brain) Brief(id string) (string, string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := b.nodes[id]
	if n == nil {
		return "", id
	}
	return n.Kind, b.maskText(n.Text)
}
