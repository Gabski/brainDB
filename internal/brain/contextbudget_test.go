// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"strings"
	"testing"
	"time"
)

func budgetBrain(t *testing.T) *Brain {
	b := New()
	b.Now = func() time.Time { return time.Date(2026, 11, 20, 9, 30, 0, 0, time.UTC) }
	if err := b.LoadSeedFile("testdata/parity_seed.json"); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestContextBudgetTrimsLeastActiveAndSaysSo(t *testing.T) {
	b := budgetBrain(t)
	for _, format := range []string{"xml", "md"} {
		full, fit := b.ContextBudget(format, 0)
		if fit.Trimmed || fit.Shown != fit.Lit || fit.Lit < 6 {
			t.Fatalf("%s: no budget means the whole slice (%+v)", format, fit)
		}
		limit := fit.Tokens / 2
		out, cut := b.ContextBudget(format, limit)
		if !cut.Trimmed || cut.Shown >= cut.Lit {
			t.Fatalf("%s: half the budget must drop memories (%+v)", format, cut)
		}
		if cut.Tokens > limit+60 { // the omission note itself is not counted in the search
			t.Fatalf("%s: %d tokens for a budget of %d", format, cut.Tokens, limit)
		}
		if len(out) >= len(full) || !strings.Contains(out, "trimmed to fit max_tokens") {
			t.Fatalf("%s: the reply must say it was cut", format)
		}
		if format == "xml" && !strings.HasSuffix(strings.TrimSpace(out), "</context>") {
			t.Fatal("xml must stay well-formed")
		}
		for _, id := range b.Focus() {
			if !strings.Contains(out, id) {
				t.Fatalf("%s: focus %s must never be dropped", format, id)
			}
		}
		if _, big := b.ContextBudget(format, fit.Tokens+10); big.Trimmed {
			t.Fatalf("%s: a budget that fits must not trim", format)
		}
	}
}

func TestWarnTokensFromConfig(t *testing.T) {
	b := budgetBrain(t)
	if strings.Contains(b.Status(""), "warn: context") {
		t.Skip("seed slice is already over 4000 tokens")
	}
	b.WarnTokens = 1
	if !strings.Contains(b.Status(""), "warn: context > 1 tokens") {
		t.Fatalf("budget.warnTokens must drive the warning: %s", b.Status(""))
	}
}
