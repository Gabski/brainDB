// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Example is a bundled example that can be installed as a database.
type Example struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Kind        string `json:"kind"` // seed (seed.json + docs) or import (raw/ + braindb.yaml)
}

// DefaultExamplesDir is BRAINDB_EXAMPLES, or ./examples when that directory exists.
func DefaultExamplesDir() string {
	if d := os.Getenv("BRAINDB_EXAMPLES"); d != "" {
		return d
	}
	if st, err := os.Stat("examples"); err == nil && st.IsDir() {
		return "examples"
	}
	return ""
}

// ListExamples returns installable examples in dir (seed.json, or raw/ with braindb.yaml).
// A missing or empty dir is an empty list. Federation-style folders are skipped.
func ListExamples(dir string) ([]Example, error) {
	if dir == "" {
		return []Example{}, nil
	}
	es, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Example{}, nil
		}
		return nil, err
	}
	out := []Example{}
	for _, e := range es {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if !exampleNameOK(e.Name()) {
			continue
		}
		src := filepath.Join(dir, e.Name())
		kind := exampleKind(src)
		if kind == "" {
			continue
		}
		item := Example{Name: e.Name(), Kind: kind}
		if cfg, _, err := LoadConfigFile(filepath.Join(src, "braindb.yaml")); err == nil && cfg != nil {
			item.Description = cfg.Description
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// InstallExample builds a database from a bundled example.
// Seed examples are loaded with their docs ingested and templates copied.
// Import examples (raw/ plus braindb.yaml) are created the same way as `braindb create`.
func InstallExample(dataDir, examplesDir, example, dbName string) (*Store, error) {
	if !exampleNameOK(example) {
		return nil, fmt.Errorf("invalid example name %q", example)
	}
	if dbName == "" {
		dbName = example
	}
	if !exampleNameOK(dbName) {
		return nil, fmt.Errorf("invalid database name %q", dbName)
	}
	if examplesDir == "" {
		return nil, fmt.Errorf("examples directory is not configured (set BRAINDB_EXAMPLES or run from the repository)")
	}
	src := filepath.Join(examplesDir, example)
	kind := exampleKind(src)
	if kind == "" {
		return nil, fmt.Errorf("unknown example %q (have: %s)", example, strings.Join(exampleNames(examplesDir), ", "))
	}
	if dataDir == "" {
		return nil, fmt.Errorf("data directory is required")
	}
	dir := filepath.Join(dataDir, dbName)
	if HasBrain(dir) {
		return nil, fmt.Errorf("database %s already exists (%s)", dbName, dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(dir)
		}
	}()
	if err := copyFileIfExists(filepath.Join(src, "braindb.yaml"), filepath.Join(dir, "braindb.yaml")); err != nil {
		return nil, err
	}
	if st, err := os.Stat(filepath.Join(src, "templates")); err == nil && st.IsDir() {
		if err := copyDir(filepath.Join(src, "templates"), filepath.Join(dir, "templates")); err != nil {
			return nil, err
		}
	}
	var store *Store
	var err error
	switch kind {
	case "seed":
		store, err = installSeed(dir, src, dbName)
	default:
		store, err = installImport(dir, src, dbName)
	}
	if err != nil {
		return nil, err
	}
	store.Name = dbName
	cleanup = false
	return store, nil
}

func installSeed(dir, src, dbName string) (*Store, error) {
	st, err := OpenStore(filepath.Join(dir, "brain.xml"), filepath.Join(src, "seed.json"))
	if err != nil {
		return nil, err
	}
	st.Name = dbName
	docs := filepath.Join(src, "docs")
	if fi, err := os.Stat(docs); err == nil && fi.IsDir() {
		if _, err := st.B.IngestPath(docs, IngestOptions{Focus: false}); err != nil {
			st.Close()
			return nil, err
		}
		if err := st.Save(); err != nil {
			st.Close()
			return nil, err
		}
	}
	return st, nil
}

func installImport(dir, src, dbName string) (*Store, error) {
	st, err := OpenStore(filepath.Join(dir, "brain.xml"), "")
	if err != nil {
		return nil, err
	}
	st.Name = dbName
	files, err := CollectFiles([]string{filepath.Join(src, "raw")}, 0)
	if err != nil {
		st.Close()
		return nil, err
	}
	if len(files) == 0 {
		st.Close()
		return nil, fmt.Errorf("example %s: raw/ is empty", dbName)
	}
	if _, err := st.B.Import(files, ImportOptions{Config: st.Conf}); err != nil {
		st.Close()
		return nil, err
	}
	if err := st.Save(); err != nil {
		st.Close()
		return nil, err
	}
	return st, nil
}

func exampleKind(dir string) string {
	if st, err := os.Stat(filepath.Join(dir, "seed.json")); err == nil && !st.IsDir() {
		return "seed"
	}
	raw, rerr := os.Stat(filepath.Join(dir, "raw"))
	yaml, yerr := os.Stat(filepath.Join(dir, "braindb.yaml"))
	if rerr == nil && raw.IsDir() && yerr == nil && !yaml.IsDir() {
		return "import"
	}
	return ""
}

func exampleNames(dir string) []string {
	list, err := ListExamples(dir)
	if err != nil || len(list) == 0 {
		return []string{"(none)"}
	}
	names := make([]string, len(list))
	for i, e := range list {
		names[i] = e.Name
	}
	return names
}

func exampleNameOK(n string) bool {
	if n == "" || len(n) > 64 || strings.HasPrefix(n, ".") {
		return false
	}
	for _, r := range n {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func copyFileIfExists(src, dst string) error {
	d, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.WriteFile(dst, d, 0o644)
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		d, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, d, 0o644)
	})
}
