// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build linux && ts_restricted

package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestLoadProxyEnvironment(t *testing.T) {
	for _, tt := range []struct {
		name      string
		env, want map[string]string
	}{
		{"file defaults", map[string]string{}, map[string]string{
			"HTTPS_PROXY": "http://browser-proxy:8889", "https_proxy": "http://browser-proxy:8889",
			"NO_PROXY": "localhost,127.0.0.1,::1", "SSL_CERT_FILE": "/workspace/proxy-ca.pem",
		}},
		{"explicit lower-case overrides both file cases", map[string]string{"https_proxy": "http://override:8080"}, map[string]string{
			"https_proxy": "http://override:8080", "NO_PROXY": "localhost,127.0.0.1,::1", "SSL_CERT_FILE": "/workspace/proxy-ca.pem",
		}},
		{"explicit empty disables file proxy", map[string]string{"HTTPS_PROXY": ""}, map[string]string{
			"HTTPS_PROXY": "", "NO_PROXY": "localhost,127.0.0.1,::1", "SSL_CERT_FILE": "/workspace/proxy-ca.pem",
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			file := `# system defaults
export HTTPS_PROXY="http://browser-proxy:8889" # comment
https_proxy='http://browser-proxy:8889'
NO_PROXY=localhost,127.0.0.1,::1
SSL_CERT_FILE=/workspace/proxy-ca.pem
PATH=/do/not/import
`
			err := loadProxyEnvironment(strings.NewReader(file), func(key string) (string, bool) { v, ok := tt.env[key]; return v, ok }, func(k, v string) error { tt.env[k] = v; return nil })
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(tt.env, tt.want) {
				t.Fatalf("environment = %v, want %v", tt.env, tt.want)
			}
		})
	}
}

func TestLoadProxyEnvironmentDoesNotExposeCredentials(t *testing.T) {
	const secret = "private-password"
	err := loadProxyEnvironment(strings.NewReader("HTTPS_PROXY='http://user:"+secret+"@proxy"), func(string) (string, bool) { return "", false }, func(string, string) error { t.Fatal("applied malformed file"); return nil })
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatal("expected a parse error without credentials")
	}
}

func TestLoadProxyEnvironmentEmptyAssignment(t *testing.T) {
	var got string
	err := loadProxyEnvironment(strings.NewReader("HTTPS_PROXY= # disabled\n"), func(string) (string, bool) { return "", false }, func(_, value string) error { got = value; return nil })
	if err != nil || got != "" {
		t.Fatalf("empty assignment: value=%q, err=%v", got, err)
	}
}
