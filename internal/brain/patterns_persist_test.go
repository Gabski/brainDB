// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gabski/brainDB/internal/patterns"
)

const persistCfg = `# the team's brain
name: persist-test            # keep this comment
patterns:
  # built-ins we do not want
  disabled: [card]
  custom:
    - {name: sku, type: id_code, regex: '\bSKU-\d{5}\b'}
privacy:
  mask: [phone]               # and this one
`

func persistStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "braindb.yaml")
	if err := os.WriteFile(cfg, []byte(persistCfg), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := OpenStore(filepath.Join(dir, "brain.xml"), "")
	if err != nil {
		t.Fatal(err)
	}
	return st, cfg
}

func customNames(o *patterns.Options) []string {
	out := []string{}
	for _, c := range o.Custom {
		out = append(out, c.Name)
	}
	return out
}

// A pattern added at runtime has to reach braindb.yaml, or it vanishes on the next config reload —
// and any other pattern change triggers one.
func TestPatternsSurviveSaveAndReload(t *testing.T) {
	st, cfg := persistStore(t)
	if err := st.B.AddPattern(&patterns.Custom{Name: "zzq", Type: "id_code", Regex: `\bzzq-\d{4}\b`}); err != nil {
		t.Fatal(err)
	}
	if err := st.SavePatterns(); err != nil {
		t.Fatal(err)
	}

	text, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got := string(text)
	for _, keep := range []string{
		"# the team's brain",
		"name: persist-test            # keep this comment",
		"  # built-ins we do not want",
		"  mask: [phone]               # and this one",
	} {
		if !strings.Contains(got, keep) {
			t.Errorf("saving patterns dropped %q from braindb.yaml:\n%s", keep, got)
		}
	}
	if !strings.Contains(got, "zzq") {
		t.Fatalf("the new pattern was not written:\n%s", got)
	}

	// Reopen the store: the pattern is only durable if it comes back from the file.
	reopened, err := OpenStore(filepath.Join(filepath.Dir(cfg), "brain.xml"), "")
	if err != nil {
		t.Fatal(err)
	}
	names := customNames(reopened.B.PatternOptions())
	if strings.Join(names, ",") != "sku,zzq" {
		t.Errorf("after reload custom patterns are %v, want [sku zzq]", names)
	}
	if ents := reopened.B.ExtractText("ref zzq-7781 here", nil); len(ents) == 0 {
		t.Error("the reloaded pattern does not extract anything")
	}
}

// Toggling one pattern must not take the custom ones with it: the save reapplies the config, so both
// halves have to be written together.
func TestTogglingAPatternKeepsCustomOnes(t *testing.T) {
	st, _ := persistStore(t)
	if err := st.B.AddPattern(&patterns.Custom{Name: "zzq", Type: "id_code", Regex: `\bzzq-\d{4}\b`}); err != nil {
		t.Fatal(err)
	}
	if err := st.SavePatterns(); err != nil {
		t.Fatal(err)
	}
	if err := st.SetPatternEnabled("email", false); err != nil {
		t.Fatal(err)
	}
	if names := customNames(st.B.PatternOptions()); strings.Join(names, ",") != "sku,zzq" {
		t.Fatalf("toggling a built-in deleted custom patterns: %v", names)
	}
	// And the disabled list round-trips, including what the file already had.
	dis := st.B.PatternOptions().Disabled
	if !contains(dis, "card") || !contains(dis, "email") {
		t.Errorf("disabled = %v, want both card and email", dis)
	}
	if err := st.SetPatternEnabled("email", true); err != nil {
		t.Fatal(err)
	}
	if dis := st.B.PatternOptions().Disabled; contains(dis, "email") {
		t.Errorf("re-enabling left email disabled: %v", dis)
	}
}

// Removing the last custom pattern has to clear the block, or the next load resurrects it.
func TestRemovingACustomPatternSticks(t *testing.T) {
	st, cfg := persistStore(t)
	if err := st.B.RemovePattern("sku"); err != nil {
		t.Fatal(err)
	}
	if err := st.SavePatterns(); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(cfg)
	if strings.Contains(string(text), "SKU-") {
		t.Errorf("the removed pattern is still in the file:\n%s", text)
	}
	reopened, err := OpenStore(filepath.Join(filepath.Dir(cfg), "brain.xml"), "")
	if err != nil {
		t.Fatal(err)
	}
	if names := customNames(reopened.B.PatternOptions()); len(names) != 0 {
		t.Errorf("after reload custom patterns are %v, want none", names)
	}
}
