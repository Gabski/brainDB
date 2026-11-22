// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package brain

import (
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// lockProcess takes an exclusive lock on path (created if missing) and returns the function that releases it.
func lockProcess(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	h := windows.Handle(f.Fd())
	deadline := time.Now().Add(lockTimeout)
	for {
		ol := new(windows.Overlapped)
		err = windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			f.Close()
			return nil, err
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, fmt.Errorf("another brainDB process holds %s for more than %s", path, lockTimeout)
		}
		time.Sleep(lockPoll)
	}
	return func() {
		_ = windows.UnlockFileEx(h, 0, 1, 0, new(windows.Overlapped))
		_ = f.Close()
	}, nil
}
