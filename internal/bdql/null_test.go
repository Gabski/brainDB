// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package bdql

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Gabski/brainDB/internal/brain"
)

// count runs a single COUNT(*) statement and returns the number it reported.
func count(t *testing.T, where string) int {
	t.Helper()
	st := fixture(t)
	script := fmt.Sprintf("SELECT COUNT(*) FROM memory WHERE %s;", where)
	r, err := run(t, st, script, true)
	if err != nil {
		t.Fatalf("%s: %v", script, err)
	}
	rows := r.Results[0].Rows
	if len(rows) != 1 {
		t.Fatalf("%s: want one row, got %d", script, len(rows))
	}
	v, ok := rows[0].Vals["COUNT(*)"]
	if !ok {
		t.Fatalf("%s: no COUNT(*) column in %v", script, rows[0].Keys)
	}
	n, ok := v.(int)
	if !ok {
		t.Fatalf("%s: COUNT(*) is %T (%v), want int", script, v, v)
	}
	return n
}

// The fixture holds 10 memories; only the 3 deals carry fields.close_date
// (2026-11-30, 2026-12-15, 2027-01-20). The other 7 have no such field.
func TestMissingValueMatchesNoComparison(t *testing.T) {
	for _, c := range []struct {
		where string
		want  int
	}{
		// A node without the field must not satisfy any comparison. Before the NULL rule the
		// empty string stood in for the missing value, so every "<" matched all 7 of them.
		{"fields.close_date < '1900-01-01'", 0},
		{"fields.close_date <= '1900-01-01'", 0},
		{"fields.close_date > '2100-01-01'", 0},
		{"fields.close_date >= '2100-01-01'", 0},
		{"fields.close_date = '1900-01-01'", 0},
		{"fields.close_date != '1900-01-01'", 3},
		{"fields.close_date IN ('1900-01-01')", 0},
		{"fields.close_date NOT IN ('1900-01-01')", 3},
		{"fields.close_date BETWEEN '1800-01-01' AND '1900-01-01'", 0},
		// Real values still compare, and the two halves partition the 3 nodes that have a date.
		{"fields.close_date < '2026-12-01'", 1},
		{"fields.close_date > '2026-12-01'", 2},
		{"fields.close_date BETWEEN '2026-01-01' AND '2026-12-31'", 2},
		{"fields.close_date != ''", 3},
		// Numbers take the same path: "below zero" must not mean "has no value".
		{"fields.value < 0", 0},
		{"fields.value > 10000", 2},
	} {
		if got := count(t, c.where); got != c.want {
			t.Errorf("WHERE %s = %d, want %d", c.where, got, c.want)
		}
	}
}

func TestUnknownColumnIsAnError(t *testing.T) {
	for _, script := range []string{
		"SELECT id, banana FROM memory;",
		"SELECT id FROM memory WHERE banana < '2026-11-01';",
		"SELECT id FROM memory WHERE fields.banana < '2026-11-01';",
		"SELECT id FROM memory ORDER BY banana DESC;",
	} {
		st := fixture(t)
		_, err := run(t, st, script, true)
		if err == nil {
			t.Errorf("%s: want an error, got none", script)
			continue
		}
		if !strings.Contains(err.Error(), "unknown column") || !strings.Contains(err.Error(), "banana") {
			t.Errorf("%s: want an unknown-column error naming banana, got %v", script, err)
		}
	}
}

// A real column must stay valid however the conditions are ordered. Short-circuit evaluation
// visits different rows depending on that order, so the verdict may not be drawn from the rows
// that happened to be probed: here no fact carries close_date, yet the deals do.
func TestKnownColumnNeverDependsOnConditionOrder(t *testing.T) {
	for _, script := range []string{
		"SELECT id FROM memory WHERE kind = fact AND fields.close_date > '2020-01-01';",
		"SELECT id FROM memory WHERE fields.close_date > '2020-01-01' AND kind = fact;",
	} {
		st := fixture(t)
		r, err := run(t, st, script, true)
		if err != nil {
			t.Fatalf("%s: %v", script, err)
		}
		if n := len(r.Results[0].Rows); n != 0 {
			t.Errorf("%s: got %d rows, want 0", script, n)
		}
	}
}

// touched is the absolute last-touch timestamp the XML carries; without it the brain can only be
// queried by relative age.
func TestTouchedColumn(t *testing.T) {
	st := fixture(t)
	r, err := run(t, st, "SELECT id, touched FROM memory ORDER BY touched DESC;", true)
	if err != nil {
		t.Fatal(err)
	}
	rows := r.Results[0].Rows
	if len(rows) != 10 {
		t.Fatalf("got %d rows, want 10", len(rows))
	}
	prev := ""
	for i, row := range rows {
		s, ok := row.Vals["touched"].(string)
		if !ok || !strings.HasSuffix(s, "Z") {
			t.Fatalf("row %d: touched is %v, want an ISO-8601 timestamp", i, row.Vals["touched"])
		}
		if prev != "" && s > prev {
			t.Errorf("row %d: %s is newer than the row before it (%s), so ORDER BY touched DESC did not sort", i, s, prev)
		}
		prev = s
	}
	// The fixture's clock is fixed, so every node was touched before this date and none after it.
	if got := count(t, "touched < '2100-01-01'"); got != 10 {
		t.Errorf("touched < 2100 = %d, want 10", got)
	}
	if got := count(t, "touched > '2100-01-01'"); got != 0 {
		t.Errorf("touched > 2100 = %d, want 0", got)
	}
}

// The purpose preview has to predict what a write will score, or the operator reviews one number and
// the brain stores another. A plain REMEMBER has no links when it is scored; one with LINK TO does,
// so the preview needs to be told about them.
func TestPurposePreviewMatchesTheWrite(t *testing.T) {
	st := fixture(t)
	p := &brain.Purpose{Name: "unrelated", Description: "submarine hull welding tolerances and sonar calibration logs"}
	accept, review := 0.99, 0.0
	p.Admission.Mode, p.Admission.Accept, p.Admission.Review = "enforce", &accept, &review
	p.Admission.Apply = []string{"remember"}
	st.B.SetPurpose(p)

	scoreOf := func(text string) (float64, bool) {
		for _, it := range st.B.QuarantineList() {
			if it.Text == text {
				return it.Score, true
			}
		}
		return 0, false
	}

	// Without links the preview and the write have to agree exactly.
	const plain = "quarterly cake rota for the office kitchen"
	preview := st.B.CheckPurpose(plain, "note", "notes", nil)
	if _, err := run(t, st, `REMEMBER note "`+plain+`" IN notes;`, false); err != nil {
		t.Fatal(err)
	}
	got, ok := scoreOf(plain)
	if !ok {
		t.Fatalf("the item was not quarantined: %+v", st.B.QuarantineList())
	}
	if diff := got - preview.Score; diff > 0.0001 || diff < -0.0001 {
		t.Errorf("preview scored %.4f, the write scored %.4f", preview.Score, got)
	}

	// With links a bare preview underestimates; the hinted one matches what the write records.
	const linked = "annual office summer picnic seating plan"
	bare := st.B.CheckPurpose(linked, "note", "notes", nil)
	hinted := st.B.CheckPurposeWith(linked, "note", "notes", nil, []string{"f101", "f102"}, 0)
	if hinted.Score <= bare.Score {
		t.Errorf("link hints did not raise the score: bare %.4f, hinted %.4f", bare.Score, hinted.Score)
	}
	if _, err := run(t, st, `REMEMBER note "`+linked+`" IN notes LINK TO f101, f102;`, false); err != nil {
		t.Fatal(err)
	}
	got, ok = scoreOf(linked)
	if !ok {
		t.Fatalf("the linked item was not quarantined: %+v", st.B.QuarantineList())
	}
	if diff := got - hinted.Score; diff > 0.0001 || diff < -0.0001 {
		t.Errorf("hinted preview %.4f, the write scored %.4f", hinted.Score, got)
	}
}
