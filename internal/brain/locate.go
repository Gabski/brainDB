// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"os"
	"path/filepath"
)

// brainFileNames are the durable files a database directory may hold, in tie-break order.
var brainFileNames = []string{"brain.db", "brain.xml"}

// FindBrainFile returns the brain file inside a database directory, whichever backend wrote it.
// Hard-coding brain.xml made a directory that only held brain.db look empty, and opening it then
// created a second, empty brain.xml next to the real data. When both exist (`braindb migrate` leaves
// both), the more recently written one is the live copy.
func FindBrainFile(dir string) (string, bool) {
	best, bestTime := "", int64(-1)
	for _, n := range brainFileNames {
		p := filepath.Join(dir, n)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			if t := st.ModTime().UnixNano(); t > bestTime {
				best, bestTime = p, t
			}
		}
	}
	return best, best != ""
}

// BrainFileIn is FindBrainFile with brain.xml as the answer for a directory that holds no brain yet.
func BrainFileIn(dir string) string {
	if p, ok := FindBrainFile(dir); ok {
		return p
	}
	return filepath.Join(dir, "brain.xml")
}

// HasBrain reports whether dir holds a database.
func HasBrain(dir string) bool {
	_, ok := FindBrainFile(dir)
	return ok
}
