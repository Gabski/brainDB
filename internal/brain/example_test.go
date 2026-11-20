// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListShippedExamples(t *testing.T) {
	list, err := ListExamples(filepath.Join("..", "..", "examples"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Example{}
	for _, e := range list {
		got[e.Name] = e
	}
	for _, name := range []string{"sales-company", "research-lab", "braindb-self"} {
		e, ok := got[name]
		if !ok {
			t.Errorf("missing seed example %s", name)
			continue
		}
		if e.Kind != "seed" {
			t.Errorf("%s kind %s, want seed", name, e.Kind)
		}
		if e.Description == "" {
			t.Errorf("%s has no description", name)
		}
	}
	if e, ok := got["import-demo"]; !ok || e.Kind != "import" {
		t.Fatalf("import-demo: %+v", got["import-demo"])
	}
	if _, ok := got["federation"]; ok {
		t.Fatal("federation is two servers, not an installable database")
	}
}

func TestInstallSeedExample(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "examples", "mini")
	if err := os.MkdirAll(filepath.Join(src, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(p, s string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(src, "seed.json"), `{"name":"Mini","goal":"Ship the pack","memories":[{"id":"f1","kind":"fact","text":"sodium packs ship in Q4"}]}`)
	write(filepath.Join(src, "braindb.yaml"), "name: mini\ndescription: a tiny example\n")
	write(filepath.Join(src, "docs", "note.md"), "# Note\n\nPacks are late.\n")
	write(filepath.Join(src, "templates", "brief.md"), "status: x\n")

	data := filepath.Join(root, "data")
	st, err := InstallExample(data, filepath.Join(root, "examples"), "mini", "mini")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	n, _ := st.B.Counts()
	if n < 2 {
		t.Fatalf("seed plus docs should leave more than one node, got %d", n)
	}
	xml, err := os.ReadFile(filepath.Join(data, "mini", "brain.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(xml), "sodium packs ship in Q4") || !strings.Contains(string(xml), "Packs are late") {
		t.Fatal("installed brain is missing the seed fact or the ingested note")
	}
	if _, err := os.Stat(filepath.Join(data, "mini", "templates", "brief.md")); err != nil {
		t.Fatal("templates were not copied")
	}
	if _, err := InstallExample(data, filepath.Join(root, "examples"), "mini", "mini"); err == nil {
		t.Fatal("installing twice should fail")
	}
	if _, err := InstallExample(data, filepath.Join(root, "examples"), "../mini", "x"); err == nil {
		t.Fatal("example name must not be a path")
	}
}

func TestInstallImportExample(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "examples", "pile")
	if err := os.MkdirAll(filepath.Join(src, "raw"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "braindb.yaml"), []byte("name: pile\ndescription: a raw pile\nimport:\n  rules:\n    - match: \"*.md\"\n      importer: markdown\n      kind: file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "raw", "note.md"), []byte("# Hello\n\nA note from the pile.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := InstallExample(filepath.Join(root, "data"), filepath.Join(root, "examples"), "pile", "")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	n, _ := st.B.Counts()
	if n < 1 {
		t.Fatal("import example produced no nodes")
	}
}
