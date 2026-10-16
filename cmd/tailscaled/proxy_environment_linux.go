// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build linux && ts_restricted

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kballard/go-shellquote"
)

func init() {
	hookLoadProxyEnvironment.Set(func() error {
		f, err := os.Open("/etc/environment")
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read proxy settings from /etc/environment: %w", err)
		}
		defer f.Close()
		return loadProxyEnvironment(f, os.LookupEnv, os.Setenv)
	})
}

// loadProxyEnvironment imports only transport proxy and CA settings. A process
// setting in either case takes precedence over the file's entire case pair,
// including explicitly empty values. No shell code or expansions are evaluated.
func loadProxyEnvironment(r io.Reader, lookup func(string) (string, bool), set func(string, string) error) error {
	families := [][]string{
		{"HTTP_PROXY", "http_proxy"}, {"HTTPS_PROXY", "https_proxy"},
		{"ALL_PROXY", "all_proxy"}, {"NO_PROXY", "no_proxy"},
		{"SSL_CERT_FILE"}, {"SSL_CERT_DIR"},
	}
	allowed := make(map[string]bool)
	for _, family := range families {
		for _, name := range family {
			allowed[name] = true
		}
	}
	values := make(map[string]string)
	scanner := bufio.NewScanner(r)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		name, value, ok := strings.Cut(strings.TrimPrefix(text, "export "), "=")
		name = strings.TrimSpace(name)
		if !ok || !allowed[name] {
			continue
		}
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, "#") {
			value = "" // An empty assignment followed by a comment.
		}
		words, err := shellquote.Split(value)
		if err != nil || len(words) > 1 && !strings.HasPrefix(words[1], "#") {
			// Do not include the value: a proxy URL may contain credentials.
			return fmt.Errorf("invalid /etc/environment assignment for %s on line %d", name, line)
		}
		values[name] = ""
		if len(words) > 0 {
			values[name] = words[0]
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read /etc/environment: %w", err)
	}
	for _, family := range families {
		explicit := false
		for _, name := range family {
			if _, ok := lookup(name); ok {
				explicit = true
			}
		}
		if explicit {
			continue
		}
		for _, name := range family {
			if value, ok := values[name]; ok {
				if err := set(name, value); err != nil {
					return fmt.Errorf("set %s from /etc/environment: %w", name, err)
				}
			}
		}
	}
	return nil
}
