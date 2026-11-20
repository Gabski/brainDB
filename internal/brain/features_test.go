// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestYAMLRoundTrip(t *testing.T) {
	src := `# comment
name: crm
server: {addr: "127.0.0.1:7791", tokenEnv: BRAINDB_TOKEN}
context: {depth: 2, threshold: 0.16, focus: [d121, "d 123"]}
kinds:
  - {name: customer, halfLife: 90, base: 0.7}
  - name: fact
    halfLife: 45
deals[]: "open deals|kind=deal|limit=5"   # list slot
deals[].title: ".text"
quoted: 'it''s'
escaped: "tab\there"
empty: {}
none: []
nothing:
flag: true
multi: |
  line one
  line two
`
	v, err := ParseYAML(src)
	if err != nil {
		t.Fatal(err)
	}
	m := v.(*OMap)
	if m.Keys[0] != "name" || m.Keys[len(m.Keys)-1] != "multi" {
		t.Fatalf("key order lost: %v", m.Keys)
	}
	if m.Lines["kinds"] != 5 {
		t.Fatalf("line numbers: kinds at %d", m.Lines["kinds"])
	}
	ctx := m.Vals["context"].(*OMap)
	if f := ctx.Vals["focus"].([]any); f[1] != "d 123" {
		t.Fatalf("flow list: %v", f)
	}
	if m.Vals["quoted"] != "it's" || m.Vals["escaped"] != "tab\there" || m.Vals["multi"] != "line one\nline two\n" || m.Vals["flag"] != true {
		t.Fatalf("scalars: %#v", m.Vals)
	}
	if k := m.Vals["kinds"].([]any)[1].(*OMap); k.Vals["halfLife"] != json.Number("45") {
		t.Fatalf("map in list: %#v", k.Vals)
	}
	out := EmitYAML(v)
	v2, err := ParseYAML(out)
	if err != nil {
		t.Fatalf("emitted YAML does not parse: %v\n%s", err, out)
	}
	a, _ := json.Marshal(v)
	b, _ := json.Marshal(v2)
	if string(a) != string(b) {
		t.Fatalf("round trip differs:\n%s\n%s\n%s", a, b, out)
	}
	if out2 := EmitYAML(v2); out2 != out {
		t.Fatal("emitter not stable")
	}
	for _, bad := range []string{"a: 1\n  b: 2\n", "a: [1, 2\n", "a: 1\na: 2\n", "- x\nb: 1\n"} {
		if _, err := ParseYAML(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		} else if !strings.Contains(err.Error(), "line ") {
			t.Errorf("error without line number: %v", err)
		}
	}
}

func TestQueryParser(t *testing.T) {
	q, err := ParseQuery(`kind:deal,account section:pipeline concept:renewal lit:true pinned:false superseded:false w>=0.2 age<=30 stage=negotiation value>10000 owner~kim "free text" near:"renewal risk" sort:-value limit:10 select:id,text,fields.value`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(q.Where.Kind, ",") != "deal,account" || q.Where.Section[0] != "pipeline" || q.Where.Concept != "renewal" || !*q.Where.Lit || *q.Where.Pinned ||
		*q.Where.MinWeight != .2 || *q.Where.MaxAgeDays != 30 || q.Near != "renewal risk" || q.OrderBy != "-value" || q.Limit != 10 || len(q.Select) != 3 || q.Where.Text != "free text" {
		t.Fatalf("parsed: %+v", q)
	}
	if len(q.Where.Fields) != 3 || q.Where.Fields[1] != (FieldFilter{"value", ">", "10000"}) || q.Where.Fields[2].Op != "~" {
		t.Fatalf("field filters: %+v", q.Where.Fields)
	}
	q2, err := ParseQuery(`{"where":{"kind":"deal","fields":[{"name":"value","op":">","value":10000}]},"limit":5}`)
	if err != nil || q2.Where.Kind[0] != "deal" || q2.Where.Fields[0].Value != "10000" {
		t.Fatalf("json query: %+v %v", q2, err)
	}
	for _, c := range []struct {
		a, op, b string
		want     bool
	}{{"€48,000", ">", "10000", true}, {"$12.5k", "=", "12500", true}, {"12%", "<", "20", true}, {"Negotiation", "=", "negotiation", true},
		{"Kim Aalto", "~", "aalto", true}, {"abc", "!=", "abd", true}, {"9 000 zł", ">=", "9000", true}, {"b", "<", "a", false}} {
		if compare(c.a, c.op, c.b) != c.want {
			t.Errorf("compare(%q %s %q) != %v", c.a, c.op, c.b, c.want)
		}
	}
}

func fillBrain(t *testing.T) *Brain {
	b := New()
	fixedClock(b)
	seed := `{"name":"F","goal":"Renew Halvorsen","kinds":[{"name":"deal","halfLife":21,"prefix":"g","section":"pipeline"}],
"concepts":[{"id":"renewal","label":"Renewal"}],
"memories":[
 {"id":"f1","kind":"fact","text":"Client Halvorsen Freight AS, contact Kim Aalto, kim.aalto@halvorsen.example, phone +47 22 33 44 55","concepts":["renewal"],"ageDays":1},
 {"id":"f2","kind":"fact","text":"Contract renewal date is 15 Dec 2026; annual contract value €48,000; discount 6%","concepts":["renewal"],"ageDays":1},
 {"id":"f3","kind":"fact","text":"Pricing page https://halvorsen.example/pricing lists 140 trucks","ageDays":1},
 {"id":"g1","kind":"deal","text":"Halvorsen renewal","concepts":["renewal"],"ageDays":1,"fields":{"stage":"negotiation","value":"48000"}},
 {"id":"g2","kind":"deal","text":"Ostrava upsell","ageDays":1,"fields":{"stage":"proposal","value":"12500"}},
 {"id":"q1","kind":"question","text":"Main risk: procurement caps terms at 24 months","concepts":["renewal"],"ageDays":1}],
"links":[["g1","q1",0.8],["g1","f2",0.7]],"focus":["g1","f1"]}`
	var s Seed
	if err := json.Unmarshal([]byte(seed), &s); err != nil {
		t.Fatal(err)
	}
	if err := b.LoadSeed(&s); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestFillSchemaYAML(t *testing.T) {
	b := fillBrain(t)
	tpl := `client:
  name: "client company:org"
  contact: "contact person:name"
  email: "contact email:email"
  phone: "contact phone:phone"
  renewal_date: "contract renewal date:date"
  arr: "annual contract value:money"
  discount: "discount:percent"
  site: "pricing page:url"
  trucks: "number of trucks:number"
  domain: "client domain:domain"
deals[]: "open deals|kind=deal|limit=5"
deals[].title: ".text"
deals[].stage: ".fields.stage"
deals[].risk: "main risk for this deal"
unknown: "galactic tax rate on the moon:percent|min=0.95"
`
	r, err := b.Fill(FillRequest{Template: tpl, Output: "json", WithSources: true})
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(r.Output), &out); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, r.Output)
	}
	c := out["client"].(map[string]any)
	want := map[string]any{"name": "Halvorsen Freight", "contact": "Kim Aalto", "email": "kim.aalto@halvorsen.example", "phone": "+4722334455",
		"renewal_date": "2026-12-15", "arr": "€48,000", "discount": "6%", "site": "https://halvorsen.example/pricing", "trucks": 140.0, "domain": "halvorsen.example"}
	for k, v := range want {
		if c[k] != v {
			t.Errorf("client.%s = %#v, want %#v", k, c[k], v)
		}
	}
	deals := out["deals"].([]any)
	if len(deals) != 2 || deals[0].(map[string]any)["stage"] == nil {
		t.Fatalf("list slot: %#v", deals)
	}
	if !contains(r.Missing, "unknown") {
		t.Fatalf("unknown must be missing: %v", r.Missing)
	}
	if _, ok := out["_sources"]; !ok {
		t.Error("withSources adds _sources")
	}
	for _, v := range r.Values {
		if v.Path == "client.email" && v.Source != "f1" {
			t.Errorf("evidence source: %+v", v)
		}
	}
	for _, f := range []string{"yaml", "xml", "md"} {
		if _, err := b.Fill(FillRequest{Template: tpl, Output: f}); err != nil {
			t.Errorf("output %s: %v", f, err)
		}
	}
}

func TestFillJSONXMLText(t *testing.T) {
	b := fillBrain(t)
	r, err := b.Fill(FillRequest{Template: `{"deals": [{"_each": "open deals|kind=deal", "title": ".text", "value": ".fields.value"}], "email": "contact email:email", "list": ["open deals|kind=deal"]}`})
	if err != nil {
		t.Fatal(err)
	}
	if r.Format != "json" || !strings.Contains(r.Output, `"title": "Halvorsen renewal"`) || !strings.Contains(r.Output, `"Ostrava upsell"`) {
		t.Fatalf("json array-of-one-object list:\n%s", r.Output)
	}
	r, err = b.Fill(FillRequest{Template: `<brief><client email="contact email:email">client company:org</client><deal repeat="open deals|kind=deal" stage=".fields.stage">.text</deal></brief>`, WithSources: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Output, `<client email="kim.aalto@halvorsen.example" source="f1">Halvorsen Freight</client>`) || strings.Count(r.Output, "<deal ") != 2 {
		t.Fatalf("xml:\n%s", r.Output)
	}
	r, err = b.Fill(FillRequest{Template: "Contact {{ contact person:name }} at {{ contact email:email }}.\n{{#each open deals|kind=deal}}- {{ .text }} ({{ .fields.stage }})\n{{/each}}Tax: {{ moon tax:percent|min=0.99 }}", WithSources: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Output, "Contact Kim Aalto[^1] at kim.aalto@halvorsen.example") || !strings.Contains(r.Output, "- Halvorsen renewal") ||
		!strings.Contains(r.Output, "[missing: moon tax]") || !strings.Contains(r.Output, "[^1]: f1") {
		t.Fatalf("text template:\n%s", r.Output)
	}
	if _, err := b.Fill(FillRequest{Template: "{{#each x}} no end"}); err == nil {
		t.Error("unclosed each must fail")
	}
}

func TestFillSlotParsing(t *testing.T) {
	s := parseSlot("deal value:money|kind=deal|min=0.3")
	if s.desc != "deal value" || s.typ != "money" || s.opts["kind"] != "deal" || s.opts["min"] != "0.3" {
		t.Fatalf("%+v", s)
	}
	if s := parseSlot("A:ratio of things"); s.typ != "text" || s.desc != "A:ratio of things" {
		t.Fatalf("unknown type suffix stays in the description: %+v", s)
	}
	for typ := range SlotTypes {
		if s := parseSlot("x:" + typ); s.typ != typ {
			t.Errorf("type %s not parsed", typ)
		}
	}
}

func TestConnectors(t *testing.T) {
	b := New()
	fixedClock(b)
	for _, id := range []string{"a", "b", "c", "d"} {
		b.addNode(&Node{ID: id, Kind: "fact", Base: 1, Pin: true, Text: "item " + id})
	}
	h, _ := b.addConnector(ConnectorSpec{Type: "anchor", Label: "Customer 42", Key: "customer:42"})
	o, _ := b.addConnector(ConnectorSpec{Type: "hub", Label: "Q4"})
	b.addEdge("a", h, 1, EdgeConnector)
	b.addEdge(h, "b", 1, EdgeConnector)
	b.addEdge("a", o, 1, EdgeConnector)
	b.addEdge(o, "c", 1, EdgeConnector)
	b.embedAll()
	r := b.activate([]string{"a"}, Params{Depth: 1, Threshold: .01, Hop: .85}, nil)
	if !r.has("b") || r.depth["b"] != 1 || r.parent["b"] != "a" || strings.Join(r.route["b"], ",") != h {
		t.Fatalf("transparent anchor: b act=%v depth=%d via=%q route=%v", r.act["b"], r.depth["b"], r.parent["b"], r.route["b"])
	}
	if want := .95 * .85; math.Abs(r.act["b"]-want) > 1e-12 {
		t.Fatalf("gain without hop on entry: %v want %v", r.act["b"], want)
	}
	if r.has("c") {
		t.Fatal("opaque hub consumes a depth level")
	}
	r = b.activate([]string{"a"}, Params{Depth: 2, Threshold: .01, Hop: .85}, nil)
	if !r.has("c") || r.depth["c"] != 2 {
		t.Fatalf("opaque hub at depth 2: %v", r.depth["c"])
	}
	b.seeds = []string{"a"}
	b.P = Params{Depth: 2, Threshold: .01, Hop: .85}
	b.refresh()
	st := b.contextStats(b.cur)
	if st.Mem != 3 || st.Abs != 0 {
		t.Fatalf("connectors are not counted as lit memories: %+v", st)
	}
	x := b.contextXML(b.curLit())
	if !strings.Contains(x, `route="`+h+`"`) || !strings.Contains(x, `<route id="`+h+`" type="anchor"`) || strings.Contains(x, `<item id="`+h) {
		t.Fatalf("context XML routes:\n%s", x)
	}
	res, _ := b.search("item", "hybrid", 10)
	for _, row := range res.Rows {
		if isConn(b.nodes[row.ID]) {
			t.Fatal("connectors are never search rows")
		}
	}
	// fan-out guard
	old := MaxFanout
	MaxFanout = 1
	defer func() { MaxFanout = old }()
	b.addEdge(h, "d", .1, EdgeConnector)
	r = b.activate([]string{"a"}, Params{Depth: 1, Threshold: .001, Hop: .85}, nil)
	if r.has("d") && r.has("b") {
		t.Fatal("fan-out guard must limit a connector to its top neighbours")
	}
}

func TestAliasExpansion(t *testing.T) {
	b := fillBrain(t)
	before, _ := b.Search("ARR", "vector", 3)
	if _, err := b.Exec([]Op{{Op: "connector", CType: "alias", Label: "ARR", Terms: StrList{"annual contract value"}}}); err != nil {
		t.Fatal(err)
	}
	after, _ := b.Search("ARR", "vector", 3)
	if after.Rows[0].ID != "f2" || (len(before.Rows) > 0 && before.Rows[0].ID == "f2") {
		t.Fatalf("alias must route ARR to f2: before %v after %v", before.Rows, after.Rows)
	}
}

func TestResolveEntities(t *testing.T) {
	b := fillBrain(t)
	if _, err := b.Exec([]Op{{Op: "remember", Kind: "note", Text: sp("Email kim.aalto@halvorsen.example about the renewal")}}); err != nil {
		t.Fatal(err)
	}
	r, err := b.ResolveEntities("email", 2, false)
	if err != nil || len(r.Created) != 1 {
		t.Fatalf("resolve email: %+v %v", r, err)
	}
	if d := len(b.adj[r.Created[0]]); d != 2 {
		t.Fatalf("anchor degree %d", d)
	}
}

func TestPrivacyMasking(t *testing.T) {
	b := fillBrain(t)
	if _, err := b.Exec([]Op{{Op: "remember", Kind: "note", Text: sp("Paid with card 4111 1111 1111 1111"), Focus: json.RawMessage("true")}}); err != nil {
		t.Fatal(err)
	}
	b.Privacy = []string{"phone"}
	for name, out := range map[string]string{"xml": b.ContextXML(), "md": b.ContextMD()} {
		if strings.Contains(out, "4111 1111 1111 1111") || strings.Contains(out, "+47 22 33 44 55") {
			t.Errorf("%s leaks PII:\n%s", name, out)
		}
	}
	r, _ := b.QueryContext(ContextQuery{Focus: StrList{"f1"}, Format: "json"})
	if strings.Contains(r.Output, "22 33 44 55") {
		t.Error("context query json leaks the phone")
	}
}
