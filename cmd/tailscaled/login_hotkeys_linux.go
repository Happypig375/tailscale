// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build linux && ts_restricted

package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnlocal"
	"tailscale.com/types/logger"
)

func init() { hookLoginHotkeys.Set(startLoginHotkeys) }

func startLoginHotkeys(ctx context.Context, lb *ipnlocal.LocalBackend, logf logger.Logf) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		loginHotkeys(ctx, lb, logf)
	}()
	return func() { cancel(); <-done }
}

// loginHotkeys reads single keys only from an interactive daemon terminal.
// Keep ISIG and output processing intact, so Ctrl+C and log newlines work as
// usual. Service runs and redirected input never change terminal settings.
func loginHotkeys(ctx context.Context, lb *ipnlocal.LocalBackend, logf logger.Logf) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) || !term.IsTerminal(int(os.Stderr.Fd())) {
		return
	}
	foreground, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP)
	if err != nil || foreground != unix.Getpgrp() {
		return // A background daemon must not capture its parent shell's input.
	}
	var mu sync.Mutex
	var authURL string
	needsLogin := true
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go lb.WatchNotifications(watchCtx, ipn.NotifyInitialState, nil, func(n *ipn.Notify) bool {
		mu.Lock()
		defer mu.Unlock()
		if n.State != nil {
			needsLogin = *n.State == ipn.NeedsLogin || *n.State == ipn.NoState
			if !needsLogin {
				authURL = ""
			}
		}
		if needsLogin && n.BrowseToURL != nil && *n.BrowseToURL != "" && *n.BrowseToURL != authURL {
			authURL = *n.BrowseToURL
			logf("To log in: %s\nPress c to copy the login URL, or Enter to open it in a browser.", authURL)
		}
		return true
	})
	var original *unix.Termios
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	defer func() {
		if original != nil {
			unix.IoctlSetTermios(fd, unix.TCSETS, original)
		}
	}()
	for ctx.Err() == nil {
		mu.Lock()
		url := authURL
		mu.Unlock()
		// Only intercept input while there is a login URL to act on.
		if url == "" {
			if original != nil {
				unix.IoctlSetTermios(fd, unix.TCSETS, original)
				original = nil
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				continue
			}
		}
		if original == nil {
			var err error
			original, err = unix.IoctlGetTermios(fd, unix.TCGETS)
			if err != nil {
				return
			}
			mode := *original
			mode.Lflag &^= unix.ICANON | unix.ECHO
			mode.Cc[unix.VMIN], mode.Cc[unix.VTIME] = 1, 0
			if err := unix.IoctlSetTermios(fd, unix.TCSETS, &mode); err != nil {
				return
			}
		}
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		if _, err := unix.Poll(fds, 200); err != nil {
			if err == unix.EINTR {
				continue
			}
			return
		}
		if fds[0].Revents&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 {
			return
		}
		if fds[0].Revents&unix.POLLIN == 0 {
			continue
		}
		var key [1]byte
		if n, err := unix.Read(fd, key[:]); err != nil || n == 0 {
			return
		}
		mu.Lock()
		url = authURL
		mu.Unlock()
		if url == "" {
			continue
		}
		var err error
		actionCtx, actionCancel := context.WithTimeout(ctx, 5*time.Second)
		switch key[0] {
		case 'c', 'C':
			err = copyLoginURL(actionCtx, url)
			if err == nil {
				logf("Login URL copied.")
			}
		case '\r', '\n':
			err = exec.CommandContext(actionCtx, "xdg-open", url).Run()
		default:
			actionCancel()
			continue
		}
		actionCancel()
		if err != nil {
			logf("Login shortcut unavailable: %v; use the printed URL.", err)
		}
	}
}

func copyLoginURL(ctx context.Context, url string) error {
	var args []string
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		args = []string{"wl-copy"}
	} else if os.Getenv("DISPLAY") != "" {
		if _, err := exec.LookPath("xclip"); err == nil {
			args = []string{"xclip", "-selection", "clipboard"}
		} else if _, err := exec.LookPath("xsel"); err == nil {
			args = []string{"xsel", "--input", "--clipboard"}
		}
	}
	if len(args) > 0 {
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Stdin = strings.NewReader(url)
		return cmd.Run()
	}
	// OSC 52 lets a supporting terminal copy to the user's clipboard even
	// over SSH, where a clipboard service on the daemon's host is unavailable.
	_, err := fmt.Fprintf(os.Stderr, "\x1b]52;c;%s\a", base64.StdEncoding.EncodeToString([]byte(url)))
	return err
}
