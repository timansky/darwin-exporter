//go:build darwin

package collector

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// defaultCommandTimeout is applied to all exec.Command calls in collectors.
// system_profiler and ioreg can be slow; 30s is generous but bounded.
const defaultCommandTimeout = 30 * time.Second

// runCommand executes name with args under a context-derived timeout and
// returns stdout bytes. It wraps exec.CommandContext so that a hung
// subprocess is killed when the deadline is exceeded.
func runCommand(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultCommandTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("command %s timed out after %s: %w", name, defaultCommandTimeout, ctx.Err())
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
