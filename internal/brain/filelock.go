// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

package brain

import "time"

const (
	lockTimeout = 30 * time.Second
	lockPoll    = 5 * time.Millisecond
)

// lockStore serialises writers across processes. Store.mu only orders goroutines inside one process;
// two agents, each with its own `braindb mcp` on the same brain file, would otherwise each load the file,
// apply a change and write the whole brain back, and the later write silently discarded the earlier one.
// The lock lives in a sidecar file so it survives the atomic rename of the brain file.
func (s *Store) lockStore() (func(), error) {
	if s.Path == "" {
		return func() {}, nil
	}
	return lockProcess(s.Path + ".lock")
}
