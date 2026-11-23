// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

//go:build !unix && !windows

package brain

// lockProcess is a no-op on platforms without advisory file locks.
func lockProcess(path string) (func(), error) { return func() {}, nil }
