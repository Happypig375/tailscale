// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build linux && ts_restricted

package localtcp

// Enabled selects authenticated loopback IPC for restricted Linux builds.
const Enabled = true
