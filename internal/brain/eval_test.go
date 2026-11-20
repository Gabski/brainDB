// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func writeGold(t *testing.T, g GoldSet) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "gold.json")
	d, _ := json.Marshal(g)
	if err := os.WriteFile(p, d, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// A gold set with a typo in an id is the most dangerous bug a benchmark can have: it looks like a retrieval failure
// and quietly lowers every score. It must be rejected at load time.
func TestGoldSetRejectsUnknownIds(t *testing.T) {
	b := seeded(t)
	for _, g := range []GoldSet{
		{Queries: []GoldQuery{{Q: "anything", Relevant: []string{"nosuchnode"}}}},
		{Queries: []GoldQuery{{Q: "anything", Relevant: nil}}},
		{Queries: []GoldQuery{{Q: "   ", Relevant: []string{"f101"}}}},
		{Queries: nil},
	} {
		if _, err := b.LoadGoldSet(writeGold(t, g)); err == nil {
			t.Fatalf("expected a validation error for %+v", g)
		}
	}
}

// A query that is the exact text of a memory must rank that memory first, in every mode. If this fails the
// evaluation harness is measuring something other than retrieval.
func TestEvaluateScoresAPerfectQuery(t *testing.T) {
	b := seeded(t)
	id, text := "", ""
	b.mu.Lock()
	for _, x := range b.order {
		if n := b.nodes[x]; n.Kind == "fact" && len(n.Text) > 40 {
			id, text = x, n.Text
			break
		}
	}
	b.mu.Unlock()
	if id == "" {
		t.Skip("no suitable fact in the fixture")
	}
	p := writeGold(t, GoldSet{K: 10, Modes: []string{"vector", "hybrid", "graph", "tree"},
		Queries: []GoldQuery{{Q: text, Bucket: "self", Relevant: []string{id}}}})
	g, err := b.LoadGoldSet(p)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := b.Evaluate(g, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range rep.Detail {
		if d.FirstHit != 1 {
			t.Errorf("mode %s: a memory must rank first for its own text, got rank %d (%v)", d.Mode, d.FirstHit, d.Got)
		}
	}
	for _, s := range rep.Scores {
		if s.Bucket != "self" {
			continue
		}
		if math.Abs(s.MRR-1) > 1e-9 || math.Abs(s.RecallK-1) > 1e-9 || math.Abs(s.NDCG-1) > 1e-9 || s.Misses != 0 {
			t.Errorf("mode %s: want perfect scores, got %+v", s.Mode, s)
		}
	}
}

// Every metric is a ratio and must stay inside [0,1], including when a query has more relevant items than k.
func TestEvaluateMetricsAreBounded(t *testing.T) {
	b := seeded(t)
	ids := []string{}
	b.mu.Lock()
	for _, x := range b.order {
		if n := b.nodes[x]; n.Kind != Concept && n.Conn == nil {
			ids = append(ids, x)
		}
		if len(ids) == 12 {
			break
		}
	}
	b.mu.Unlock()
	p := writeGold(t, GoldSet{K: 5, Modes: []string{"vector", "hybrid", "graph", "tree"}, Queries: []GoldQuery{
		{Q: "renewal discount pricing", Bucket: "wide", Relevant: ids},
		{Q: "zzzz qqqq xxxx no such words anywhere", Bucket: "impossible", Relevant: ids[:1]},
	}})
	g, err := b.LoadGoldSet(p)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := b.Evaluate(g, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Scores) == 0 {
		t.Fatal("no scores")
	}
	for _, s := range rep.Scores {
		for name, v := range map[string]float64{"recall5": s.Recall5, "recall": s.RecallK, "mrr": s.MRR, "ndcg": s.NDCG} {
			if v < 0 || v > 1 || math.IsNaN(v) {
				t.Errorf("%s/%s: %s = %v, must be in [0,1]", s.Mode, s.Bucket, name, v)
			}
		}
		if s.Misses < 0 || s.Misses > s.Queries {
			t.Errorf("%s/%s: misses %d of %d", s.Mode, s.Bucket, s.Misses, s.Queries)
		}
	}
}

// The shipped gold set must stay in sync with the shipped seed: an id removed from seed.json has to fail here and
// not silently become a permanent miss in the published benchmark.
func TestShippedGoldSetMatchesSeed(t *testing.T) {
	seed := filepath.Join("..", "..", "examples", "sales-company", "seed.json")
	gold := filepath.Join("..", "..", "examples", "sales-company", "eval.json")
	if _, err := os.Stat(gold); err != nil {
		t.Skip("no shipped gold set")
	}
	b := New()
	if err := b.LoadSeedFile(seed); err != nil {
		t.Fatal(err)
	}
	g, err := b.LoadGoldSet(gold)
	if err != nil {
		t.Fatal(err)
	}
	lex, par := 0, 0
	for _, q := range g.Queries {
		switch q.Bucket {
		case "lexical":
			lex++
		case "paraphrase":
			par++
		default:
			t.Errorf("query %q: bucket must be lexical or paraphrase, got %q", q.Q, q.Bucket)
		}
	}
	if lex < 10 || par < 10 {
		t.Fatalf("both buckets need at least 10 queries, got lexical=%d paraphrase=%d", lex, par)
	}
}
