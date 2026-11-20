// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/Gabski/brainDB/internal/brain"
)

func init() {
	extraCommands["example"] = cmdExample
}

func cmdExample(argv []string) {
	if len(argv) == 0 {
		die(errors.New("usage: braindb example ls | install NAME [--as DB] [--data-dir DIR] [--examples DIR]"))
	}
	switch argv[0] {
	case "ls":
		a, err := parse(argv[1:], flagSpec{"examples": true})
		if err != nil {
			die(err)
		}
		dir := a.str("examples", brain.DefaultExamplesDir())
		list, err := brain.ListExamples(dir)
		if err != nil {
			die(err)
		}
		if len(list) == 0 {
			fmt.Fprintf(os.Stderr, "no examples in %s\n", dir)
			return
		}
		for _, e := range list {
			fmt.Printf("%s\t%s\t%s\n", e.Name, e.Kind, e.Description)
		}
	case "install":
		a, err := parse(argv[1:], flagSpec{"as": true, "data-dir": true, "examples": true})
		if err != nil {
			die(err)
		}
		if len(a.pos) != 1 {
			die(errors.New("usage: braindb example install NAME [--as DB] [--data-dir DIR] [--examples DIR]"))
		}
		dbName := a.str("as", a.pos[0])
		dataDir := a.str("data-dir", env("BRAINDB_DATA_DIR", "./data"))
		examples := a.str("examples", brain.DefaultExamplesDir())
		st, err := brain.InstallExample(dataDir, examples, a.pos[0], dbName)
		if err != nil {
			die(err)
		}
		n, e := st.B.Counts()
		st.Close()
		logf("installed %s as %s in %s (%d nodes, %d edges)", a.pos[0], dbName, dataDir, n, e)
		logf("a server that is already running does not see it until restart; the UI and POST /api/dbs install into the live server")
	default:
		die(errors.New("usage: braindb example ls | install NAME [--as DB] [--data-dir DIR] [--examples DIR]"))
	}
}
