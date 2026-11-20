// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Gabski/brainDB/internal/bdql"
	"github.com/Gabski/brainDB/internal/brain"
)

func init() {
	serverHooks.Run = bdql.HTTPRun
	extraCommands["run"] = cmdRun
	extraCommands["shell"] = cmdShell
	extraUsage = append(extraUsage,
		"  run      --brain brain.xml (-e 'SELECT …' | script.bdql | -) [--format md|json|yaml|xml|csv] [--json] [--dry-run] [--read-only]\n",
		"  shell    --brain brain.xml   (BDQL REPL; \\d = SHOW, \\f md = default format, \\q = quit)\n")
}

func cmdRun(argv []string) {
	a, err := parse(argv, flagSpec{"brain": true, "e": true, "format": true, "json": false, "dry-run": false, "read-only": false, "actor": true, "data-dir": true, "db": true, "user": true})
	if err != nil {
		die(err)
	}
	script := a.str("e", "")
	if script == "" {
		if len(a.pos) != 1 {
			die(errors.New("usage: braindb run --brain brain.xml (-e 'SELECT …' | script.bdql | -)"))
		}
		var d []byte
		if a.pos[0] == "-" {
			d, err = io.ReadAll(os.Stdin)
		} else {
			d, err = os.ReadFile(a.pos[0])
		}
		if err != nil {
			die(err)
		}
		script = string(d)
	}
	p := brainPath(a)
	if dd := a.str("data-dir", ""); dd != "" && a.has("db") {
		p = filepath.Join(dd, a.str("db", ""), "brain.xml")
	}
	st := openExisting(p)
	if a.has("db") {
		st.Name = a.str("db", "")
	} else if filepath.Base(p) == "brain.xml" {
		st.Name = filepath.Base(filepath.Dir(p))
	}
	r := &bdql.Runner{Store: st, ReadOnly: a.bool("read-only"), Actor: a.str("actor", "cli"), DryRun: a.bool("dry-run")}
	if bdql.NewRunner != nil {
		bdql.NewRunner(r)
	}
	dataDir := a.str("data-dir", env("BRAINDB_DATA_DIR", ""))
	r.Use = func(name string) (*brain.Store, error) {
		if name == st.Name {
			return st, nil
		}
		if dataDir == "" {
			return nil, fmt.Errorf("no database %s (this run is bound to %s; pass --data-dir to switch)", name, st.Name)
		}
		q := filepath.Join(dataDir, name, "brain.xml")
		if !fileExists(q) {
			return nil, fmt.Errorf("no database %s in %s", name, dataDir)
		}
		s2 := openStore(q, "")
		s2.Name = name
		return s2, nil
	}
	if u := a.str("user", ""); u != "" {
		pr, err := st.B.PrincipalFor(u)
		if err != nil {
			die(err)
		}
		r.Principal = pr
	}
	res, err := r.Run(script)
	if a.bool("json") {
		printJSON(res)
	} else {
		fmt.Println(res.Render(a.str("format", "md")))
	}
	if err != nil {
		os.Exit(1)
	}
}

func cmdShell(argv []string) {
	a, err := parse(argv, flagSpec{"brain": true, "actor": true, "read-only": false})
	if err != nil {
		die(err)
	}
	st := openExisting(brainPath(a))
	format := "md"
	home, _ := os.UserHomeDir()
	hist := filepath.Join(home, ".braindb_history")
	hf, _ := os.OpenFile(hist, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if hf != nil {
		defer hf.Close()
	}
	fmt.Fprintln(os.Stderr, banner())
	fmt.Fprintln(os.Stderr, "BDQL shell on", st.Path, "· end statements with ; · \\d = SHOW kinds · \\f FORMAT · \\q quits")
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 16<<20)
	buf := ""
	prompt := func() {
		if buf == "" {
			fmt.Fprint(os.Stderr, "braindb> ")
		} else {
			fmt.Fprint(os.Stderr, "      -> ")
		}
	}
	prompt()
	for in.Scan() {
		line := in.Text()
		t := strings.TrimSpace(line)
		if buf == "" && strings.HasPrefix(t, "\\") {
			f := strings.Fields(t)
			switch f[0] {
			case "\\q":
				return
			case "\\d":
				what := "kinds"
				if len(f) > 1 {
					what = f[1]
				}
				line = "SHOW " + what + ";"
			case "\\f":
				if len(f) > 1 {
					format = f[1]
				}
				fmt.Fprintln(os.Stderr, "format:", format)
				prompt()
				continue
			default:
				fmt.Fprintln(os.Stderr, "unknown command; \\d [what], \\f FORMAT, \\q")
				prompt()
				continue
			}
		}
		buf += line + "\n"
		if !strings.HasSuffix(strings.TrimSpace(stripComment(buf)), ";") {
			prompt()
			continue
		}
		if hf != nil {
			_, _ = hf.WriteString(strings.ReplaceAll(strings.TrimSpace(buf), "\n", " ") + "\n")
		}
		r := &bdql.Runner{Store: st, ReadOnly: a.bool("read-only"), Actor: a.str("actor", "shell")}
		if bdql.NewRunner != nil {
			bdql.NewRunner(r)
		}
		res, _ := r.Run(buf)
		fmt.Println(res.Render(format))
		buf = ""
		prompt()
	}
}

func stripComment(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if j := strings.Index(l, "--"); j >= 0 {
			lines[i] = l[:j]
		}
	}
	return strings.Join(lines, "\n")
}

var _ = json.Marshal
