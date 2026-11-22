// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Two stores on one file stand in for two agents running two `braindb mcp` processes. flock conflicts between
// separate open files even inside one process, so this exercises the same path.
func TestConcurrentWritersLoseNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.xml")
	a, err := OpenStore(path, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := OpenStore(path, "")
	if err != nil {
		t.Fatal(err)
	}
	const each = 25
	var wg sync.WaitGroup
	for _, st := range []*Store{a, b} {
		wg.Add(1)
		go func(st *Store, tag string) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				text := fmt.Sprintf("%s fact %d", tag, i)
				err := st.Mutate(tag, "test", func(b *Brain) (string, []string, error) {
					_, err := b.Exec([]Op{{Op: "remember", Kind: "fact", Text: &text}})
					return text, nil, err
				})
				if err != nil {
					t.Error(err)
					return
				}
			}
		}(st, fmt.Sprintf("w%p", st))
	}
	wg.Wait()
	c, err := OpenStore(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := c.B.Counts(); n != 2*each {
		t.Fatalf("both writers saved %d facts, the file holds %d", 2*each, n)
	}
}

// Flush at shutdown must not write an old copy over what another process saved since.
func TestFlushDoesNotOverwriteNewerFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.xml")
	a, err := OpenStore(path, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := OpenStore(path, "")
	if err != nil {
		t.Fatal(err)
	}
	text := "written by the other process"
	if err := b.Mutate("b", "test", func(br *Brain) (string, []string, error) {
		_, err := br.Exec([]Op{{Op: "remember", Kind: "fact", Text: &text}})
		return text, nil, err
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Flush(); err != nil {
		t.Fatal(err)
	}
	c, err := OpenStore(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := c.B.Counts(); n != 1 {
		t.Fatalf("the stale store must not wipe the other write: %d nodes", n)
	}
	_ = os.Remove(path + ".lock")
}
