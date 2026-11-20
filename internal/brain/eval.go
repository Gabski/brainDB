// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

// Retrieval quality evaluation.
//
// Latency is the easy half of "is this database any good". The claim that needs evidence is that activation over the
// graph retrieves better than flat vector top-k at the same budget, and that the offline hash embedder is or is not
// good enough for real use. This file measures both, against a hand-written gold set of query → relevant ids.
//
// Everything here is deterministic: the same brain, gold set and engine give the same numbers every time.

// GoldQuery is one judged query.
type GoldQuery struct {
	Q string `json:"q"`
	// Bucket groups queries so that the report can separate, for example, queries that share words with the
	// target text from queries that only share meaning. That split is the whole point of the exercise.
	Bucket   string   `json:"bucket,omitempty"`
	Relevant []string `json:"relevant"`
	Note     string   `json:"note,omitempty"`
}

// GoldSet is a file of judged queries.
type GoldSet struct {
	Name    string      `json:"name,omitempty"`
	K       int         `json:"k,omitempty"`
	Modes   []string    `json:"modes,omitempty"`
	Queries []GoldQuery `json:"queries"`
}

// LoadGoldSet reads and checks a gold set. Every relevant id must exist in the brain: a typo in a gold file otherwise
// shows up as a retrieval failure, which is the most misleading bug a benchmark can have.
func (b *Brain) LoadGoldSet(path string) (*GoldSet, error) {
	d, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	g := &GoldSet{}
	if err := json.Unmarshal(d, g); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(g.Queries) == 0 {
		return nil, fmt.Errorf("%s: no queries", path)
	}
	if g.K <= 0 {
		g.K = 10
	}
	if len(g.Modes) == 0 {
		g.Modes = []string{"vector", "hybrid", "graph", "tree"}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var bad []string
	for i, q := range g.Queries {
		if strings.TrimSpace(q.Q) == "" {
			bad = append(bad, fmt.Sprintf("queries[%d]: empty query", i))
		}
		if len(q.Relevant) == 0 {
			bad = append(bad, fmt.Sprintf("queries[%d] %q: no relevant ids", i, q.Q))
		}
		for _, id := range q.Relevant {
			if b.nodes[id] == nil {
				bad = append(bad, fmt.Sprintf("queries[%d] %q: no such node %s", i, q.Q, id))
			}
		}
	}
	if len(bad) > 0 {
		return nil, fmt.Errorf("%s:\n  %s", path, strings.Join(bad, "\n  "))
	}
	return g, nil
}

// EvalScore holds the metrics of one (mode, bucket) cell.
type EvalScore struct {
	Mode    string  `json:"mode"`
	Bucket  string  `json:"bucket"`
	Queries int     `json:"queries"`
	RecallK float64 `json:"recall"`  // share of relevant items retrieved in the top K
	Recall5 float64 `json:"recall5"` // same, top 5
	MRR     float64 `json:"mrr"`     // mean reciprocal rank of the first relevant hit
	NDCG    float64 `json:"ndcg"`    // nDCG at K, binary gains
	Misses  int     `json:"misses"`  // queries with no relevant item anywhere in the top K
}

// EvalQuery is the per-query detail, so a bad number can be traced to the query that produced it.
type EvalQuery struct {
	Q        string   `json:"q"`
	Bucket   string   `json:"bucket,omitempty"`
	Mode     string   `json:"mode"`
	Relevant []string `json:"relevant"`
	Got      []string `json:"got"`
	FirstHit int      `json:"firstHit"` // 1-based rank of the first relevant hit, 0 = none
}

// EvalReport is the result of Evaluate.
type EvalReport struct {
	Name     string      `json:"name,omitempty"`
	Engine   string      `json:"engine"`
	Dim      int         `json:"dim"`
	K        int         `json:"k"`
	Nodes    int         `json:"nodes"`
	Queries  int         `json:"queries"`
	Buckets  []string    `json:"buckets"`
	Scores   []EvalScore `json:"scores"`
	Detail   []EvalQuery `json:"detail,omitempty"`
	Warnings []string    `json:"warnings,omitempty"`
}

// Evaluate runs every gold query through every mode and scores the ranking.
func (b *Brain) Evaluate(g *GoldSet, detail bool) (*EvalReport, error) {
	rep := &EvalReport{Name: g.Name, K: g.K, Engine: b.emb().Name(), Dim: b.emb().Dim(), Queries: len(g.Queries)}
	b.mu.Lock()
	rep.Nodes = len(b.nodes)
	b.mu.Unlock()

	buckets := []string{}
	seen := map[string]bool{}
	for _, q := range g.Queries {
		bk := q.Bucket
		if bk == "" {
			bk = "all"
		}
		if !seen[bk] {
			seen[bk], buckets = true, append(buckets, bk)
		}
	}
	sort.Strings(buckets)
	rep.Buckets = buckets

	type acc struct {
		n                         int
		recall, recall5, mrr, dcg float64
		misses                    int
	}
	cells := map[string]*acc{}
	key := func(mode, bucket string) string { return mode + "\x00" + bucket }

	for _, mode := range g.Modes {
		for _, q := range g.Queries {
			res, err := b.Search(q.Q, mode, g.K)
			if err != nil {
				return nil, fmt.Errorf("mode %s, query %q: %w", mode, q.Q, err)
			}
			got := make([]string, 0, len(res.Rows))
			for _, r := range res.Rows {
				got = append(got, r.ID)
			}
			rel := map[string]bool{}
			for _, id := range q.Relevant {
				rel[id] = true
			}
			hits, hits5, first, dcg := 0, 0, 0, 0.0
			for i, id := range got {
				if !rel[id] {
					continue
				}
				hits++
				if i < 5 {
					hits5++
				}
				if first == 0 {
					first = i + 1
				}
				dcg += 1 / math.Log2(float64(i)+2)
			}
			ideal := 0.0
			for i := 0; i < len(q.Relevant) && i < g.K; i++ {
				ideal += 1 / math.Log2(float64(i)+2)
			}
			bk := q.Bucket
			if bk == "" {
				bk = "all"
			}
			for _, k := range []string{key(mode, bk), key(mode, "*")} {
				a := cells[k]
				if a == nil {
					a = &acc{}
					cells[k] = a
				}
				a.n++
				a.recall += float64(hits) / float64(len(q.Relevant))
				a.recall5 += float64(hits5) / math.Min(float64(len(q.Relevant)), 5)
				if first > 0 {
					a.mrr += 1 / float64(first)
				} else {
					a.misses++
				}
				if ideal > 0 {
					a.dcg += dcg / ideal
				}
			}
			if detail {
				rep.Detail = append(rep.Detail, EvalQuery{Q: q.Q, Bucket: bk, Mode: mode, Relevant: q.Relevant, Got: got, FirstHit: first})
			}
		}
	}

	for _, mode := range g.Modes {
		for _, bk := range append(append([]string{}, buckets...), "*") {
			a := cells[key(mode, bk)]
			if a == nil || a.n == 0 {
				continue
			}
			n := float64(a.n)
			rep.Scores = append(rep.Scores, EvalScore{Mode: mode, Bucket: bk, Queries: a.n,
				RecallK: a.recall / n, Recall5: a.recall5 / n, MRR: a.mrr / n, NDCG: a.dcg / n, Misses: a.misses})
		}
	}
	if re, ok := b.emb().(*remoteEmbedder); ok {
		if d := re.Degraded(); d != "" {
			rep.Warnings = append(rep.Warnings, "embedder degraded to hash vectors: "+d+" — these numbers are not a measurement of "+re.o.Model)
		}
	}
	return rep, nil
}

// Markdown renders the report as the table that goes into docs/BENCHMARKS.md.
func (r *EvalReport) Markdown() string {
	var x strings.Builder
	fmt.Fprintf(&x, "Engine `%s` (dim %d) · %d nodes · %d queries · k = %d\n\n", r.Engine, r.Dim, r.Nodes, r.Queries, r.K)
	for _, w := range r.Warnings {
		fmt.Fprintf(&x, "> **Warning:** %s\n\n", w)
	}
	cols := append(append([]string{}, r.Buckets...), "*")
	for _, bk := range cols {
		label := bk
		if bk == "*" {
			label = "all queries"
		}
		fmt.Fprintf(&x, "**%s**\n\n", label)
		x.WriteString("| mode | recall@5 | recall@" + fmt.Sprint(r.K) + " | MRR | nDCG@" + fmt.Sprint(r.K) + " | queries with no hit |\n|---|---|---|---|---|---|\n")
		for _, s := range r.Scores {
			if s.Bucket != bk {
				continue
			}
			fmt.Fprintf(&x, "| `%s` | %.2f | %.2f | %.2f | %.2f | %d/%d |\n", s.Mode, s.Recall5, s.RecallK, s.MRR, s.NDCG, s.Misses, s.Queries)
		}
		x.WriteString("\n")
	}
	return x.String()
}
