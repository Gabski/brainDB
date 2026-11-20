// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExampleConfigsParse keeps the shipped examples honest: every braindb.yaml under examples/ must validate against
// the current schema, so a config key renamed in code cannot quietly break the examples people copy from.
func TestExampleConfigsParse(t *testing.T) {
	ms, err := filepath.Glob(filepath.Join("..", "..", "examples", "*", "braindb.yaml"))
	if err != nil || len(ms) == 0 {
		t.Skip("no example configs")
	}
	for _, p := range ms {
		d, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		cfg, errs := ParseConfig(string(d))
		if len(errs) > 0 {
			t.Errorf("%s: %v", p, errs[0])
			continue
		}
		if cfg.Purpose != nil && cfg.Purpose.mode() == "enforce" {
			t.Errorf("%s: examples must not ship purpose admission in enforce mode", p)
		}
	}
}

// TestPurposeEmptyForbiddenEntities pins the meaning of an empty purpose.outOfScope.entities list: it forbids
// nothing. patterns.ExtractTypes treats an empty type list as "every type", so without the guard in checkPurpose
// every item carrying a date, an amount or a version was rejected outright — in advise mode as well, because a
// forbidden entity bypasses the advise branch. Every shipped example happens to set `entities: [card]`, which is
// why this never surfaced there.
func TestPurposeEmptyForbiddenEntities(t *testing.T) {
	b := New()
	p := &Purpose{Name: "test", Description: "project decisions, releases and measurements"}
	p.InScope.Topics = []string{"release decisions", "measurements"}
	p.Admission.Mode = "advise"
	b.purpose = p

	withEntity := "0.0.3 ships on 2026-12-01 and costs EUR 0"
	forbidden := func(c PurposeCheck) bool {
		for _, r := range c.Rules {
			if strings.HasPrefix(r, "forbidden entity") {
				return true
			}
		}
		return false
	}

	if c := b.checkPurpose(withEntity, "decision", "decisions", 0, nil); forbidden(c) {
		t.Errorf("empty outOfScope.entities must forbid nothing, got %v", c.Rules)
	}

	p.OutOfScope.Entities = []string{"card"}
	if c := b.checkPurpose(withEntity, "decision", "decisions", 0, nil); forbidden(c) {
		t.Errorf("a version is not a card, got %v", c.Rules)
	}
	if c := b.checkPurpose("pay with 4111 1111 1111 1111", "decision", "decisions", 0, nil); !forbidden(c) {
		t.Errorf("a listed forbidden entity must still reject, got %q %v", c.Decision, c.Rules)
	}
}

// TestClauseSplitKeepsDottedTokens pins the clause splitter used by template fill: a full stop ends a clause only
// when whitespace follows. Without that rule "brain.xml", "0.0.2" and "MRR 0.57" were cut in half, and a filled
// template reported values such as "A brain is stored today as one brain".
func TestClauseSplitKeepsDottedTokens(t *testing.T) {
	text := "A brain is stored as one brain.xml file. Graph MRR is 0.57 on version 0.0.2; done"
	got := clauseSplit.Split(text, -1)
	want := []string{"A brain is stored as one brain.xml file", "Graph MRR is 0.57 on version 0.0.2", "done"}
	if len(got) != len(want) {
		t.Fatalf("got %d clauses %q, want %d", len(got), got, len(want))
	}
	for i := range want {
		if strings.TrimSpace(got[i]) != want[i] {
			t.Errorf("clause %d: got %q, want %q", i, got[i], want[i])
		}
	}
}
