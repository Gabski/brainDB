// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func importBrain(t *testing.T) *Brain {
	b := New()
	fixedClock(b)
	b.addKind(Kind{Name: "customer", HalfLife: fp(180), Base: .7, Prefix: "u", Section: "customers"})
	return b
}

func findBySrc(b *Brain, src string) *Node {
	for _, id := range b.order {
		if n := b.nodes[id]; n.Src == src && n.Sup == "" {
			return n
		}
	}
	return nil
}

func TestImportMarkdown(t *testing.T) {
	b := importBrain(t)
	files := []InFile{
		{Rel: "notes/a.md", Data: []byte("---\ntitle: Alpha note\nkind: decision\nsection: plans\ntags: [renewal, pricing]\ncustomer_id: [12, 9]\n---\n# Alpha\n\nSee [[b]] and [the other](b.md).\n\n## Risks\n\nProcurement caps terms.\n\n```go\nfunc x() {}\n```\n")},
		{Rel: "notes/b.md", Data: []byte("# Beta\n\nPlain note.\n")},
	}
	rep, err := b.Import(files, ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	a := findBySrc(b, "notes/a.md")
	if a == nil || a.Kind != "decision" || a.Section != "plans" {
		t.Fatalf("front-matter kind/section: %+v", a)
	}
	if v, _ := a.Fields.Get("customer_id"); v != "12, 9" {
		t.Fatalf("list front-matter joined: %q", v)
	}
	if v, _ := a.Fields.Get("title"); v != "Alpha note" {
		t.Fatal("title kept as field")
	}
	if b.edgeBetween(a.ID, b.findConcept("renewal", false)) == nil || b.findConcept("Risks", false) == "" {
		t.Fatal("tags and headings become concepts")
	}
	bn := findBySrc(b, "notes/b.md")
	if e := b.edgeBetween(a.ID, bn.ID); e == nil || e.Type != EdgeAssoc {
		t.Fatal("wiki/relative links become assoc edges")
	}
	hasCode := false
	for _, id := range b.order {
		if n := b.nodes[id]; n.Parent == a.ID && strings.Contains(n.Text, "func x()") {
			hasCode = true
		}
	}
	if !hasCode || rep.Totals.Created != 2 {
		t.Fatalf("code stays in chunks; report %+v", rep.Totals)
	}
}

func TestImportCSVJSON(t *testing.T) {
	b := importBrain(t)
	cfg, errs := ParseConfig(`import:
  rules:
    - {match: "items.json", importer: json, kind: record, section: catalog, json: {path: "$.data.items"}, text: "{sku} {price.list}", concepts: [category]}
    - {match: "*.jsonl", importer: json, section: tickets}
`)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	files := []InFile{
		{Rel: "deals.csv", Data: []byte("deal,stage,value\nA,won,100\nB,lost,200\n")},
		{Rel: "items.json", Data: []byte(`{"meta":{},"data":{"items":[{"sku":"PX-1","category":"espresso","price":{"list":7900},"tags":["a","b"]},{"sku":"PX-2","category":"grinder","price":{"list":900}}]}}`)},
		{Rel: "t.jsonl", Data: []byte("{\"id\":\"T1\",\"customer\":{\"name\":\"Acme\"}}\n{\"id\":\"T2\",\"customer\":{\"name\":\"Beta\"}}\n")},
	}
	rep, err := b.Import(files, ImportOptions{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Totals.Records != 6 {
		t.Fatalf("records: %+v", rep.Files)
	}
	px := findBySrc(b, "items.json#0")
	if px == nil || px.Text != "PX-1 7900" || px.Section != "catalog" {
		t.Fatalf("json path + nested flattening + text template: %+v", px)
	}
	if v, _ := px.Fields.Get("tags"); v != "a, b" {
		t.Fatalf("scalar arrays joined: %q", v)
	}
	if b.edgeBetween(px.ID, b.findConcept("espresso", false)) == nil {
		t.Fatal("concept from field value")
	}
	t1 := findBySrc(b, "t.jsonl#0")
	if v, _ := t1.Fields.Get("customer.name"); v != "Acme" || t1.Section != "tickets" {
		t.Fatalf("jsonl: %+v", t1)
	}
	d := findBySrc(b, "deals.csv#1")
	if v, _ := d.Fields.Get("value"); v != "200" || d.Section != "deals" {
		t.Fatalf("csv record: %+v", d)
	}
}

const pgDump = `-- PostgreSQL dump
SET statement_timeout = 0;
CREATE TABLE public.customers (
    id integer NOT NULL,
    name text NOT NULL,
    segment text,
    CONSTRAINT customers_pkey PRIMARY KEY (id)
);
CREATE TABLE public.orders (
    id integer PRIMARY KEY,
    customer_id integer NOT NULL REFERENCES public.customers(id),
    total numeric(10,2),
    note text
);
COPY public.customers (id, name, segment) FROM stdin;
1	Acme	smb
2	Beta	smb
3	Gamma	enterprise
4	Delta	enterprise
\.
INSERT INTO public.orders (id, customer_id, total, note) VALUES (10, 1, 99.50, 'it''s ok'), (11, 2, 15.00, NULL);
ALTER TABLE ONLY public.orders ADD CONSTRAINT fk_c FOREIGN KEY (customer_id) REFERENCES public.customers(id);
`

const myDump = "-- MySQL dump\nCREATE TABLE `suppliers` (\n  `id` int NOT NULL AUTO_INCREMENT,\n  `name` varchar(100),\n  PRIMARY KEY (`id`)\n) ENGINE=InnoDB;\n" +
	"CREATE TABLE `products` (\n  `sku` varchar(20) NOT NULL,\n  `supplier_id` int,\n  `name` varchar(100),\n  PRIMARY KEY (`sku`),\n  KEY `idx` (`supplier_id`),\n  CONSTRAINT `fk_s` FOREIGN KEY (`supplier_id`) REFERENCES `suppliers` (`id`)\n);\n" +
	"INSERT INTO `suppliers` VALUES (1,'Pellucid S.r.l.'),(2,'Autoflow \\'GmbH\\'');\n" +
	"INSERT INTO `products` (`sku`,`supplier_id`,`name`) VALUES ('PX-1',1,'Espresso; two group'),('AX-1',2,'Bean to cup');\n"

func TestImportSQL(t *testing.T) {
	b := importBrain(t)
	cfg, errs := ParseConfig(`import:
  rules:
    - match: "crm.sql"
      importer: sql
      tables:
        customers: {kind: customer, text: "{name} ({segment})", key: id}
access:
  enabled: true
  defaults:
    import: {"crm.sql#orders": [finance]}
`)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	b.access = &cfg.Access
	rep, err := b.Import([]InFile{{Rel: "crm.sql", Data: []byte(pgDump)}, {Rel: "my.sql", Data: []byte(myDump)}}, ImportOptions{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	c1 := findBySrc(b, "crm.sql#customers/1")
	o10 := findBySrc(b, "crm.sql#orders/10")
	if c1 == nil || c1.Kind != "customer" || c1.Text != "Acme (smb)" || o10 == nil {
		t.Fatalf("COPY + config: %+v %+v\n%+v", c1, o10, rep.Files)
	}
	if v, _ := o10.Fields.Get("note"); v != "it's ok" {
		t.Fatalf("quote escapes: %q", v)
	}
	if e := b.edgeBetween(o10.ID, c1.ID); e == nil || e.W != .7 {
		t.Fatal("foreign key → assoc 0.7")
	}
	if strings.Join(o10.Labels, ",") != "finance" || len(c1.Labels) != 0 {
		t.Fatalf("table-scoped default labels: %v / %v", o10.Labels, c1.Labels)
	}
	if b.edgeBetween(c1.ID, b.findConcept("smb", false)) == nil {
		t.Fatal("low-cardinality column → concept")
	}
	p1 := findBySrc(b, "my.sql#products/PX-1")
	s1 := findBySrc(b, "my.sql#suppliers/1")
	s2 := findBySrc(b, "my.sql#suppliers/2")
	if p1 == nil || s2 == nil {
		t.Fatalf("mysql backticks / multi-row insert: %+v", rep.Files)
	}
	if v, _ := s2.Fields.Get("name"); v != "Autoflow 'GmbH'" {
		t.Fatalf("mysql escapes: %q", v)
	}
	if v, _ := p1.Fields.Get("name"); v != "Espresso; two group" {
		t.Fatalf("semicolon inside string: %q", v)
	}
	if b.edgeBetween(p1.ID, s1.ID) == nil {
		t.Fatal("mysql FK edge")
	}
	if !strings.Contains(rep.Mapping, "my.sql") || !strings.Contains(rep.Mapping, "products") {
		t.Fatalf("detected mapping: %s", rep.Mapping)
	}
}

func TestImportIncremental(t *testing.T) {
	b := importBrain(t)
	f := []InFile{{Rel: "d.csv", Data: []byte("k,v\na,1\nb,2\n")}}
	if _, err := b.Import(f, ImportOptions{}); err != nil {
		t.Fatal(err)
	}
	n0 := len(b.order)
	rep, _ := b.Import(f, ImportOptions{})
	if rep.Files[0].Status != "skipped" || len(b.order) != n0 {
		t.Fatalf("unchanged re-import must skip: %+v", rep.Files)
	}
	f2 := []InFile{{Rel: "d.csv", Data: []byte("k,v\na,1\nb,3\n")}}
	rep, _ = b.Import(f2, ImportOptions{})
	if old := findBySrc(b, "d.csv#1"); old == nil || old.Fields[1].Value != "2" {
		t.Fatalf("without --update changed items are kept: %+v", rep.Files)
	}
	rep, _ = b.Import(f2, ImportOptions{Update: true})
	nw := findBySrc(b, "d.csv#1")
	if rep.Files[0].Status != "updated" || nw.Fields[1].Value != "3" {
		t.Fatalf("--update supersedes: %+v", rep.Files)
	}
	superseded := 0
	for _, id := range b.order {
		if n := b.nodes[id]; n.Src == "d.csv#1" && n.Sup == nw.ID {
			superseded++
		}
	}
	if superseded != 1 {
		t.Fatal("the old version is superseded, not deleted")
	}
	dry := len(b.order)
	if _, err := b.Import([]InFile{{Rel: "x.csv", Data: []byte("a,b\n1,2\n")}}, ImportOptions{DryRun: true}); err != nil || len(b.order) != dry {
		t.Fatal("dry run must not change the brain")
	}
}

func TestImportDemoEndToEnd(t *testing.T) {
	root := filepath.Join("..", "..", "examples", "import-demo")
	if _, err := os.Stat(filepath.Join(root, "raw")); err != nil {
		t.Skip("examples/import-demo not present")
	}
	cfg, _, err := LoadConfigFile(filepath.Join(root, "braindb.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	b := New()
	fixedClock(b)
	if err := b.ApplyConfig(cfg, true); err != nil {
		t.Fatal(err)
	}
	files, err := CollectFiles([]string{filepath.Join(root, "raw")}, 0)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := b.Import(files, ImportOptions{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Totals.Errors != 0 || rep.Totals.Records < 100 || len(rep.Connectors) == 0 {
		t.Fatalf("import-demo: %s", rep.Text())
	}
}

func TestConfigValidation(t *testing.T) {
	_, errs := ParseConfig("name: x\ncontext:\n  depth: 2\n  trheshold: 0.2\nkinds:\n  - {name: a, halfLif: 3}\naccess:\n  users:\n    - {name: u, roles: [admn]}\n")
	got := []string{}
	for _, e := range errs {
		got = append(got, e.Error())
	}
	all := strings.Join(got, "\n")
	for _, want := range []string{`line 4: context.trheshold: unknown key "trheshold" (did you mean "threshold"?)`, `unknown key "halfLif" (did you mean "halfLife"?)`} {
		if !strings.Contains(all, want) {
			t.Errorf("want %q in:\n%s", want, all)
		}
	}
	if _, errs := ParseConfig("access:\n  enabled: true\n  users:\n    - {name: u, roles: [admn]}\n"); len(errs) == 0 || !strings.Contains(errs[0].Error(), `did you mean "admin"`) {
		t.Errorf("unknown role: %v", errs)
	}
	for _, ex := range []string{"sales-company", "research-lab", "import-demo", "braindb-self"} {
		p := filepath.Join("..", "..", "examples", ex, "braindb.yaml")
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if _, _, err := LoadConfigFile(p); err != nil {
			t.Errorf("%s: %v", ex, err)
		}
	}
}

// Server tokens name environment variables. The loader dies on a scope it does not know, so validation has to
// reject the same thing: a config that passes the dry run in the UI and then kills the server on start is worse
// than no validation at all. The shape check is what keeps a pasted secret out of a file that gets committed.
func TestServerTokenValidation(t *testing.T) {
	if _, errs := ParseConfig("name: x\nserver:\n  tokenEnv: BRAINDB_TOKEN\n  tokens:\n    - {name: bot, tokenEnv: BRAINDB_TOKEN_BOT, scope: read}\n    - {name: ci, tokenEnv: BRAINDB_TOKEN_CI, scope: write, dbs: [sso]}\n"); len(errs) > 0 {
		t.Fatalf("a valid token block was rejected: %v", errs)
	}
	for _, tc := range []struct{ yaml, want string }{
		{"server:\n  tokens:\n    - {name: bot, tokenEnv: T, scope: nonsense}\n", "must be read, write or admin"},
		{"server:\n  tokens:\n    - {name: bot}\n", "required: name the environment variable"},
		{"server:\n  tokens:\n    - {tokenEnv: T, scope: read}\n", "name is required"},
		{"server:\n  tokens:\n    - {name: bot, tokenEnv: \"sk-live-2b9f00\", scope: read}\n", "must be the NAME of an environment variable"},
		{"server:\n  tokenEnv: \"sk-live-2b9f00\"\n", "must be the NAME of an environment variable"},
	} {
		_, errs := ParseConfig(tc.yaml)
		got := ""
		for _, e := range errs {
			got += e.Error() + "\n"
		}
		if !strings.Contains(got, tc.want) {
			t.Errorf("%q: want %q in:\n%s", tc.yaml, tc.want, got)
		}
	}
}

func TestConfigApply(t *testing.T) {
	cfg, errs := ParseConfig("goal: G\ncontext: {depth: 3, threshold: 0.2}\nkinds:\n  - {name: fact, halfLife: 45}\n  - {name: deal, halfLife: 21, prefix: g, section: pipeline}\nconnectors:\n  aliases:\n    - {label: ARR, terms: [annual recurring revenue]}\n")
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	b := New()
	if err := b.ApplyConfig(cfg, true); err != nil {
		t.Fatal(err)
	}
	if b.Goal != "G" || b.P.Depth != 3 || *b.kindM["fact"].HalfLife != 45 || b.kindM["deal"] == nil || len(b.connectors()) != 1 {
		t.Fatalf("apply: goal %q depth %d", b.Goal, b.P.Depth)
	}
}
