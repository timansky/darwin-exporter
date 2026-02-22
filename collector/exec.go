//go:build darwin

package collector

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Command timeouts by command class. Slow macOS tooling gets larger budgets,
// while quick syscalls use tighter limits.
const (
	defaultCommandTimeout      = 30 * time.Second
	powermetricsCommandTimeout = 60 * time.Second
	systemProfilerTimeout      = 45 * time.Second
	wdutilCommandTimeout       = 15 * time.Second
	ioregCommandTimeout        = 10 * time.Second
	quickCommandTimeout        = 10 * time.Second
)

// runCommand executes name with args under a context-derived timeout and
// returns stdout bytes. It wraps exec.CommandContext so that a hung
// subprocess is killed when the deadline is exceeded.
func runCommand(name string, args ...string) ([]byte, error) {
	timeout := commandTimeout(name, args...)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("command %s timed out after %s: %w", name, timeout, ctx.Err())
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if stderr != "" {
				return nil, fmt.Errorf("%w: %s", err, stderr)
			}
		}
		return nil, err
	}
	return out, nil
}

func commandTimeout(name string, args ...string) time.Duration {
	switch commandKey(name, args...) {
	case "powermetrics":
		return powermetricsCommandTimeout
	case "system_profiler":
		return systemProfilerTimeout
	case "wdutil":
		return wdutilCommandTimeout
	case "ioreg":
		return ioregCommandTimeout
	case "notifyutil", "sysctl":
		return quickCommandTimeout
	default:
		return defaultCommandTimeout
	}
}

func commandKey(name string, args ...string) string {
	base := filepath.Base(name)
	if base != "sudo" {
		return base
	}
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if trimmed == "" || strings.HasPrefix(trimmed, "-") {
			continue
		}
		return filepath.Base(trimmed)
	}
	return base
}
