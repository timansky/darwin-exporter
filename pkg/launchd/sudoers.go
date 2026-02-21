//go:build darwin

package launchd

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

const (
	// SudoersPath is the file written for the sudoers NOPASSWD rule.
	SudoersPath = "/etc/sudoers.d/darwin-exporter"

	// WdutilBin is the path to the wdutil binary that gets NOPASSWD access.
	WdutilBin = "/usr/bin/wdutil"

	// IPConfigBin is the path to ipconfig used to enable verbose WiFi summary.
	IPConfigBin = "/usr/sbin/ipconfig"

	// PowermetricsBin is the path to powermetrics used for CPU temperature.
	PowermetricsBin = "/usr/bin/powermetrics"
)

// ValidUsernameRe is a POSIX allowlist for Unix usernames.
// Permits: letters, digits, underscore, hyphen, dot.
// First character must be a letter, digit, or underscore.
var ValidUsernameRe = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.-]*$`)

// GenerateSudoers returns the content for a sudoers drop-in file that
// grants username passwordless access to run:
// - `wdutil info` as root
// - `ipconfig setverbose 1|0` as root (for WiFi summary enrichment)
// - `powermetrics -n 1 -i 500 --samplers cpu_power` as root (CPU temperature)
func GenerateSudoers(username string) ([]byte, error) {
	if username == "" {
		return nil, fmt.Errorf("username must not be empty")
	}
	// Validate against POSIX username allowlist — rejects all sudoers special
	// characters (!, %, #, (, ), comma, space, etc.).
	if !ValidUsernameRe.MatchString(username) {
		return nil, fmt.Errorf("invalid username %q: must match ^[a-zA-Z0-9_][a-zA-Z0-9_.-]*$", username)
	}
	allowed := []string{
		WdutilBin + " info",
		IPConfigBin + " setverbose 1",
		IPConfigBin + " setverbose 0",
		PowermetricsBin + " -n 1 -i 500 --samplers cpu_power",
	}
	line := fmt.Sprintf("%s ALL=(root) NOPASSWD: %s\n", username, strings.Join(allowed, ", "))
	return []byte(line), nil
}

// WriteSudoers writes the sudoers drop-in file to SudoersPath, validates it
// with visudo, and sets the required 0440 permissions.
// When running as root the file is installed directly; otherwise sudo is used
// for the privileged copy step so the caller does not need to be root.
func WriteSudoers(username string) error {
	content, err := GenerateSudoers(username)
	if err != nil {
		return err
	}

	// Write to a temp file in the user-writable temp directory so that a
	// non-root caller can create and write the file before handing it to sudo.
	// Use os.CreateTemp with a random suffix to prevent symlink race (TOCTOU).
	tmpFile, err := os.CreateTemp("", ".darwin-exporter-*.tmp")
	if err != nil {
		return fmt.Errorf("creating sudoers temp file: %w", err)
	}
	// Ensure cleanup on error paths.
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmpFile.Write(content); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("writing sudoers temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing sudoers temp file: %w", err)
	}

	// Set permissions before visudo validation.
	if err := os.Chmod(tmpPath, 0440); err != nil {
		return fmt.Errorf("setting sudoers temp file permissions: %w", err)
	}

	// Validate with visudo.
	if err := ValidateSudoers(tmpPath); err != nil {
		return fmt.Errorf("sudoers validation failed: %w", err)
	}

	if os.Geteuid() == 0 {
		// Already root — rename directly and set final permissions.
		if err := os.Rename(tmpPath, SudoersPath); err != nil {
			return fmt.Errorf("installing sudoers file: %w", err)
		}
		if err := os.Chmod(SudoersPath, 0440); err != nil {
			return fmt.Errorf("setting sudoers permissions: %w", err)
		}
		return nil
	}

	// Not root — use sudo to copy the validated temp file into place.
	return WriteSudoersWithSudo(tmpPath, SudoersPath)
}

// WriteSudoersWithSudo copies a validated sudoers temp file to the destination
// using sudo, so the operation succeeds without the process running as root.
// The sudo password prompt (if required) is forwarded to the terminal.
func WriteSudoersWithSudo(tmpPath, destPath string) error {
	cmd := exec.Command("sudo", "cp", tmpPath, destPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sudo cp to sudoers: %w", err)
	}

	cmd2 := exec.Command("sudo", "chmod", "0440", destPath)
	cmd2.Stdin = os.Stdin
	cmd2.Stdout = os.Stdout
	cmd2.Stderr = os.Stderr
	if err := cmd2.Run(); err != nil {
		return fmt.Errorf("sudo chmod sudoers: %w", err)
	}
	return nil
}

// ValidateSudoers runs `visudo -cf path` to syntax-check a sudoers file.
func ValidateSudoers(path string) error {
	out, err := exec.Command("visudo", "-cf", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("visudo: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// RemoveSudoers deletes the sudoers drop-in file.
// Errors are ignored if the file does not exist.
// When not running as root, sudo rm is used to perform the privileged removal.
func RemoveSudoers() error {
	if os.Geteuid() == 0 {
		if err := os.Remove(SudoersPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing sudoers file: %w", err)
		}
		return nil
	}

	// Not root — use sudo to remove the file.
	cmd := exec.Command("sudo", "rm", "-f", SudoersPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sudo rm sudoers: %w", err)
	}
	return nil
}
