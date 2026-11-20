// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package bdql

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	sqlBlock    = regexp.MustCompile("(?s)```sql\n(.*?)```")
	placeholder = regexp.MustCompile(`<[^>\s][^>]*>`)
)

// TestDocsParse parses every ```sql block in docs/AGENTS.md, README.md and skills/**/*.md (blocks with <…> placeholders
// are skipped), so the documentation cannot drift from the grammar.
func TestDocsParse(t *testing.T) {
	root := filepath.Join("..", "..")
	files := []string{filepath.Join(root, "docs", "AGENTS.md"), filepath.Join(root, "README.md")}
	_ = filepath.Walk(filepath.Join(root, "skills"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".md") {
			files = append(files, p)
		}
		return nil
	})
	n := 0
	for _, f := range files {
		d, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, m := range sqlBlock.FindAllStringSubmatch(string(d), -1) {
			if placeholder.MatchString(m[1]) {
				continue
			}
			n++
			if _, err := Parse(m[1]); err != nil {
				t.Errorf("%s: %v\n---\n%s", f, err, m[1])
			}
		}
	}
	t.Logf("parsed %d sql blocks", n)
}

// TestExamplesParse parses every examples/*/queries.bdql.
func TestExamplesParse(t *testing.T) {
	ms, _ := filepath.Glob(filepath.Join("..", "..", "examples", "*", "queries.bdql"))
	for _, f := range ms {
		d, _ := os.ReadFile(f)
		if _, err := Parse(string(d)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}
