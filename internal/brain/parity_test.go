// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"encoding/json"
	"flag"
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

// Parity golden test. testdata/parity_oracle.json holds results computed once by the original JavaScript
// implementation of these algorithms (run as an oracle outside this repository) on testdata/parity_brain.xml, which is
// written in the legacy XML dialect (<abstractions>, <context seeds=…>, edge types abstr/sup/sim). The Go port must
// reproduce activation, stats, vectors (bit-exact float32), all four search modes, suggestions, ablation, paths,
// discover and chunking. `go test ./internal/brain -run TestParity -update` regenerates parity_brain.xml from
// testdata/parity_seed.json; the oracle JSON must then be regenerated with the reference implementation.

var update = flag.Bool("update", false, "regenerate parity_brain.xml")

var parityNow = time.Date(2026, 11, 20, 9, 30, 0, 0, time.UTC)

func fixedClock(b *Brain) { b.Now = func() time.Time { return parityNow } }

// protoXML exports a brain in the legacy XML dialect.
func protoXML(b *Brain) string {
	x := b.brainXML()
	r := strings.NewReplacer(`<concepts>`, `<abstractions>`, `</concepts>`, `</abstractions>`, `use="enriched"`, `use="enr"`,
		`type="concept"`, `type="abstr"`, `type="supersede"`, `type="sup"`, `type="similar"`, `type="sim"`, `<context focus=`, `<context seeds=`)
	return r.Replace(x)
}

type oracle struct {
	Act []struct {
		ID    string  `json:"id"`
		Act   float64 `json:"act"`
		Depth int     `json:"depth"`
		Via   string  `json:"via"`
	} `json:"act"`
	Stats struct {
		Mem, Abs, Tok int
		Sum, AvgW     float64
	} `json:"stats"`
	Search map[string]struct {
		Rows []struct {
			ID     string   `json:"id"`
			Score  float64  `json:"score"`
			Raw    *float64 `json:"raw"`
			Enr    *float64 `json:"enr"`
			Vec    *float64 `json:"vec"`
			Graph  *float64 `json:"graph"`
			Lift   *int     `json:"lift"`
			Depth  *int     `json:"depth"`
			Via    string   `json:"via"`
			Parent string   `json:"parent"`
		} `json:"rows"`
		Via []struct {
			ID    string  `json:"id"`
			Score float64 `json:"score"`
			Depth int     `json:"depth"`
			Via   string  `json:"via"`
		} `json:"via"`
	} `json:"search"`
	HashEmbed []struct {
		Text string    `json:"text"`
		V    []float64 `json:"v"`
	} `json:"hashEmbed"`
	Raw       map[string][]float64 `json:"raw"`
	Enr       map[string][]float64 `json:"enr"`
	Coherence struct{ Raw, Enr float64 }
	Hash      []struct {
		S string `json:"s"`
		H uint32 `json:"h"`
	} `json:"hash"`
	Stems   string   `json:"stems"`
	Chunks  []string `json:"chunks"`
	Suggest []struct {
		V string  `json:"v"`
		S float64 `json:"s"`
	} `json:"suggest"`
	Test struct {
		Reached  []Reached `json:"reached"`
		Ablation []struct {
			To       string  `json:"to"`
			DLit     int     `json:"dLit"`
			DSum     float64 `json:"dSum"`
			Critical bool    `json:"critical"`
		} `json:"ablation"`
	} `json:"test"`
	Paths []struct {
		Path     []string `json:"path"`
		Strength float64  `json:"strength"`
	} `json:"paths"`
	Discover struct{ Inner, Bridge [][]any } `json:"discover"`
	NodeW    map[string]float64              `json:"nodeW"`
}

func near(a, b float64) bool { return math.Abs(a-b) <= 1e-9*math.Max(1, math.Abs(b)) }

func loadParity(t *testing.T) *Brain {
	t.Helper()
	if *update {
		b := New()
		fixedClock(b)
		if err := b.LoadSeedFile("testdata/parity_seed.json"); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile("testdata/parity_brain.xml", []byte(protoXML(b)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	src, err := os.ReadFile("testdata/parity_brain.xml")
	if err != nil {
		t.Fatal(err)
	}
	b := New()
	fixedClock(b)
	if err := b.LoadXML(string(src)); err != nil {
		t.Fatal(err)
	}
	// The oracle predates the durable-knowledge decay defaults, so the fixture pins the old policy explicitly.
	// Parity is about the algorithms (activation, vectors, search, paths), not about which half-life ships today.
	if err := b.SetKindDecay(LegacyDecay()...); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParity(t *testing.T) {
	b := loadParity(t)
	d, err := os.ReadFile("testdata/parity_oracle.json")
	if err != nil {
		t.Skip("no oracle output; run testdata/oracle.mjs")
	}
	var o oracle
	if err := json.Unmarshal(d, &o); err != nil {
		t.Fatal(err)
	}
	// weights
	for id, w := range o.NodeW {
		if n := b.nodes[id]; n == nil || !near(b.nodeW(n), w) {
			t.Errorf("nodeW %s: go %v js %v", id, b.nodeW(b.nodes[id]), w)
		}
	}
	// activation map: same ids, order, values, depth, via
	if len(o.Act) != len(b.cur.order) {
		t.Fatalf("lit count: go %d js %d", len(b.cur.order), len(o.Act))
	}
	for i, a := range o.Act {
		id := b.cur.order[i]
		if id != a.ID || !near(b.cur.act[id], a.Act) || b.cur.depth[id] != a.Depth || b.cur.parent[id] != a.Via {
			t.Errorf("act[%d]: go %s %v d%d via %q; js %s %v d%d via %q", i, id, b.cur.act[id], b.cur.depth[id], b.cur.parent[id], a.ID, a.Act, a.Depth, a.Via)
		}
	}
	st := b.contextStats(b.cur)
	if st.Mem != o.Stats.Mem || st.Abs != o.Stats.Abs || st.Tok != o.Stats.Tok || !near(st.Sum, o.Stats.Sum) || !near(st.AvgW, o.Stats.AvgW) {
		t.Errorf("contextStats: go %+v js %+v", st, o.Stats)
	}
	// embeddings: bit-exact float32
	for _, h := range o.HashEmbed {
		v := HashEmbed(h.Text)
		for i := range v {
			if float64(v[i]) != h.V[i] {
				t.Fatalf("hashEmbed(%q)[%d]: go %v js %v", h.Text, i, v[i], h.V[i])
			}
		}
	}
	for id, v := range o.Raw {
		for i := range v {
			if float64(b.raw[id][i]) != v[i] || float64(b.enr[id][i]) != o.Enr[id][i] {
				t.Fatalf("vector %s[%d]: raw go %v js %v; enr go %v js %v", id, i, b.raw[id][i], v[i], b.enr[id][i], o.Enr[id][i])
			}
		}
	}
	if !near(b.coherence(b.raw), o.Coherence.Raw) || !near(b.coherence(b.enr), o.Coherence.Enr) {
		t.Errorf("coherence differs")
	}
	for _, h := range o.Hash {
		if hash(h.S) != h.H {
			t.Errorf("hash(%q): go %d js %d", h.S, hash(h.S), h.H)
		}
	}
	if got := deacc("Zażółć Gęślą ŁÓDŹ façade"); got != o.Stems {
		t.Errorf("deacc: go %q js %q", got, o.Stems)
	}
	chunkSrc := "# Title\n\nFirst paragraph with some text.\n\n\n  \nSecond paragraph. " + strings.Repeat("Long sentence goes here. ", 60) + "\n## Heading two\nmore text\n\nok"
	if got := chunkText(chunkSrc, 650); strings.Join(got, "|") != strings.Join(o.Chunks, "|") {
		t.Errorf("chunkText:\ngo %q\njs %q", got, o.Chunks)
	}
	// search: same row order and scores
	for key, want := range o.Search {
		mode, q, _ := strings.Cut(key, ":")
		r, err := b.search(q, mode, 8)
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Rows) != len(want.Rows) {
			t.Errorf("%s: %d rows, js %d", key, len(r.Rows), len(want.Rows))
			continue
		}
		for i, w := range want.Rows {
			g := r.Rows[i]
			p := ""
			if g.Parent != nil {
				p = *g.Parent
			}
			if g.ID != w.ID || !near(g.Score, w.Score) || g.Via != w.Via || p != w.Parent {
				t.Errorf("%s row %d: go %s %v via %q parent %q; js %s %v via %q parent %q", key, i, g.ID, g.Score, g.Via, p, w.ID, w.Score, w.Via, w.Parent)
			}
			if w.Lift != nil && (g.Lift == nil || *g.Lift != *w.Lift) {
				t.Errorf("%s row %d lift differs", key, i)
			}
		}
		for i, w := range want.Via {
			if i >= len(r.Via) || r.Via[i].ID != w.ID || !near(r.Via[i].Score, w.Score) {
				t.Errorf("%s via %d differs", key, i)
			}
		}
	}
	sg := b.suggestFor("d121", nil, 6)
	for i, w := range o.Suggest {
		if i >= len(sg) || sg[i].V != w.V || !near(sg[i].Score, w.S) {
			t.Errorf("suggest %d: js %v %v", i, w.V, w.S)
		}
	}
	tl := b.testLinks("d121")
	for i, w := range o.Test.Reached {
		if tl.Reached[i].ID != w.ID || !near(tl.Reached[i].Act, w.Act) || tl.Reached[i].Depth != w.Depth {
			t.Errorf("testLinks reached %d differs", i)
		}
	}
	for i, w := range o.Test.Ablation {
		g := tl.Ablation[i]
		if g.To != w.To || g.DLit != w.DLit || !near(g.DSum, w.DSum) || g.Critical != w.Critical {
			t.Errorf("ablation %d: go %+v js %+v", i, g, w)
		}
	}
	ps := b.kPaths("n150", "f101", 3)
	for i, w := range o.Paths {
		if strings.Join(ps[i].Path, ">") != strings.Join(w.Path, ">") || !near(ps[i].Strength, w.Strength) {
			t.Errorf("path %d: go %v js %v", i, ps[i].Path, w.Path)
		}
	}
	in, br := b.discover()
	cmp := func(name string, g []Suggestion, w [][]any) {
		if len(g) != len(w) {
			t.Errorf("discover %s: %d vs %d", name, len(g), len(w))
			return
		}
		for i := range w {
			if g[i].U != w[i][0] || g[i].V != w[i][1] || !near(g[i].Score, w[i][2].(float64)) {
				t.Errorf("discover %s %d differs", name, i)
			}
		}
	}
	cmp("inner", in, o.Discover.Inner)
	cmp("bridge", br, o.Discover.Bridge)
}
