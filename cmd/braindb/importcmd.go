// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Gabski/brainDB/internal/bdql"
	"github.com/Gabski/brainDB/internal/brain"
)

func init() {
	extraCommands["create"] = cmdCreate
	extraCommands["import"] = cmdImport
	extraCommands["config"] = cmdConfig
	extraUsage = append(extraUsage,
		"  create   NAME --from PATH... [--config braindb.yaml] [--dry-run] [--data-dir data]\n",
		"  import   (--db NAME [--data-dir data] | --brain brain.xml) PATH|GLOB... [--dry-run] [--update]\n",
		"  config   show|validate (--db NAME [--data-dir data] | --brain brain.xml | FILE)\n")
	bdql.NewRunner = func(r *bdql.Runner) {
		// PATTERN ADD / REMOVE only change the live options; this makes them outlive the process.
		r.SavePat = func(st *brain.Store) error { return st.SavePatterns() }
		r.Import = func(st *brain.Store, path string, update, dry bool) (string, error) {
			files, err := brain.CollectFiles([]string{path}, 0)
			if err != nil {
				return "", err
			}
			rep, err := st.B.Import(files, brain.ImportOptions{Update: update, DryRun: dry, Config: st.Conf})
			if err != nil {
				return "", err
			}
			return rep.Text(), nil
		}
	}
}

func runImport(st *brain.Store, paths []string, update, dry bool) *brain.ImportReport {
	files, err := brain.CollectFiles(paths, 0)
	if err != nil {
		die(err)
	}
	if len(files) == 0 {
		die(errors.New("no files to import"))
	}
	rep, err := st.B.Import(files, brain.ImportOptions{Update: update, DryRun: dry, Config: st.Conf,
		Progress: func(done, total int, file string) {
			if file != "" && (done%50 == 0 || done == total-1) {
				logf("import %d/%d %s", done+1, total, file)
			}
		}})
	if err != nil {
		die(err)
	}
	if !dry {
		if err := st.Save(); err != nil {
			die(err)
		}
		st.Emit("cli", "import", fmt.Sprintf("import: %d files, %d nodes", rep.Totals.Files, rep.Totals.Nodes), nil)
	}
	fmt.Println(rep.Text())
	return rep
}

func cmdCreate(argv []string) {
	a, err := parse(argv, flagSpec{"from": true, "config": true, "dry-run": false, "data-dir": true, "json": false})
	if err != nil {
		die(err)
	}
	if len(a.pos) != 1 || len(a.all("from")) == 0 {
		die(errors.New("usage: braindb create NAME --from PATH... [--config braindb.yaml] [--dry-run] [--data-dir data]"))
	}
	name := a.pos[0]
	dir := filepath.Join(a.str("data-dir", env("BRAINDB_DATA_DIR", "./data")), name)
	p := filepath.Join(dir, "brain.xml")
	if _, err := os.Stat(p); err == nil {
		die(fmt.Errorf("database %s already exists (%s); use braindb import --db %s", name, p, name))
	}
	cfgText := ""
	if c := a.str("config", ""); c != "" {
		d, err := os.ReadFile(c)
		if err != nil {
			die(err)
		}
		cfgText = string(d)
		if _, errs := brain.ParseConfig(cfgText); len(errs) > 0 {
			for _, e := range errs {
				fmt.Fprintln(os.Stderr, c+": "+e.Error())
			}
			os.Exit(1)
		}
	}
	dry := a.bool("dry-run")
	var st *brain.Store
	if dry {
		b := brain.New()
		st = brain.NewMemStore(b, name)
		if cfgText != "" {
			cfg, _ := brain.ParseConfig(cfgText)
			st.Conf = cfg
			_ = b.ApplyConfig(cfg, true)
		}
	} else {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			die(err)
		}
		if cfgText != "" {
			if err := os.WriteFile(filepath.Join(dir, "braindb.yaml"), []byte(cfgText), 0o644); err != nil {
				die(err)
			}
		}
		st = openStore(p, "")
		if st.B.Name == "" {
			st.B.Name = name
		}
	}
	configure(st)
	runImport(st, a.all("from"), false, dry)
	if !dry {
		logf("created database %s in %s", name, dir)
	}
}

func cmdImport(argv []string) {
	a, err := parse(argv, flagSpec{"db": true, "data-dir": true, "brain": true, "dry-run": false, "update": false})
	if err != nil {
		die(err)
	}
	if len(a.pos) == 0 {
		die(errors.New("usage: braindb import --db NAME PATH... [--dry-run] [--update]"))
	}
	p := brainPath(a)
	if db := a.str("db", ""); db != "" {
		p = filepath.Join(a.str("data-dir", env("BRAINDB_DATA_DIR", "./data")), db, "brain.xml")
	}
	st := openExisting(p)
	runImport(st, a.pos, a.bool("update"), a.bool("dry-run"))
}

func cmdConfig(argv []string) {
	a, err := parse(argv, flagSpec{"db": true, "data-dir": true, "brain": true})
	if err != nil {
		die(err)
	}
	if len(a.pos) == 0 || (a.pos[0] != "show" && a.pos[0] != "validate") {
		die(errors.New("usage: braindb config show|validate (--db NAME | --brain brain.xml | FILE)"))
	}
	path := ""
	switch {
	case len(a.pos) > 1:
		path = a.pos[1]
	case a.has("db"):
		path = filepath.Join(a.str("data-dir", env("BRAINDB_DATA_DIR", "./data")), a.str("db", ""), "braindb.yaml")
	default:
		path = brain.ConfigPath(brainPath(a))
	}
	d, err := os.ReadFile(path)
	if err != nil {
		die(err)
	}
	cfg, errs := brain.ParseConfig(string(d))
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Println(path + ": " + e.Error())
		}
		os.Exit(1)
	}
	if a.pos[0] == "validate" {
		fmt.Println(path + ": ok")
		return
	}
	fmt.Print(brain.EmitYAML(cfg.Effective()))
	if flag := os.Getenv("BRAINDB_TOKEN"); flag != "" {
		fmt.Println("# env BRAINDB_TOKEN is set (value masked)")
	}
	_ = strings.TrimSpace
}
