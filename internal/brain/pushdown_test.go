// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// pushdownBrain builds a brain with the shapes the pre-filter has to get right: mixed kinds and
// sections, numeric and non-numeric values under the same field name, mixed case, an accented value
// and a field only some nodes carry.
func pushdownBrain(t *testing.T) (*Brain, func()) {
	t.Helper()
	b := New()
	b.Now = func() time.Time { return time.Date(2026, 11, 20, 9, 30, 0, 0, time.UTC) }
	mk := func(kind, section, text string, f Fields) {
		t.Helper()
		if _, err := b.Exec([]Op{{Op: "remember", Kind: kind, Section: section, Text: sp(text), Fields: f}}); err != nil {
			t.Fatal(err)
		}
	}
	mk("fact", "facts", "renewal risk at Halvorsen", Fields{{"stage", "won"}, {"value", "48000"}})
	mk("fact", "facts", "pricing policy update", Fields{{"stage", "WON"}, {"value", "1200"}})
	mk("note", "notes", "a note with no fields at all", nil)
	mk("note", "notes", "discount approved", Fields{{"stage", "lost"}, {"value", "n/a"}})
	mk("decision", "decisions", "go ahead", Fields{{"stage", "proposal"}, {"value", "€9,000"}})
	mk("task", "tasks", "call the customer", Fields{{"owner", "Kim"}})
	mk("fact", "pricing", "margin floor", Fields{{"stage", "won"}, {"value", "0"}})
	mk("fact", "facts", "zażółć gęślą jaźń", Fields{{"owner", "Żółw"}, {"value", "5"}})

	dir := t.TempDir()
	db, err := OpenSQLiteDB(filepath.Join(dir, "brain.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := b.WriteSQLite(db); err != nil {
		t.Fatal(err)
	}
	return b, func() { db.Close() }
}

func runQ(t *testing.T, b *Brain, src string) string {
	t.Helper()
	q, err := ParseQuery(src)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	r, err := b.RunQuery(q, "json")
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	d, _ := json.Marshal(r.Rows)
	return string(d)
}

// The pre-filter may change how many nodes are looked at, never which rows come back.
func TestPushdownMatchesTheScanExactly(t *testing.T) {
	b, done := pushdownBrain(t)
	defer done()

	queries := []string{
		"kind:fact limit:50",
		"kind:fact,note limit:50",
		"section:facts limit:50",
		"section:notes,tasks limit:50",
		"stage=won limit:50",
		"stage=WON limit:50",
		"stage=lost limit:50",
		"value>1000 limit:50",
		"value>=1200 limit:50",
		"value<1000 limit:50",
		"value<=0 limit:50",
		"owner=Kim limit:50",
		"owner=kim limit:50",
		"owner=Żółw limit:50",
		"kind:fact stage=won limit:50",
		"kind:fact stage=won value>10000 limit:50",
		"kind:note value>1 limit:50",
		"stage=nothing limit:50",
		"limit:50",
		"kind:fact sort:-weight limit:3",
	}
	for _, src := range queries {
		if !b.sqlReady() {
			t.Fatal("the database handle went stale before the comparison")
		}
		withPushdown := runQ(t, b, src)

		db := b.sq
		b.sq = nil // force the in-memory scan
		scan := runQ(t, b, src)
		b.sq = db

		if withPushdown != scan {
			t.Errorf("%s:\n  pushdown %s\n  scan     %s", src, withPushdown, scan)
		}
	}
}

// A non-numeric value under a numeric predicate still reaches the in-memory comparison: dropping it
// in SQL (field.num is NULL for it) would hide a row the scan returns.
func TestPushdownKeepsNonNumericValues(t *testing.T) {
	b, done := pushdownBrain(t)
	defer done()
	ids, ok := b.sqlCandidates(&Where{Fields: []FieldFilter{{Name: "value", Op: ">", Value: "1000"}}}, "memory")
	if !ok {
		t.Fatal("the predicate was not pushed down")
	}
	found := false
	for _, id := range ids {
		if v, _ := b.nodes[id].Fields.Get("value"); v == "n/a" {
			found = true
		}
	}
	if !found {
		t.Error("the candidate set dropped a value SQL cannot compare; the scan would have kept it")
	}
}

// The pre-filter must stand down the moment memory and the database can disagree.
func TestPushdownStopsWhenTheBrainChanges(t *testing.T) {
	b, done := pushdownBrain(t)
	defer done()
	sel := &Where{Fields: []FieldFilter{{Name: "stage", Op: "=", Value: "lost"}}}
	if _, ok := b.sqlCandidates(sel, "memory"); !ok {
		t.Fatal("a saved brain should push down a selective field predicate")
	}
	if _, err := b.Exec([]Op{{Op: "remember", Kind: "fact", Text: sp("written after the save")}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := b.sqlCandidates(sel, "memory"); ok {
		t.Error("the pre-filter used a database that no longer matches memory")
	}
	// And the query still finds the new node, through the scan.
	if got := runQ(t, b, "kind:fact limit:50"); !contains([]string{got}, got) || len(got) == 0 {
		t.Fatal("unexpected")
	}
	q, _ := ParseQuery("text:written limit:50")
	r, err := b.RunQuery(q, "json")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Rows) != 1 {
		t.Errorf("the unsaved node is not findable: %d rows", len(r.Rows))
	}
}

func TestExplainQueryReportsTheNarrowing(t *testing.T) {
	b, done := pushdownBrain(t)
	defer done()
	q, _ := ParseQuery("stage=lost")
	p := b.ExplainQuery(q)
	if !p.Pushdown {
		t.Fatal("want a pushdown plan for a field predicate")
	}
	if p.Candidates >= p.Total {
		t.Errorf("narrowing did nothing: %d of %d", p.Candidates, p.Total)
	}
	for _, src := range []string{"limit:5", "kind:fact"} {
		// Only field predicates are routed through the index: a kind filter matches a large part of
		// the brain, and measured slower through SQL than in memory.
		q, _ = ParseQuery(src)
		if p := b.ExplainQuery(q); p.Pushdown {
			t.Errorf("%s should not claim a pushdown", src)
		}
	}
	_ = fmt.Sprint()
}

// The cheap pre-check in maskText must never let a card through. It is an optimisation in a privacy
// path, which is the one place where being fast is worth nothing if it is wrong.
func TestCardMaskingSurvivesTheFastPath(t *testing.T) {
	b := New()
	cards := []string{
		"4111 1111 1111 1111",
		"4111-1111-1111-1111",
		"4111111111111111",
		"pay with 378282246310005 today",     // Amex, 15 digits
		"ref 4111 1111 1111 1111, then more", // inside a sentence
	}
	for _, c := range cards {
		if !mayHoldCard(c) {
			t.Errorf("the pre-check skipped %q", c)
		}
		if got := b.maskText(c); got == c {
			t.Errorf("card not masked: %q", got)
		}
	}
	// And text that cannot hold a card is returned untouched, without running the pattern library.
	for _, s := range []string{
		"id f101 weight 0.42 activation 0.1",
		"2026-11-20 09:30 · 48000 EUR · 12 items",
		"",
		"no digits here at all",
	} {
		if mayHoldCard(s) {
			t.Errorf("the pre-check claimed a card in %q", s)
		}
		if got := b.maskText(s); got != s {
			t.Errorf("maskText changed %q to %q", s, got)
		}
	}
	// With a configured type the full path still runs.
	b.Privacy = []string{"email"}
	if got := b.maskText("write to kim@halvorsen.example"); got == "write to kim@halvorsen.example" {
		t.Error("a configured PII type was not masked")
	}
}

// At scale the pre-filter has to be both correct and worth having. This builds brains far larger than
// the other tests, so it only runs with -test.short=false.
func TestPushdownAtScale(t *testing.T) {
	if testing.Short() {
		t.Skip("builds brains of up to 100k nodes")
	}
	for _, n := range []int{25000, 100000} {
		b := New()
		b.Now = func() time.Time { return time.Date(2026, 11, 20, 9, 30, 0, 0, time.UTC) }
		for i := 0; i < n; i++ {
			kind := "fact"
			var f Fields
			if i%3 == 0 {
				kind = "record"
				f = Fields{{"stage", []string{"won", "lost", "proposal"}[i%3]}, {"value", fmt.Sprintf("%d", 1000+i)}}
			}
			b.addNode(&Node{ID: fmt.Sprintf("n%d", i), Kind: kind, Section: "facts",
				Text: fmt.Sprintf("renewal pricing for account %d", i%500), Base: .5, Fields: f})
		}
		dir := t.TempDir()
		db, err := OpenSQLiteDB(filepath.Join(dir, "b.db"))
		if err != nil {
			t.Fatal(err)
		}
		if err := b.WriteSQLite(db); err != nil {
			t.Fatal(err)
		}
		for _, src := range []string{
			"kind:record stage=won limit:50", // unselective: expected to fall back to the scan
			"value=1000 limit:50",            // one match in the whole brain
			"stage=won value>299000 limit:50",
		} {
			q, _ := ParseQuery(src)
			t0 := time.Now()
			rp, err := b.RunQuery(q, "json")
			if err != nil {
				t.Fatal(err)
			}
			pushdown := time.Since(t0)

			keep := b.sq
			b.sq = nil
			t0 = time.Now()
			rs, err := b.RunQuery(q, "json")
			if err != nil {
				t.Fatal(err)
			}
			scan := time.Since(t0)
			b.sq = keep

			// The whole point: identical answers, including the exact match count.
			if len(rp.Rows) != len(rs.Rows) || rp.Total != rs.Total {
				t.Errorf("n=%d %q: pushdown %d rows / total %d, scan %d rows / total %d",
					n, src, len(rp.Rows), rp.Total, len(rs.Rows), rs.Total)
			}
			// And it must never make a query materially slower; an unselective predicate falls back.
			if pushdown > scan*2 {
				t.Errorf("n=%d %q: pushdown %s is more than twice the scan %s", n, src, pushdown, scan)
			}
			t.Logf("n=%-7d %-34s scan=%-10s pushdown=%-10s rows=%d total=%d",
				n, src, scan.Round(time.Microsecond), pushdown.Round(time.Microsecond), len(rp.Rows), rp.Total)
		}
		db.Close()
	}
}
