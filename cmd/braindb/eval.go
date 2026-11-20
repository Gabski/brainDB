// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	extraCommands["eval"] = cmdEval
	extraUsage = append(extraUsage,
		"  eval     --brain brain.xml --gold gold.json [--k 10] [--modes vector,hybrid] [--format md|json] [--detail]\n")
}

// cmdEval scores retrieval quality against a hand-judged gold set. Run it once per embedding engine to find out
// whether a remote provider is worth its bill, and compare `vector` against `hybrid` to find out whether graph
// activation earns its complexity.
func cmdEval(argv []string) {
	a, err := parse(argv, flagSpec{"brain": true, "gold": true, "k": true, "modes": true, "format": true,
		"detail": false, "data-dir": true, "db": true, "out": true})
	if err != nil {
		die(err)
	}
	gold := a.str("gold", "")
	if gold == "" && len(a.pos) == 1 {
		gold = a.pos[0]
	}
	if gold == "" {
		die(errors.New("usage: braindb eval --brain brain.xml --gold gold.json"))
	}
	p := brainPath(a)
	if dd := a.str("data-dir", ""); dd != "" && a.has("db") {
		p = filepath.Join(dd, a.str("db", ""), "brain.xml")
	}
	st := openExisting(p)
	configure(st)

	g, err := st.B.LoadGoldSet(gold)
	if err != nil {
		die(err)
	}
	if k := a.str("k", ""); k != "" {
		n := 0
		if _, err := fmt.Sscanf(k, "%d", &n); err != nil || n <= 0 {
			die(errors.New("--k must be a positive number"))
		}
		g.K = n
	}
	if m := a.str("modes", ""); m != "" {
		g.Modes = nil
		for _, x := range strings.Split(m, ",") {
			if x = strings.TrimSpace(x); x != "" {
				g.Modes = append(g.Modes, x)
			}
		}
	}
	rep, err := st.B.Evaluate(g, a.bool("detail"))
	if err != nil {
		die(err)
	}
	out := rep.Markdown()
	if a.str("format", "md") == "json" {
		d, _ := json.MarshalIndent(rep, "", "  ")
		out = string(d) + "\n"
	}
	if f := a.str("out", ""); f != "" {
		if err := os.WriteFile(f, []byte(out), 0o644); err != nil {
			die(err)
		}
		fmt.Fprintf(os.Stderr, "braindb: wrote %s\n", f)
		return
	}
	fmt.Print(out)
}
