// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"strings"
	"testing"
)

const spliceSrc = `# brainDB config for the sales team
name: sales-company          # the database name
description: |
  A folded note
  over two lines
patterns:
  # which built-ins to switch off
  disabled: [card]
  custom:
    - {name: sku, type: id_code, regex: '\bSKU-\d{5}\b'}
  fx: {base: EUR}
privacy:
  mask: [phone, iban]        # masked everywhere
`

func TestSpliceKeepsCommentsAndFormatting(t *testing.T) {
	out := SpliceYAML(spliceSrc, []string{"patterns", "disabled"}, []any{"card", "iban"})
	for _, keep := range []string{
		"# brainDB config for the sales team",
		"name: sales-company          # the database name",
		"  A folded note",
		"  # which built-ins to switch off",
		`    - {name: sku, type: id_code, regex: '\bSKU-\d{5}\b'}`,
		"  fx: {base: EUR}",
		"  mask: [phone, iban]        # masked everywhere",
	} {
		if !strings.Contains(out, keep) {
			t.Errorf("lost %q from the document:\n%s", keep, out)
		}
	}
	if strings.Contains(out, "disabled: [card]\n") {
		t.Errorf("old value still present:\n%s", out)
	}
	cfg, errs := ParseConfig(out)
	if len(errs) > 0 {
		t.Fatalf("result does not parse: %v\n%s", errs, out)
	}
	if got := strings.Join(cfg.Patterns.Disabled, ","); got != "card,iban" {
		t.Errorf("disabled = %q, want \"card,iban\"", got)
	}
}

func TestSpliceReplacesAListBlock(t *testing.T) {
	// A sequence written with dashes under the key, at the key's own indentation.
	src := "patterns:\n  disabled:\n  - card\n  - iban\n  fx: {base: EUR}\nname: x\n"
	out := SpliceYAML(src, []string{"patterns", "disabled"}, []any{"vat"})
	if strings.Contains(out, "card") || strings.Contains(out, "iban") {
		t.Errorf("old list items survived:\n%s", out)
	}
	for _, keep := range []string{"  fx: {base: EUR}", "name: x"} {
		if !strings.Contains(out, keep) {
			t.Errorf("lost %q:\n%s", keep, out)
		}
	}
	cfg, errs := ParseConfig(out)
	if len(errs) > 0 {
		t.Fatalf("result does not parse: %v\n%s", errs, out)
	}
	if strings.Join(cfg.Patterns.Disabled, ",") != "vat" {
		t.Errorf("disabled = %v\n%s", cfg.Patterns.Disabled, out)
	}
}

func TestSpliceAddsMissingKeyAndParent(t *testing.T) {
	// Missing child under an existing parent.
	out := SpliceYAML("patterns:\n  fx: {base: EUR}\nname: x\n", []string{"patterns", "disabled"}, []any{"card"})
	cfg, errs := ParseConfig(out)
	if len(errs) > 0 {
		t.Fatalf("missing child: %v\n%s", errs, out)
	}
	if strings.Join(cfg.Patterns.Disabled, ",") != "card" || cfg.Patterns.Fx.Base != "EUR" {
		t.Errorf("missing child: disabled=%v fx=%q\n%s", cfg.Patterns.Disabled, cfg.Patterns.Fx.Base, out)
	}
	if !strings.Contains(out, "name: x") {
		t.Errorf("missing child dropped a sibling:\n%s", out)
	}

	// Missing parent entirely.
	out = SpliceYAML("name: x\n", []string{"patterns", "disabled"}, []any{"card"})
	cfg, errs = ParseConfig(out)
	if len(errs) > 0 {
		t.Fatalf("missing parent: %v\n%s", errs, out)
	}
	if strings.Join(cfg.Patterns.Disabled, ",") != "card" || cfg.Name != "x" {
		t.Errorf("missing parent: %v / %q\n%s", cfg.Patterns.Disabled, cfg.Name, out)
	}

	// Empty and unparsable sources fall back to a fresh document.
	if _, errs := ParseConfig(SpliceYAML("", []string{"patterns", "disabled"}, []any{"card"})); len(errs) > 0 {
		t.Errorf("empty source: %v", errs)
	}
}

func TestSpliceTopLevelKey(t *testing.T) {
	out := SpliceYAML(spliceSrc, []string{"privacy"}, NewOMap().Set("mask", []any{"vat"}))
	if !strings.Contains(out, "# brainDB config for the sales team") || !strings.Contains(out, "  fx: {base: EUR}") {
		t.Errorf("top-level splice damaged the rest:\n%s", out)
	}
	cfg, errs := ParseConfig(out)
	if len(errs) > 0 {
		t.Fatalf("does not parse: %v\n%s", errs, out)
	}
	if strings.Join(cfg.Privacy.Mask, ",") != "vat" {
		t.Errorf("mask = %v\n%s", cfg.Privacy.Mask, out)
	}
}
