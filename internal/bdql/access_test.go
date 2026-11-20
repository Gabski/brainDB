// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package bdql

import (
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Gabski/brainDB/internal/brain"
)

const accessYAML = `access:
  enabled: true
  labels: [{name: finance}, {name: hr}, {name: secret}]
  roles:
    - {name: rep, grants: [{actions: [read, write], labels: ["*"]}, {deny: true, actions: [read, write], labels: [hr]}, {deny: true, actions: [read], labels: [finance], sections: [facts]}]}
    - {name: fin, grants: [{actions: [read], labels: [finance]}]}
    - {name: admin, grants: [{actions: [read, write, admin], labels: ["*"]}]}
  users:
    - {name: rep1, roles: [rep]}
    - {name: fin1, roles: [fin]}
    - {name: pub, roles: []}
    - {name: root, roles: [admin]}
`

var nowRe = regexp.MustCompile(`now="[^"]*"|step="\d+"|"step": \d+|step \d+|"now": "[^"]*"|> \d{4}-\d\d-\d\d \d\d:\d\d · step \d+`)

func randomBrain(t *testing.T, seed int64) *brain.Brain {
	rng := rand.New(rand.NewSource(seed))
	words := strings.Fields("renewal price discount margin forecast salary leave contract Halvorsen Ostrava invoice plant battery cycle lab result risk deadline")
	labels := [][]string{nil, nil, nil, {"finance"}, {"hr"}, {"secret"}, {"finance", "hr"}}
	secs := []string{"facts", "decisions", "pricing", "notes"}
	var sb strings.Builder
	sb.WriteString(`{"name":"R","goal":"random","concepts":[{"id":"a","label":"Alpha"},{"id":"b","label":"Beta"}],"memories":[`)
	n := 25
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		txt := []string{}
		for k := 0; k < 4; k++ {
			txt = append(txt, words[rng.Intn(len(words))])
		}
		lb := labels[rng.Intn(len(labels))]
		lj := `[]`
		if len(lb) > 0 {
			lj = `["` + strings.Join(lb, `","`) + `"]`
		}
		c := []string{"a", "b"}[rng.Intn(2)]
		fmt.Fprintf(&sb, `{"id":"m%d","kind":"fact","section":"%s","text":"%s %d","concepts":["%s"],"ageDays":%d,"labels":%s,"fields":{"value":"%d"}}`,
			i, secs[rng.Intn(len(secs))], strings.Join(txt, " "), i, c, rng.Intn(60), lj, rng.Intn(1000))
	}
	sb.WriteString(`],"links":[`)
	for i := 0; i < 30; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `["m%d","m%d",%.2f]`, rng.Intn(n), rng.Intn(n), .3+rng.Float64()*.6)
	}
	fmt.Fprintf(&sb, `],"focus":["m%d","m%d","m%d"]}`, rng.Intn(n), rng.Intn(n), rng.Intn(n))
	b := brain.New()
	b.Now = func() time.Time { return testNow }
	var s brain.Seed
	if err := jsonUnmarshal(sb.String(), &s); err != nil {
		t.Fatal(err)
	}
	if err := b.LoadSeed(&s); err != nil {
		t.Fatal(err)
	}
	cfg, errs := brain.ParseConfig(accessYAML)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	// normalise node order through XML so the independently rebuilt copy shares tie-breaking order
	if err := b.LoadXML(b.XML()); err != nil {
		t.Fatal(err)
	}
	if err := b.ApplyConfig(cfg, false); err != nil {
		t.Fatal(err)
	}
	return b
}

// deletedCopy builds, independently of ViewFor, the admin view of a copy with P-invisible nodes forgotten.
func deletedCopy(t *testing.T, b *brain.Brain, p *brain.Principal) *brain.Brain {
	c := brain.New()
	c.Now = b.Now
	if err := c.LoadXML(b.XML()); err != nil {
		t.Fatal(err)
	}
	ops := []brain.Op{}
	for _, id := range c.IDs() {
		ls, _ := c.LabelsOf(id)
		k, _ := c.Brief(id)
		sec := ""
		for _, n := range c.State().Nodes {
			if n.ID == id {
				sec = n.Section
			}
		}
		if !p.Visible(&brain.Node{ID: id, Kind: k, Section: sec, Labels: ls}, "test") {
			ops = append(ops, brain.Op{Op: "forget", ID: id, Force: true})
		}
	}
	if len(ops) > 0 {
		if _, err := c.Exec(ops); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

var readScript = `CONTEXT FORMAT xml;
CONTEXT FORMAT md;
CONTEXT FOR "renewal price" DEPTH 2 FORMAT json;
SEARCH "renewal discount margin" MODE hybrid LIMIT 8;
SEARCH "renewal discount margin" MODE vector LIMIT 8;
SEARCH "renewal discount margin" MODE tree LIMIT 8;
SEARCH "renewal discount margin" MODE graph LIMIT 8;
SELECT id, text, weight, activation FROM memory ORDER BY id;
SELECT section, COUNT(*), SUM(fields.value), MAX(fields.value) FROM memory GROUP BY section ORDER BY section;
SELECT id, text FROM memory NEAR "invoice salary" LIMIT 5;
SELECT id, degree FROM concepts ORDER BY id;
SHOW stats;
DISCOVER;
FILL $$
price: "price discount"
risk: "main risk"
$$ AS yaml OUTPUT json;
`

func TestAccessNonLeakageProperty(t *testing.T) {
	for seed := int64(1); seed <= 12; seed++ {
		b := randomBrain(t, seed)
		for _, user := range []string{"rep1", "fin1", "pub"} {
			p, err := b.PrincipalFor(user)
			if err != nil {
				t.Fatal(err)
			}
			full := brain.NewMemStore(b, "test")
			r1 := &Runner{Store: full, Principal: p, DB: "test"}
			res1, err := r1.Run(readScript)
			if err != nil {
				t.Fatalf("seed %d %s: %v", seed, user, err)
			}
			cp := deletedCopy(t, b, p)
			r2 := &Runner{Store: brain.NewMemStore(cp, "test"), DB: "test"}
			res2, err := r2.Run(readScript)
			if err != nil {
				t.Fatal(err)
			}
			a, c := nowRe.ReplaceAllString(res1.Render("json"), ""), nowRe.ReplaceAllString(res2.Render("json"), "")
			if a != c {
				t.Fatalf("seed %d user %s: view differs from the deleted copy\n--- view ---\n%s\n--- copy ---\n%s", seed, user, a, c)
			}
			// targeted APIs
			v := b.ViewFor(p, "test")
			for _, id := range cp.IDs() {
				if !v.Has(id) {
					t.Fatalf("visible %s missing from view", id)
				}
			}
			for _, id := range b.IDs() {
				if !cp.Has(id) {
					if _, err := r1.Run("INSPECT " + id + ";"); err == nil || !strings.Contains(err.Error(), "no such item "+id) {
						t.Fatalf("invisible %s must look nonexistent: %v", id, err)
					}
					if _, err := r1.Run("BOOST " + id + ";"); err == nil || !strings.Contains(err.Error(), "no such item "+id) {
						t.Fatalf("writes to invisible %s must say no such item: %v", id, err)
					}
				}
			}
		}
	}
}

func TestAccessWriteRules(t *testing.T) {
	b := randomBrain(t, 99)
	st := brain.NewMemStore(b, "test")
	rep, _ := b.PrincipalFor("rep1")
	fin, _ := b.PrincipalFor("fin1")
	run := func(p *brain.Principal, s string) error {
		_, err := (&Runner{Store: st, Principal: p, DB: "test"}).Run(s)
		return err
	}
	if err := run(rep, `REMEMBER fact "rep note" LABELS (hr);`); err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("deny overrides allow for writes: %v", err)
	}
	if err := run(rep, `REMEMBER fact "rep note" IN notes LABELS (finance);`); err != nil {
		t.Fatalf("rep may write finance outside the denied section: %v", err)
	}
	if err := run(fin, `REMEMBER fact "x";`); err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("read-only role cannot write: %v", err)
	}
	if err := run(rep, `CREATE USER eve ROLES (rep);`); err == nil || !strings.Contains(err.Error(), "admin") {
		t.Fatalf("user management needs admin: %v", err)
	}
	if err := run(rep, `AS USER fin1;`); err == nil {
		t.Fatal("AS USER needs admin")
	}
	// admin previews another view; AS USER stays in effect until the next AS USER
	res, err := (&Runner{Store: st, DB: "test"}).Run(`AS USER pub; SELECT COUNT(*) FROM memory; AS USER admin; SELECT COUNT(*) FROM memory;`)
	if err != nil {
		t.Fatal(err)
	}
	if res.Results[1].Rows[0].Vals["COUNT(*)"] == res.Results[3].Rows[0].Vals["COUNT(*)"] {
		t.Fatal("pub must see fewer memories than admin")
	}
	// explain access
	res, err = (&Runner{Store: st, DB: "test"}).Run(`LET $h = (SELECT id FROM memory WHERE labels ~ 'hr' LIMIT 1); EXPLAIN ACCESS $h FOR rep1;`)
	if err != nil || !strings.Contains(res.Render("md"), "hidden") || !strings.Contains(res.Render("md"), "deny") {
		t.Fatalf("explain access: %v %s", err, res.Render("md"))
	}
	// LABEL statement and labels column
	if _, err := (&Runner{Store: st, DB: "test"}).Run(`LABEL m1 ADD secret; SELECT labels FROM memory WHERE id = m1;`); err != nil {
		t.Fatal(err)
	}
	ls, _ := b.LabelsOf("m1")
	if !strings.Contains(strings.Join(ls, ","), "secret") {
		t.Fatalf("LABEL ADD: %v", ls)
	}
}

func jsonUnmarshal(s string, v any) error { return jsonStd([]byte(s), v) }
