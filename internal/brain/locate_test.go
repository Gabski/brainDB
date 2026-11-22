// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFindBrainFile(t *testing.T) {
	dir := t.TempDir()
	if _, ok := FindBrainFile(dir); ok {
		t.Fatal("empty directory holds no brain")
	}
	if got := BrainFileIn(dir); got != filepath.Join(dir, "brain.xml") {
		t.Fatalf("new databases default to brain.xml: %s", got)
	}
	db := filepath.Join(dir, "brain.db")
	if err := os.WriteFile(db, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, ok := FindBrainFile(dir); !ok || got != db {
		t.Fatalf("a directory with only brain.db must be found, not shadowed by a new brain.xml: %s", got)
	}
	xml := filepath.Join(dir, "brain.xml")
	if err := os.WriteFile(xml, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(db, old, old); err != nil {
		t.Fatal(err)
	}
	if got, _ := FindBrainFile(dir); got != xml {
		t.Fatalf("with both files the newer one is live: %s", got)
	}
}
