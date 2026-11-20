// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func seeded(t testing.TB) *Brain {
	t.Helper()
	b := New()
	fixedClock(b)
	if err := b.LoadSeedFile("testdata/parity_seed.json"); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestWeightDecay(t *testing.T) {
	b := New()
	// floor 0 is the plain half-life curve
	if err := b.SetKindDecay(KindDecay{Kind: "fact", HalfLife: 30}); err != nil {
		t.Fatal(err)
	}
	n := &Node{ID: "f1", Kind: "fact", Base: .6, Age: 30}
	if w := b.nodeW(n); math.Abs(w-.3) > 1e-12 {
		t.Fatalf("one half-life: want 0.3, got %v", w)
	}
	n.Pin = true
	if b.nodeW(n) != .6 {
		t.Fatal("pinned items must not decay")
	}
	n.Pin, n.Sup = false, "f2"
	if w := b.nodeW(n); math.Abs(w-.09) > 1e-12 {
		t.Fatalf("superseded ×0.3: want 0.09, got %v", w)
	}
	c := &Node{ID: "A:x", Kind: Concept, Base: .8, Age: 1000}
	if b.nodeW(c) != .8 {
		t.Fatal("concepts have a fixed weight")
	}
	big := &Node{ID: "f3", Kind: "fact", Base: 5, Pin: true}
	if b.nodeW(big) != 1 {
		t.Fatal("weights are clamped to 1")
	}
}

func TestActivationRule(t *testing.T) {
	b := New()
	b.Now = func() time.Time { return parityNow }
	b.addNode(&Node{ID: "a", Kind: "fact", Base: 1, Pin: true})
	b.addNode(&Node{ID: "b", Kind: "fact", Base: 1, Pin: true})
	b.addNode(&Node{ID: "c", Kind: "fact", Base: 1, Pin: true, Sup: "b"})
	b.addNode(&Node{ID: "d", Kind: "fact", Base: .2, Pin: true})
	b.addEdge("a", "b", .8, EdgeAssoc)
	b.addEdge("b", "d", .5, EdgeAssoc)
	b.addEdge("a", "c", 1, EdgeAssoc)
	r := b.activate([]string{"a"}, Params{Depth: 2, Threshold: .01, Hop: .85}, nil)
	if want := .8 * .85; math.Abs(r.act["b"]-want) > 1e-12 || r.depth["b"] != 1 || r.parent["b"] != "a" {
		t.Fatalf("b: act %v depth %d via %q", r.act["b"], r.depth["b"], r.parent["b"])
	}
	wantD := r.act["b"] * .5 * .85 * (.35 + .65*.2)
	if math.Abs(r.act["d"]-wantD) > 1e-12 || r.depth["d"] != 2 {
		t.Fatalf("d: act %v want %v", r.act["d"], wantD)
	}
	if want := 1 * .85 * (.35 + .65*.3) * .15; math.Abs(r.act["c"]-want) > 1e-12 {
		t.Fatalf("superseded c must be ×0.15: %v want %v", r.act["c"], want)
	}
	r = b.activate([]string{"a"}, Params{Depth: 1, Threshold: .1, Hop: .85}, nil)
	if r.has("d") {
		t.Fatal("depth 1 must not reach d")
	}
	r = b.activate([]string{"a"}, Params{Depth: 2, Threshold: .5, Hop: .85}, nil)
	if r.has("d") || !r.has("b") {
		t.Fatal("threshold must cut weak activation")
	}
}

func TestXMLRoundTrip(t *testing.T) {
	b := seeded(t)
	if _, err := b.Exec([]Op{{Op: "remember", Kind: "fact", Text: sp(`Quotes "<&>" and ünïcödé`), Fields: Fields{{"stage", "won"}, {"value", "€1,200"}}},
		{Op: "connector", CType: "anchor", Label: "Customer 42", Key: "customer:42", Connect: StrList{"f101", "f102"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddKind(NewKind{Name: "deal", HalfLife: 21, Color: "#ff8787", Section: "pipeline"}); err != nil {
		t.Fatal(err)
	}
	_ = b.AddSource(SourceConfig{Name: "lab", URI: "http://lab:7791", TokenEnv: "LAB_TOKEN"})
	x1 := b.XML()
	b2 := New()
	fixedClock(b2)
	if err := b2.LoadXML(x1); err != nil {
		t.Fatal(err)
	}
	x2 := b2.XML()
	if x1 != x2 {
		t.Fatalf("round trip differs:\n%s\n----\n%s", x1, x2)
	}
	for _, want := range []string{`data="{&quot;stage&quot;:&quot;won&quot;`, `<connector id="H:customer-42" type="anchor"`, `<source name="lab"`, `<kinds>`} {
		if !strings.Contains(x1, want) {
			t.Errorf("XML lacks %s", want)
		}
	}
	if strings.Contains(x1, "LAB_TOKEN=") {
		t.Error("tokens must never be stored")
	}
}

func sp(s string) *string { return &s }

func TestExecAtomicAndTmpRefs(t *testing.T) {
	b := seeded(t)
	before := b.XML()
	_, err := b.Exec([]Op{{Op: "remember", Kind: "fact", Text: sp("x"), Tmp: "A"}, {Op: "link", A: "@A", B: "nosuch"}})
	if err == nil || err.Error() != "op#2 link: no such item nosuch" {
		t.Fatalf("want op#2 error, got %v", err)
	}
	if b.XML() != before {
		t.Fatal("failed batch changed the brain")
	}
	r, err := b.Exec([]Op{
		{Op: "remember", Kind: "fact", Text: sp("Nightly sync fits the Graph API limit"), Concepts: StrList{"API integrations", "Performance"}, Tmp: "F", Focus: json.RawMessage("true")},
		{Op: "remember", Kind: "decision", Text: sp("Sync accounts once a day"), Links: []LinkSpec{{To: "@F"}}, Supersedes: "f107"},
		{Op: "boost", ID: "d121"},
	})
	if err != nil {
		t.Fatal(err)
	}
	f, d := r.Created[0], r.Created[1]
	if b.edgeBetween(d, f) == nil {
		t.Fatal("@F reference not resolved")
	}
	if b.nodes["f107"].Sup != d {
		t.Fatal("supersedes not applied")
	}
	if b.findConcept("Performance", false) == "" || b.edgeBetween(f, "A:api") == nil {
		t.Fatal("concepts must be found or created")
	}
	if !contains(b.seeds, f) {
		t.Fatal("focus: true must add the new memory to the focus")
	}
	if _, err := b.Exec([]Op{{Op: "forget", ID: "f101"}}); err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Fatalf("pinned items need force: %v", err)
	}
	// auto-link of a new memory with no concept edge
	r, err = b.Exec([]Op{{Op: "remember", Kind: "note", Text: sp("Entra ID supports OIDC for the SSO rollout")}})
	if err != nil {
		t.Fatal(err)
	}
	hasC := false
	for _, a := range r.Auto {
		if a.Type == EdgeConcept {
			hasC = true
		}
	}
	if !hasC {
		t.Fatalf("auto-link should add a concept edge: %+v", r.Auto)
	}
}

func TestSearchModes(t *testing.T) {
	b := seeded(t)
	for _, m := range []string{"hybrid", "vector", "tree", "graph"} {
		r, err := b.Search("SSO rollout with OIDC and Entra", m, 5)
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Rows) == 0 {
			t.Fatalf("%s: no rows", m)
		}
		ids := []string{}
		for _, x := range r.Rows {
			ids = append(ids, x.ID)
		}
		hit := false
		for _, id := range ids {
			if id == "d121" || id == "f103" || id == "t131" {
				hit = true
			}
		}
		if !hit {
			t.Errorf("%s: expected an SSO memory among %v", m, ids)
		}
		for i := 1; i < len(r.Rows) && m != "tree"; i++ {
			if r.Rows[i].Score > r.Rows[i-1].Score+1e-12 {
				t.Errorf("%s: rows not sorted by score", m)
			}
		}
	}
	if _, err := b.Search("  ", "hybrid", 5); err == nil {
		t.Error("empty query must fail")
	}
	if _, err := b.Search("x", "nope", 5); err == nil {
		t.Error("unknown mode must fail")
	}
}

func TestSuggestExcludesSiblingChunks(t *testing.T) {
	b := seeded(t)
	text := "# Rollout\n\n" + strings.Repeat("The SSO rollout uses OIDC with Entra ID for every user. ", 12) + "\n\n## Testing\n\n" +
		strings.Repeat("The SSO rollout uses OIDC with Entra ID and needs E2E tests. ", 12) + "\n\n## Deadline\n\nThe SSO rollout with OIDC ends on 30 November."
	if _, err := b.Ingest([]InFile{{Name: "plan.md", Data: []byte(text)}}, IngestOptions{Focus: true}); err != nil {
		t.Fatal(err)
	}
	var file string
	chunks := []string{}
	for _, id := range b.order {
		if n := b.nodes[id]; n.Kind == "file" {
			file = id
		} else if n.Kind == "chunk" {
			chunks = append(chunks, id)
		}
	}
	if len(chunks) < 2 {
		t.Fatalf("want several chunks, got %d", len(chunks))
	}
	for _, s := range b.suggestFor(chunks[0], nil, 20) {
		if b.nodes[s.V].Parent == file || s.V == file {
			t.Fatalf("suggest proposed sibling/parent %s", s.V)
		}
	}
	if !contains(b.seeds, file) {
		t.Fatal("small files are focused after ingest")
	}
}

func TestContextRender(t *testing.T) {
	b := seeded(t)
	x := b.ContextXML()
	for _, want := range []string{`<context now="2026-11-20`, `<goal>Ship the SSO login rollout</goal>`, `<focus refs="d121,d123,n150"/>`,
		`<concepts note="concepts lit by the context">`, `<section name="decisions">`, `via="d121"`, `concepts="idp:`, `<memory note=`} {
		if !strings.Contains(x, want) {
			t.Errorf("context XML lacks %q", want)
		}
	}
	md := b.ContextMD()
	for _, want := range []string{"# Context: Ship the SSO login rollout", "## Focus", "- **d121** · decision", "## Concepts (in context)",
		"## Activation tree (where the signal came from)", "## Statistics", "| lit memories |", "### Outdated versions (superseded, do not use)", "~~d120:"} {
		if !strings.Contains(md, want) {
			t.Errorf("context MD lacks %q", want)
		}
	}
	if strings.ContainsAny(md, "ąęłńśźż") && strings.Contains(md, "Kontekst") {
		t.Error("output must be English")
	}
}

func TestSeedLoader(t *testing.T) {
	b := seeded(t)
	if b.nodes["A:auth"] == nil || b.nodes["A:auth"].Kind != Concept {
		t.Fatal("concept ids get the A: prefix")
	}
	e := b.edgeBetween("d121", "A:idp")
	if e == nil || e.W < .7 || e.W > .9 {
		t.Fatalf("first concept weight 0.8 ± 0.1, got %+v", e)
	}
	if e := b.edgeBetween("d120", "d121"); e == nil || e.Type != EdgeSupersede || e.W < .5 {
		t.Fatalf("supersededBy adds a supersede edge 0.5: %+v", e)
	}
	arch := 0
	for _, id := range b.order {
		if n := b.nodes[id]; n.Age >= 25 && n.Age <= 305 && !contains([]string{"f101", "f102", "d120", "q140"}, id) && n.Kind != Concept {
			arch++
		}
	}
	if arch < 10 {
		t.Fatalf("archive memories missing: %d", arch)
	}
	// deterministic: same seed → same XML
	b2 := seeded(t)
	if b.XML() != b2.XML() {
		t.Fatal("seed loading must be deterministic")
	}
	for _, ex := range []string{"sales-company", "research-lab", "braindb-self"} {
		p := filepath.Join("..", "..", "examples", ex, "seed.json")
		if _, err := os.Stat(p); err != nil {
			continue
		}
		e := New()
		if err := e.LoadSeedFile(p); err != nil {
			t.Errorf("%s: %v", ex, err)
		}
	}
}

func TestCSVRecordsAndQuery(t *testing.T) {
	b := seeded(t)
	csv := "deal,stage,value,owner\nAcme renewal,negotiation,\"€48,000\",Kim\nBeta pilot,proposal,9000,Lee\nGamma,negotiation,12%,Kim\n"
	if _, err := b.Ingest([]InFile{{Name: "pipeline.csv", Data: []byte(csv)}}, IngestOptions{}); err != nil {
		t.Fatal(err)
	}
	q, err := ParseQuery(`kind:record section:pipeline value>10000 owner~ki sort:-value select:id,fields.deal,fields.value`)
	if err != nil {
		t.Fatal(err)
	}
	r, err := b.RunQuery(q, "csv")
	if err != nil {
		t.Fatal(err)
	}
	if r.Count != 1 || r.Rows[0].Vals["fields.deal"] != "Acme renewal" {
		t.Fatalf("currency-aware filter failed: %+v", r.Rendered)
	}
	for _, f := range []string{"json", "yaml", "xml", "md", "csv"} {
		if _, err := b.RunQuery(q, f); err != nil {
			t.Errorf("format %s: %v", f, err)
		}
	}
	if !strings.HasPrefix(r.Rendered, "id,fields.deal,fields.value\r\n") {
		t.Errorf("csv must be RFC 4180: %q", r.Rendered)
	}
}

// TestDecayFloor pins the property that makes brainDB safe to store knowledge in: a durable kind never fades to
// nothing, however old it gets. Only SUPERSEDE and FORGET remove knowledge; age only reorders it.
func TestDecayFloor(t *testing.T) {
	b := New()
	for _, kind := range []string{"fact", "decision", "record", "file", "chunk"} {
		k := b.kindM[kind]
		if k.Floor < .5 {
			t.Fatalf("%s: durable kinds need a floor ≥ 0.5, got %v", kind, k.Floor)
		}
		ancient := b.nodeW(&Node{ID: "x", Kind: kind, Base: k.Base, Age: 50 * 365})
		if want := k.Base * k.Floor; math.Abs(ancient-want) > 1e-9 {
			t.Fatalf("%s after 50 years: want %v (base × floor), got %v", kind, want, ancient)
		}
		fresh := b.nodeW(&Node{ID: "x", Kind: kind, Base: k.Base, Age: 0})
		if fresh <= ancient {
			t.Fatalf("%s: fresh must still outrank ancient (%v vs %v)", kind, fresh, ancient)
		}
	}
	// attention-shaped kinds keep the old behaviour: no floor, they really do go away
	for _, kind := range []string{"task", "question", "note"} {
		if f := b.kindM[kind].Floor; f != 0 {
			t.Fatalf("%s: attention-shaped kinds must have floor 0, got %v", kind, f)
		}
	}
}
