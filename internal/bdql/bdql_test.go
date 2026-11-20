// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package bdql

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Gabski/brainDB/internal/brain"
)

var update = flag.Bool("update", false, "rewrite golden files")

var testNow = time.Date(2026, 11, 20, 9, 30, 0, 0, time.UTC)

func fixture(t testing.TB) *brain.Store {
	t.Helper()
	b := brain.New()
	b.Now = func() time.Time { return testNow }
	if err := b.LoadSeedFile("testdata/seed.json"); err != nil {
		t.Fatal(err)
	}
	st := brain.NewMemStore(b, "fixture")
	return st
}

var msRe = regexp.MustCompile(`"elapsed_ms":[0-9.]+`)

func TestGolden(t *testing.T) {
	files, _ := filepath.Glob("testdata/*.bdql")
	if len(files) == 0 {
		t.Fatal("no golden scripts")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			src, _ := os.ReadFile(f)
			st := fixture(t)
			r := &Runner{Store: st, Actor: "test"}
			res, err := r.Run(string(src))
			got := res.Render("md")
			if err != nil {
				got += "\nERROR: " + err.Error()
			}
			golden := strings.TrimSuffix(f, ".bdql") + ".golden"
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("missing golden %s (run go test -update)", golden)
			}
			if got != string(want) {
				t.Errorf("output differs from %s; run go test ./internal/bdql -update and review the diff\n--- got ---\n%s", golden, got)
			}
		})
	}
}

func run(t *testing.T, st *brain.Store, script string, ro bool) (*Result, error) {
	t.Helper()
	r := &Runner{Store: st, ReadOnly: ro, Actor: "test"}
	return r.Run(script)
}

func TestTransactionRollback(t *testing.T) {
	st := fixture(t)
	before := st.B.XML()
	_, err := run(t, st, `REMEMBER fact "temporary" AS T; BOOST d121; BOOST nosuch;`, false)
	if err == nil || !strings.Contains(err.Error(), "statement 3") || !strings.Contains(err.Error(), "no such item nosuch") {
		t.Fatalf("want error in statement 3, got %v", err)
	}
	if st.B.XML() != before {
		t.Fatal("failed script changed the brain")
	}
	// BEGIN … COMMIT keeps the committed part
	_, err = run(t, st, `BEGIN; REMEMBER fact "kept fact" AS K; COMMIT; REMEMBER fact "lost fact"; BOOST nosuch;`, false)
	if err == nil {
		t.Fatal("want error")
	}
	r, _ := run(t, st, `SELECT id FROM memory WHERE text ~ 'kept fact'; SELECT id FROM memory WHERE text ~ 'lost fact';`, false)
	if len(r.Results[0].Rows) != 1 || len(r.Results[1].Rows) != 0 {
		t.Fatalf("commit/rollback mismatch: %d kept, %d lost", len(r.Results[0].Rows), len(r.Results[1].Rows))
	}
	// explicit ROLLBACK
	run(t, st, `BEGIN; REMEMBER fact "rolled"; ROLLBACK;`, false)
	r, _ = run(t, st, `SELECT id FROM memory WHERE text ~ 'rolled';`, false)
	if len(r.Results[0].Rows) != 0 {
		t.Fatal("ROLLBACK did not undo")
	}
}

func TestVariablesAndLet(t *testing.T) {
	st := fixture(t)
	r, err := run(t, st, `REMEMBER fact "new fact" AS F; LINK $F TO d121; LET $d = (SELECT id FROM memory WHERE kind = deal ORDER BY fields.value DESC LIMIT 1); BOOST $d;`, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Render("md"), "$d = g201") {
		t.Fatalf("LET did not bind g201: %s", r.Render("md"))
	}
	if _, err := run(t, st, `BOOST $nope;`, false); err == nil || !strings.Contains(err.Error(), "unknown variable $nope") {
		t.Fatalf("want unknown variable error, got %v", err)
	}
	if _, err := run(t, st, `LET $x = (SELECT id FROM memory WHERE kind = idea);`, false); err == nil || !strings.Contains(err.Error(), "no rows") {
		t.Fatalf("want no rows error, got %v", err)
	}
}

func TestBulkGuards(t *testing.T) {
	st := fixture(t)
	if _, err := run(t, st, `UPDATE memory SET text = 'x';`, false); err == nil || !strings.Contains(err.Error(), "WHERE") {
		t.Fatalf("UPDATE without WHERE must fail: %v", err)
	}
	if _, err := Parse(`FORGET WHERE;`); err == nil {
		t.Fatal("FORGET WHERE without predicate must fail to parse")
	}
	old := BulkLimit
	BulkLimit = 2
	defer func() { BulkLimit = old }()
	if _, err := run(t, st, `UPDATE memory SET fields.x = '1' WHERE kind = deal;`, false); err == nil || !strings.Contains(err.Error(), "CONFIRM") {
		t.Fatalf("bulk update must need CONFIRM: %v", err)
	}
	if _, err := run(t, st, `UPDATE memory SET fields.x = '1' WHERE kind = deal CONFIRM;`, false); err != nil {
		t.Fatal(err)
	}
	r, err := run(t, st, `FORGET WHERE kind = deal DRY RUN;`, false)
	if err != nil || !strings.Contains(r.Render("md"), "would forget 3") {
		t.Fatalf("dry run: %v %s", err, r.Render("md"))
	}
}

func TestReadOnly(t *testing.T) {
	st := fixture(t)
	if _, err := run(t, st, `SELECT id FROM memory LIMIT 1; BOOST d121;`, true); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("read-only must reject writes: %v", err)
	}
	if _, err := run(t, st, `SELECT id FROM memory LIMIT 1; FORGET WHERE kind = note DRY RUN;`, true); err != nil {
		t.Fatalf("reads and dry runs are allowed read-only: %v", err)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{"SELEC id FROM memory;", "line 1, col 1: unknown statement \"SELEC\" (did you mean SELECT?)"},
		{"SELECT id FROM memory\n  WHERE kind = deal ORDR BY id;", "line 2, col 21: expected WHERE or NEAR or GROUP or ORDER or LIMIT or OFFSET or FORMAT, got \"ORDR\" (did you mean ORDER?)"},
		{"SEARCH 'x' LIMT 3;", "did you mean LIMIT?"},
		{"REMEMBER fact 'unterminated;", "unterminated string"},
		{"INSERT INTO deal (text, value) VALUES ('a');", "1 values for 2 columns"},
		{"BOOST d121 d122;", "expected ; after BOOST"},
	}
	for _, c := range cases {
		_, err := Parse(c.src)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: want %q, got %v", c.src, c.want, err)
		}
		if err != nil && !strings.Contains(err.Error(), "^") {
			t.Errorf("%q: error has no caret excerpt: %v", c.src, err)
		}
	}
}

func TestDidYouMeanID(t *testing.T) {
	st := fixture(t)
	_, err := run(t, st, `BOOST d12l;`, false)
	if err == nil || !strings.Contains(err.Error(), "did you mean d121") {
		t.Fatalf("want did-you-mean for id, got %v", err)
	}
}

func TestReadOnlyScriptDoesNotSave(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "brain.xml")
	st, err := brain.OpenStore(p, "testdata/seed.json")
	if err != nil {
		t.Fatal(err)
	}
	fi1, _ := os.Stat(p)
	time.Sleep(10 * time.Millisecond)
	if _, err := run(t, st, `SELECT id FROM memory LIMIT 3;`, false); err != nil {
		t.Fatal(err)
	}
	fi2, _ := os.Stat(p)
	if !fi1.ModTime().Equal(fi2.ModTime()) {
		t.Fatal("read script rewrote brain.xml")
	}
	if _, err := run(t, st, `BOOST d121;`, false); err != nil {
		t.Fatal(err)
	}
	st2, err := brain.OpenStore(p, "")
	if err != nil {
		t.Fatal(err)
	}
	if st2.B.CurrentStep() != st.B.CurrentStep() {
		t.Fatal("write script was not saved")
	}
}

func FuzzParse(f *testing.F) {
	for _, s := range []string{"SELECT id FROM memory WHERE kind = deal;", "REMEMBER fact \"x\" AS F; LINK $F TO d1;", "FILL $$a: 'b'$$ AS yaml;",
		"SELECT COUNT(*) FROM memory GROUP BY kind ORDER BY COUNT(*) DESC;", "CONTEXT FOR 'x' DEPTH 2 SET;", "-- c\nSHOW kinds;"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		_, _ = Parse(s) // must not panic
	})
}
