// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

// Query pushdown (Addendum F4): narrow the candidate set with SQL over the node and field indexes
// instead of walking every node in memory.
//
// The pre-filter is an optimisation, never a decision. Whatever SQL returns is still run through the
// same in-memory predicate as before, so the only property the SQL has to hold is soundness: it may
// return rows that do not match, but it must never drop a row that does. Every condition below is
// written to that rule, and anything that cannot be translated soundly is simply not pushed down.

import (
	"sort"
)

// asciiFold reports whether s is pure ASCII. Case-insensitive matching in memory uses Unicode case
// folding, SQLite's NOCASE only folds ASCII; for an ASCII literal the two agree, because anything
// that folds equal to an ASCII string is itself ASCII.
func asciiFold(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// selectivityCap bounds a probe. A predicate that returns more than this is not worth routing through
// SQL: the measured crossover is well under it, and reading this many index entries costs microseconds.
const selectivityCap = 2000

// sqlCandidates returns the ids that could satisfy w, in brain order, or ok=false when the caller
// should scan as before.
//
// Only one predicate drives the lookup: the first field predicate whose index probe comes back under
// selectivityCap. That keeps the SQL to a bounded index range scan with no join over the node table
// and no ORDER BY, both of which made earlier versions sort the whole result before the cap could
// apply — and measured slower than the scan they replaced. The remaining predicates are left to the
// in-memory filter, which re-checks every candidate anyway.
func (b *Brain) sqlCandidates(w *Where, from string) ([]string, bool) {
	if !b.sqlReady() || w == nil || len(w.Fields) == 0 {
		return nil, false
	}
	for _, f := range w.Fields {
		cond, a, ok := fieldCond(f, "f")
		if !ok {
			continue // not translatable soundly
		}
		q := "SELECT f.node_id, n.ord FROM field f JOIN node n ON n.id = f.node_id WHERE f.name = ? COLLATE NOCASE AND " +
			cond + " LIMIT ?"
		args := append([]any{f.Name}, a...)
		args = append(args, selectivityCap+1)
		ids, ok := b.probe(q, args)
		if ok {
			return ids, true
		}
	}
	return nil, false
}

// probe runs one bounded candidate query and returns the ids in brain order. ok is false when the cap
// was reached, which means the predicate is not selective enough to be worth it.
func (b *Brain) probe(q string, args []any) ([]string, bool) {
	rows, err := b.sq.Query(q, args...)
	if err != nil {
		return nil, false // a broken plan must never fail a query: fall back to the scan
	}
	defer rows.Close()
	type hit struct {
		id  string
		ord int64
	}
	hits := []hit{}
	seen := map[string]bool{}
	for rows.Next() {
		var h hit
		if err := rows.Scan(&h.id, &h.ord); err != nil {
			return nil, false
		}
		if seen[h.id] {
			continue // the same node can carry a field name twice
		}
		seen[h.id] = true
		hits = append(hits, h)
		if len(hits) > selectivityCap {
			return nil, false
		}
	}
	if rows.Err() != nil {
		return nil, false
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].ord < hits[j].ord })
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.id
	}
	return out, true
}

// fieldCond translates one field predicate. Numeric comparisons keep rows whose value is not a number
// (field.num IS NULL): in memory those fall back to a string comparison, which can match, so dropping
// them here would hide rows.
func fieldCond(f FieldFilter, alias string) (string, []any, bool) {
	switch f.Op {
	case "=", ":":
		if !asciiFold(f.Value) {
			return "", nil, false
		}
		return alias + ".value = ? COLLATE NOCASE", []any{f.Value}, true
	case ">", ">=", "<", "<=":
		n, ok := parseNum(f.Value)
		if !ok {
			return "", nil, false
		}
		return "(" + alias + ".num " + f.Op + " ? OR " + alias + ".num IS NULL)", []any{n}, true
	}
	return "", nil, false
}

// PlanQuery describes how a query will be answered. It is what `braindb bench` and the stats endpoint
// report, and what makes the pushdown visible instead of a silent behaviour change.
type PlanQuery struct {
	Pushdown   bool `json:"pushdown"`
	Candidates int  `json:"candidates"`
	Total      int  `json:"total"`
}

// ExplainQuery reports whether a query can be narrowed by SQL and by how much.
func (b *Brain) ExplainQuery(q *Query) PlanQuery {
	b.mu.Lock()
	defer b.mu.Unlock()
	p := PlanQuery{Total: len(b.order)}
	if ids, ok := b.sqlCandidates(&q.Where, q.From); ok {
		p.Pushdown, p.Candidates = true, len(ids)
	} else {
		p.Candidates = len(b.order)
	}
	return p
}
