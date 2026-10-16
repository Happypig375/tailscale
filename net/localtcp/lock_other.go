// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build !linux

package localtcp

import (
	"errors"
	"os"
)

func lockEndpoint(string) (*os.File, error) {
	return nil, errors.New("restricted LocalAPI transport requires Linux")
}
