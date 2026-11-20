// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func isEmoji(r rune) bool {
	return r >= 0x1F300 && r <= 0x1FAFF || r >= 0x2600 && r <= 0x27BF || r == 0x2B50 || r == 0x2705 || r == 0x274C
}

// TestNoEmoji fails when emoji code points appear in sources, docs or examples (Addendum N). The documented symbols
// ◆ ⧉ ⇄ ▲ ▼ ~ ≈ → are outside the emoji ranges.
func TestNoEmoji(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, dir := range []string{"internal", "cmd", filepath.Join("web", "src"), "docs", "skills", "examples"} {
		_ = filepath.Walk(filepath.Join(root, dir), func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				if n := info.Name(); n == "node_modules" || n == "dist" || strings.HasPrefix(n, ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if info.Size() > 4<<20 {
				return nil
			}
			d, err := os.ReadFile(p)
			if err != nil || !utf8.Valid(d) {
				return nil
			}
			for i, line := range strings.Split(string(d), "\n") {
				for _, r := range line {
					if isEmoji(r) {
						t.Errorf("%s:%d: emoji %q (U+%04X)", p, i+1, r, r)
						break
					}
				}
			}
			return nil
		})
	}
}
