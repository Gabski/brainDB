// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package brain

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

// lockProcess takes an exclusive advisory lock on path (created if missing) and returns the function that
// releases it. It polls instead of blocking forever, so a wedged writer cannot hang every other agent.
func lockProcess(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(lockTimeout)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
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
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
