// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package localtcp

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func lockEndpoint(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("LocalAPI endpoint already in use or cannot be locked: %w", err)
	}
	return f, nil
}
