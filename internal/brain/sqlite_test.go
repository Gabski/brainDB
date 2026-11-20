// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sqliteFixture(t *testing.T) *Brain {
	t.Helper()
	b := New()
	b.Now = func() time.Time { return time.Date(2026, 11, 20, 9, 30, 0, 0, time.UTC) }
	if err := b.LoadSeedFile("testdata/parity_seed.json"); err != nil {
		t.Fatal(err)
	}
	return b
}

// The SQLite backend has to hold everything brain.xml holds. Comparing the XML of the original with
// the XML of a brain loaded back from SQLite catches any field the writer forgets, which a
// hand-written field-by-field assertion would not.
func TestSQLiteRoundTripIsLossless(t *testing.T) {
	b := sqliteFixture(t)
	// Exercise the parts a plain seed does not reach: records with fields, a file node, a connector,
	// an external stub, labels, a supersede chain and a purpose mark.
	if _, err := b.Exec([]Op{
		{Op: "remember", Kind: "fact", Text: sp("Halvorsen renewal at €48,000"), Fields: Fields{{"stage", "won"}, {"value", "48000"}}, Labels: []string{"finance"}},
		{Op: "remember", Kind: "note", Text: sp("a second note"), Tmp: "N"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.addConnector(ConnectorSpec{Type: "anchor", Label: "Customer 42", Key: "customer:42"}); err != nil {
		t.Fatal(err)
	}
	b.SetPurpose(&Purpose{Name: "sales", Description: "renewals and pricing"})

	before := b.XML()

	dir := t.TempDir()
	db, err := OpenSQLiteDB(filepath.Join(dir, "brain.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := b.WriteSQLite(db); err != nil {
		t.Fatal(err)
	}

	got := New()
	got.Now = b.Now
	if err := got.ReadSQLite(db); err != nil {
		t.Fatal(err)
	}
	if after := got.XML(); after != before {
		t.Errorf("the brain did not survive the round trip.\n--- wrote ---\n%s\n--- read back ---\n%s", before, after)
	}
}

// A second write must replace the contents, not append to them.
func TestSQLiteWriteIsIdempotent(t *testing.T) {
	b := sqliteFixture(t)
	dir := t.TempDir()
	db, err := OpenSQLiteDB(filepath.Join(dir, "brain.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for i := 0; i < 3; i++ {
		if err := b.WriteSQLite(db); err != nil {
			t.Fatal(err)
		}
	}
	var nodes, edges int
	if err := db.QueryRow("SELECT (SELECT count(*) FROM node), (SELECT count(*) FROM edge)").Scan(&nodes, &edges); err != nil {
		t.Fatal(err)
	}
	got := New()
	got.Now = b.Now
	if err := got.ReadSQLite(db); err != nil {
		t.Fatal(err)
	}
	if got.XML() != b.XML() {
		t.Error("three writes changed the result")
	}
	if n := len(b.State().Nodes); nodes != n {
		t.Errorf("node table holds %d rows, the brain has %d nodes", nodes, n)
	}
}

// The stem index is what lets text filters and graph seeding skip a full scan (F4).
func TestSQLiteBuildsTheStemIndex(t *testing.T) {
	b := sqliteFixture(t)
	dir := t.TempDir()
	db, err := OpenSQLiteDB(filepath.Join(dir, "brain.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := b.WriteSQLite(db); err != nil {
		t.Fatal(err)
	}
	var total int
	if err := db.QueryRow("SELECT count(*) FROM stem").Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total == 0 {
		t.Fatal("the stem index is empty")
	}
	// Every stem of a node's text must find that node again.
	var id, text string
	if err := db.QueryRow("SELECT id, text FROM node WHERE text != '' AND conn = '' LIMIT 1").Scan(&id, &text); err != nil {
		t.Fatal(err)
	}
	for s := range stems(text) {
		var n int
		if err := db.QueryRow("SELECT count(*) FROM stem WHERE stem = ? AND node_id = ?", s, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Errorf("stem %q of %s (%q) is not indexed", s, id, text)
		}
	}
}

// Numeric field values are stored twice: as text and normalised, so SQL can do range filters.
func TestSQLiteNormalisesNumericFields(t *testing.T) {
	b := sqliteFixture(t)
	if _, err := b.Exec([]Op{{Op: "remember", Kind: "fact", Text: sp("a deal"),
		Fields: Fields{{"value", "€48,000"}, {"stage", "won"}}}}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	db, err := OpenSQLiteDB(filepath.Join(dir, "brain.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := b.WriteSQLite(db); err != nil {
		t.Fatal(err)
	}
	var num float64
	if err := db.QueryRow("SELECT num FROM field WHERE name = 'value'").Scan(&num); err != nil {
		t.Fatal(err)
	}
	if num != 48000 {
		t.Errorf("field.num = %v, want 48000", num)
	}
	var stageNum any
	if err := db.QueryRow("SELECT num FROM field WHERE name = 'stage'").Scan(&stageNum); err != nil {
		t.Fatal(err)
	}
	if stageNum != nil {
		t.Errorf("a non-numeric field got num = %v, want NULL", stageNum)
	}
}

func TestSQLiteRefusesANewerSchema(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenSQLiteDB(filepath.Join(dir, "brain.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	b := sqliteFixture(t)
	if err := b.WriteSQLite(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE meta SET value = '999' WHERE key = 'schemaVersion'"); err != nil {
		t.Fatal(err)
	}
	err = New().ReadSQLite(db)
	if err == nil || !strings.Contains(err.Error(), "newer brainDB") {
		t.Errorf("want a refusal naming the newer schema, got %v", err)
	}
}

// Migration has to be lossless in both directions, or switching backend quietly costs data.
func TestMigrateBothWaysIsLossless(t *testing.T) {
	dir := t.TempDir()
	xmlPath := filepath.Join(dir, "brain.xml")
	st, err := OpenStore(xmlPath, "testdata/parity_seed.json")
	if err != nil {
		t.Fatal(err)
	}
	if st.BackendName() != "file" {
		t.Fatalf("a .xml brain opened as %q", st.BackendName())
	}
	before := st.B.XML()

	dbPath, err := st.MigrateTo("sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if BackendFor(dbPath) != "sqlite" {
		t.Fatalf("migration produced %s", dbPath)
	}
	st.Close()

	sq, err := OpenStore(dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	defer sq.Close()
	if sq.BackendName() != "sqlite" {
		t.Fatalf("a .db brain opened as %q", sq.BackendName())
	}
	if got := sq.B.XML(); got != before {
		t.Errorf("file -> sqlite lost something:\n--- before ---\n%s\n--- after ---\n%s", before, got)
	}

	// And back again, into a directory of its own so nothing is overwritten.
	dir2 := t.TempDir()
	db2 := filepath.Join(dir2, "brain.db")
	if err := copyFile(dbPath, db2); err != nil {
		t.Fatal(err)
	}
	sq2, err := OpenStore(db2, "")
	if err != nil {
		t.Fatal(err)
	}
	defer sq2.Close()
	back, err := sq2.MigrateTo("file")
	if err != nil {
		t.Fatal(err)
	}
	d, err := os.ReadFile(back)
	if err != nil {
		t.Fatal(err)
	}
	if string(d) != before {
		t.Errorf("sqlite -> file lost something:\n%s", string(d))
	}
}

// A write through the store has to be in the database after a reopen.
func TestSQLiteStoreWritesAreDurable(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "brain.db")
	st, err := OpenStore(p, "testdata/parity_seed.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.B.Exec([]Op{{Op: "remember", Kind: "fact", Text: sp("durable through sqlite")}}); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	st.Close()

	again, err := OpenStore(p, "")
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if !strings.Contains(again.B.XML(), "durable through sqlite") {
		t.Error("the write did not survive the reopen")
	}
}

func copyFile(from, to string) error {
	d, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, d, 0o644)
}
