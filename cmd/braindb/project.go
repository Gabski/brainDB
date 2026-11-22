// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Gabski/brainDB/internal/brain"
	"github.com/Gabski/brainDB/internal/mcp"
)

// A project keeps its databases in <project>/.braindb/<name>/ (brain.xml or brain.db, braindb.yaml, templates/).
// `braindb mcp` and `braindb run` find that directory by walking up from the working directory, so the MCP entry
// in a project needs no paths, and the agent can list and switch between the project's databases (and the
// shared ones in the user's data directory) without a restart.

const projectDirName = ".braindb"

// findProjectDir returns the nearest .braindb directory at or above start, or "".
func findProjectDir(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		p := filepath.Join(dir, projectDirName)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// globalDataDir is the user-wide directory for shared brains (BRAINDB_HOME, or $XDG_DATA_HOME/braindb), or ""
// when it does not exist. It only ever adds databases to a project; it never becomes the default by itself.
func globalDataDir() string {
	cands := []string{os.Getenv("BRAINDB_HOME")}
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		cands = append(cands, filepath.Join(x, "braindb"))
	}
	if h, err := os.UserHomeDir(); err == nil {
		cands = append(cands, filepath.Join(h, ".local", "share", "braindb"))
	}
	for _, c := range cands {
		if c == "" {
			continue
		}
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	return ""
}

// dbEntry is a database found on disk.
type dbEntry struct{ name, path string }

// projectCatalog is the set of databases one session can use. It rescans its roots on every listing, so a database
// created by another process or by `braindb init` shows up without a restart, and it keeps opened stores so
// switching back is instant.
type projectCatalog struct {
	mu     sync.Mutex
	roots  []string
	opened map[string]*brain.Store
	pinned map[string]*brain.Store // stores opened before the catalog existed (an explicit --brain)
}

func newCatalog(roots []string) *projectCatalog {
	return &projectCatalog{roots: roots, opened: map[string]*brain.Store{}, pinned: map[string]*brain.Store{}}
}

func (c *projectCatalog) pin(st *brain.Store) { c.pinned[st.Name] = st }

// scan lists the databases on disk: the first root wins when two roots hold the same name.
func (c *projectCatalog) scan() []dbEntry {
	seen := map[string]bool{}
	out := []dbEntry{}
	for _, root := range c.roots {
		es, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range es {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || seen[e.Name()] {
				continue
			}
			if p, ok := brain.FindBrainFile(filepath.Join(root, e.Name())); ok {
				seen[e.Name()] = true
				out = append(out, dbEntry{e.Name(), p})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// Names lists database names.
func (c *projectCatalog) Names() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	names := []string{}
	for _, e := range c.scan() {
		names = append(names, e.name)
	}
	for n := range c.pinned {
		if !contains(names, n) {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// describe takes the one-line "what is this brain for" from braindb.yaml without loading the brain.
func describe(path string) string {
	cfg, _, err := brain.LoadConfigFile(brain.ConfigPath(path))
	if err != nil || cfg == nil {
		return ""
	}
	switch {
	case cfg.Purpose != nil && cfg.Purpose.Description != "":
		return cfg.Purpose.Description
	case cfg.Description != "":
		return cfg.Description
	}
	return cfg.Goal
}

// List implements mcp.Catalog.
func (c *projectCatalog) List() []mcp.DBInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []mcp.DBInfo{}
	add := func(name, path string) {
		d := mcp.DBInfo{Name: name, Path: path, Backend: brain.BackendFor(path), Description: describe(path), Nodes: -1, Edges: -1}
		if st := c.opened[name]; st != nil {
			d.Nodes, d.Edges = st.B.Counts()
		} else if st := c.pinned[name]; st != nil {
			d.Nodes, d.Edges = st.B.Counts()
		}
		out = append(out, d)
	}
	listed := map[string]bool{}
	for _, e := range c.scan() {
		listed[e.name] = true
		add(e.name, e.path)
	}
	for n, st := range c.pinned {
		if !listed[n] {
			add(n, st.Path)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Open implements mcp.Catalog. Only names that scan found (or that were pinned) can be opened.
func (c *projectCatalog) Open(name string) (*brain.Store, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if st := c.pinned[name]; st != nil {
		return st, nil
	}
	if st := c.opened[name]; st != nil {
		return st, nil
	}
	for _, e := range c.scan() {
		if e.name != name {
			continue
		}
		st, err := brain.OpenStore(e.path, "")
		if err != nil {
			return nil, err
		}
		st.Name = name
		st.Log = logf
		configure(st)
		c.opened[name] = st
		return st, nil
	}
	names := []string{}
	for _, e := range c.scan() {
		names = append(names, e.name)
	}
	return nil, fmt.Errorf("no database %q (have: %s)", name, strings.Join(names, ", "))
}

// defaultDB picks the database a session starts in: --db / BRAINDB_DB, then the one named like the project
// directory (what `braindb init` creates), then "default", then the first by name.
func (c *projectCatalog) defaultDB(project string) string {
	names := c.Names()
	if len(names) == 0 {
		return ""
	}
	for _, want := range []string{initName(filepath.Dir(project)), "default"} {
		if contains(names, want) {
			return want
		}
	}
	return names[0]
}

// resolveStore opens the database a command works on, and the catalog of the others when there is one.
//
//	--brain FILE (or BRAINDB_BRAIN)   exactly that file, no catalog (plus a catalog of --data-dir when given)
//	--data-dir DIR (or env)           the databases in DIR; --db picks one
//	otherwise, inside a project       the databases in the nearest .braindb/ plus the shared ones
//	otherwise                         ./brain.xml as before (created when create is set)
func resolveStore(a *args, create bool) (*brain.Store, *projectCatalog) {
	open := func(p string) *brain.Store {
		if create {
			st := openStore(p, "")
			configure(st)
			return st
		}
		return openExisting(p)
	}
	dd := a.str("data-dir", "")
	if dd == "" && !a.has("brain") && env("BRAINDB_BRAIN", "") == "" {
		dd = env("BRAINDB_DATA_DIR", "")
	}
	if a.has("brain") || (env("BRAINDB_BRAIN", "") != "" && dd == "") {
		st := open(brainPath(a))
		if d := a.str("data-dir", ""); d != "" {
			cat := newCatalog([]string{d})
			if db := a.str("db", ""); db != "" {
				st.Name = db
			}
			cat.pin(st)
			return st, cat
		}
		return st, nil
	}
	var roots []string
	project := ""
	if dd != "" {
		roots = []string{dd}
		project = dd
	} else if cwd, err := os.Getwd(); err == nil {
		if project = findProjectDir(cwd); project != "" {
			roots = []string{project}
			if g := globalDataDir(); g != "" && !samePath(g, project) {
				roots = append(roots, g)
			}
		}
	}
	if len(roots) == 0 {
		if a.has("db") {
			die(errors.New("--db needs a project (run `braindb init`) or --data-dir"))
		}
		return open(brainPath(a)), nil
	}
	cat := newCatalog(roots)
	name := a.str("db", env("BRAINDB_DB", ""))
	if name == "" {
		name = cat.defaultDB(project)
	}
	if name == "" {
		die(fmt.Errorf("no databases in %s (create one: braindb init, or braindb db create NAME --data-dir %s)", strings.Join(roots, ", "), roots[0]))
	}
	st, err := cat.Open(name)
	if err != nil {
		die(err)
	}
	return st, cat
}

func samePath(a, b string) bool {
	aa, e1 := filepath.Abs(a)
	bb, e2 := filepath.Abs(b)
	return e1 == nil && e2 == nil && aa == bb
}

// ---------------------------------------------------------------- braindb init

func init() {
	extraCommands["init"] = cmdInit
	extraUsage = append(extraUsage,
		"  init     [NAME] [--dir DIR] [--example NAME] [--mcp]   (create .braindb/NAME in a project; --mcp writes .mcp.json)\n")
}

// initName turns a directory name into a database name.
func initName(dir string) string {
	var b strings.Builder
	for _, r := range filepath.Base(dir) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ' || r == '.':
			b.WriteRune('-')
		}
	}
	if s := strings.Trim(b.String(), "-"); s != "" {
		return s
	}
	return "project"
}

const braindbGitignore = `# brainDB working files. brain.xml / brain.db and braindb.yaml are the memory itself: commit them to share
# the brain with the team, or add "*/brain.*" here to keep it private.
*/brain.journal.jsonl
*/embeddings.cache
*/*.lock
*/*.tmp
*/*.bak
`

func cmdInit(argv []string) {
	a, err := parse(argv, flagSpec{"dir": true, "example": true, "examples": true, "mcp": false})
	if err != nil {
		die(err)
	}
	root, err := filepath.Abs(a.str("dir", "."))
	if err != nil {
		die(err)
	}
	name := initName(root)
	if len(a.pos) > 0 {
		name = a.pos[0]
	}
	dataDir := filepath.Join(root, projectDirName)
	dir := filepath.Join(dataDir, name)
	if brain.HasBrain(dir) {
		die(fmt.Errorf("database %s already exists in %s", name, dir))
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		die(err)
	}
	if gi := filepath.Join(dataDir, ".gitignore"); !fileExists(gi) {
		if err := os.WriteFile(gi, []byte(braindbGitignore), 0o644); err != nil {
			die(err)
		}
	}
	if ex := a.str("example", ""); ex != "" {
		st, err := brain.InstallExample(dataDir, a.str("examples", brain.DefaultExamplesDir()), ex, name)
		if err != nil {
			die(err)
		}
		st.Close()
	} else {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			die(err)
		}
		cfg := fmt.Sprintf("# Describe what belongs in this brain: agents see it, and brain_use lists it next to the project's other databases.\nname: %s\ndescription: Project memory for %s: decisions, conventions, open questions and what was learned.\n", name, filepath.Base(root))
		if err := os.WriteFile(filepath.Join(dir, "braindb.yaml"), []byte(cfg), 0o644); err != nil {
			die(err)
		}
		st, err := brain.OpenStore(filepath.Join(dir, "brain.xml"), "")
		if err != nil {
			die(err)
		}
		st.B.Name = name
		if err := st.Save(); err != nil {
			die(err)
		}
		st.Close()
	}
	logf("created %s", dir)
	if a.bool("mcp") {
		if err := writeMCPJSON(root); err != nil {
			die(err)
		}
		logf("wrote %s", filepath.Join(root, ".mcp.json"))
	} else {
		logf("connect an agent:  claude mcp add braindb -- braindb mcp --no-http   (run it in %s)", root)
	}
	logf("more databases:    braindb init OTHER   ·   agents list and switch with the brain_use tool")
}

// writeMCPJSON adds the braindb server to <root>/.mcp.json, keeping whatever else is in the file.
func writeMCPJSON(root string) error {
	p := filepath.Join(root, ".mcp.json")
	doc := map[string]any{}
	if d, err := os.ReadFile(p); err == nil {
		if err := json.Unmarshal(d, &doc); err != nil {
			return fmt.Errorf("%s is not valid JSON: %w", p, err)
		}
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	if _, ok := servers["braindb"]; ok {
		return nil
	}
	servers["braindb"] = map[string]any{"command": "braindb", "args": []string{"mcp", "--no-http"}}
	doc["mcpServers"] = servers
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(out, '\n'), 0o644)
}
