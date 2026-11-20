// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package main

// `braindb bench` (Addendum F5): a synthetic brain of a chosen size, measured end to end on both
// storage backends. The point is to decide where indexes are worth adding from numbers rather than
// from guesses, which is what F4 asks for.

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Gabski/brainDB/internal/brain"
)

func init() {
	extraCommands["bench"] = cmdBench
	extraUsage = append(extraUsage, "  bench    [--nodes N] [--backend file|sqlite|both] [--keep DIR]\n")
}

var benchWords = strings.Fields(`renewal pricing discount pipeline forecast product sla competition contract
	stakeholder battery pack lease warranty cycle telemetry connector invoice quote margin escalation
	onboarding churn upsell procurement logistics freight dairy bakery lounge airport depot`)

// buildBench creates a synthetic brain: records with fields, notes, concepts and a realistic edge
// density (about four edges per node).
func buildBench(n int, seed int64) *brain.Brain {
	rng := rand.New(rand.NewSource(seed))
	b := brain.New()
	ops := make([]brain.Op, 0, 512)
	flush := func() {
		if len(ops) == 0 {
			return
		}
		if _, err := b.Exec(ops); err != nil {
			die(err)
		}
		ops = ops[:0]
	}
	for i := 0; i < n; i++ {
		w := func() string { return benchWords[rng.Intn(len(benchWords))] }
		text := fmt.Sprintf("%s %s %s for account %d", w(), w(), w(), rng.Intn(n/10+1))
		op := brain.Op{Op: "remember", Kind: "fact", Text: &text}
		if i%3 == 0 {
			op.Kind = "record"
			op.Fields = brain.Fields{
				{Name: "stage", Value: []string{"negotiation", "proposal", "won", "lost"}[rng.Intn(4)]},
				{Name: "value", Value: fmt.Sprintf("%d", 1000+rng.Intn(99000))},
				{Name: "owner", Value: w()},
			}
		}
		ops = append(ops, op)
		if len(ops) >= 512 {
			flush()
		}
	}
	flush()
	return b
}

type benchRow struct {
	name string
	d    time.Duration
	note string
}

func timeIt(name string, f func() string) benchRow {
	t0 := time.Now()
	note := f()
	return benchRow{name: name, d: time.Since(t0), note: note}
}

func mib() float64 {
	var m runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m)
	return float64(m.HeapAlloc) / (1 << 20)
}

func cmdBench(argv []string) {
	a, err := parse(argv, flagSpec{"nodes": true, "backend": true, "keep": true})
	if err != nil {
		die(err)
	}
	n := atoiDef(a.str("nodes", "20000"), 20000)
	if n < 100 {
		die(errors.New("--nodes must be at least 100"))
	}
	backends := []string{"file", "sqlite"}
	switch a.str("backend", "both") {
	case "file":
		backends = []string{"file"}
	case "sqlite":
		backends = []string{"sqlite"}
	case "both", "":
	default:
		die(errors.New("--backend must be file, sqlite or both"))
	}

	dir := a.str("keep", "")
	if dir == "" {
		dir, err = os.MkdirTemp("", "braindb-bench-")
		if err != nil {
			die(err)
		}
		defer os.RemoveAll(dir)
	} else if err := os.MkdirAll(dir, 0o755); err != nil {
		die(err)
	}

	logf("building a synthetic brain of %d nodes", n)
	t0 := time.Now()
	b := buildBench(n, 1)
	nodes, edges := b.Counts()
	logf("built %d nodes, %d edges in %s · heap %.0f MiB", nodes, edges, time.Since(t0).Round(time.Millisecond), mib())

	for _, be := range backends {
		ext := ".xml"
		if be == "sqlite" {
			ext = ".db"
		}
		path := filepath.Join(dir, "bench"+ext)
		_ = os.Remove(path)
		rows := []benchRow{}

		st := brain.NewMemStore(b, "bench")
		st.Path = path
		st.Backend = be
		rows = append(rows, timeIt("save", func() string {
			if err := st.Save(); err != nil {
				die(err)
			}
			if fi, err := os.Stat(path); err == nil {
				return fmt.Sprintf("%.1f MiB on disk", float64(fi.Size())/(1<<20))
			}
			return ""
		}))
		st.Close()

		var loaded *brain.Store
		rows = append(rows, timeIt("load", func() string {
			loaded, err = brain.OpenStore(path, "")
			if err != nil {
				die(err)
			}
			return fmt.Sprintf("heap %.0f MiB", mib())
		}))
		lb := loaded.B

		runq := func(name, src string) benchRow {
			q, err := brain.ParseQuery(src)
			if err != nil {
				die(err)
			}
			// The plan is read outside the timed section: explaining runs its own uncapped query and
			// would be charged to the measurement it is supposed to describe.
			plan := lb.ExplainQuery(q)
			row := timeIt(name, func() string {
				r, err := lb.RunQuery(q, "json")
				if err != nil {
					die(err)
				}
				return fmt.Sprintf("%d rows", len(r.Rows))
			})
			if plan.Pushdown {
				row.note += fmt.Sprintf(" · SQL can narrow %d → %d", plan.Total, plan.Candidates)
			} else {
				row.note += fmt.Sprintf(" · scans %d", plan.Total)
			}
			return row
		}
		rows = append(rows, runq("query: kind filter", "kind:record limit:50"))
		rows = append(rows, runq("query: field equality", "stage=won limit:50"))
		rows = append(rows, runq("query: field range", "value>50000 limit:50"))
		rows = append(rows, runq("query: kind + field", "kind:record stage=won limit:50"))
		rows = append(rows, timeIt("search: hybrid k=10", func() string {
			r, err := lb.Search("renewal pricing risk", "hybrid", 10)
			if err != nil {
				die(err)
			}
			return fmt.Sprintf("%d hits", len(r.Rows))
		}))
		rows = append(rows, timeIt("context: depth 2", func() string {
			return fmt.Sprintf("%d chars", len(lb.ContextMD()))
		}))
		loaded.Close()

		fmt.Printf("\n%s backend · %d nodes · %d edges\n", strings.ToUpper(be), nodes, edges)
		fmt.Printf("  %-24s %10s  %s\n", "step", "time", "note")
		for _, r := range rows {
			fmt.Printf("  %-24s %10s  %s\n", r.name, r.d.Round(time.Microsecond), r.note)
		}
	}
	fmt.Println()
}

func atoiDef(s string, def int) int {
	n := 0
	if s == "" {
		return def
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	return n
}

var _ = sort.Ints
