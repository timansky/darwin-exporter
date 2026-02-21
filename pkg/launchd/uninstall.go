//go:build darwin

package launchd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// UninstallOptions holds parameters for the uninstall subcommand.
type UninstallOptions struct {
	SudoFlag bool
	RootFlag bool
	Purge    bool
	Config   string // used with --purge to remove the config file
	LogDir   string // used with --purge to remove log directory
}

// Uninstall removes the launchd service and associated files.
func Uninstall(opts UninstallOptions, hooks Hooks) error {
	mode, err := DetectInstallMode(opts.SudoFlag, opts.RootFlag)
	if err != nil {
		return err
	}

	switch mode {
	case ModeSudo:
		return UninstallSudo(opts, hooks)
	case ModeRoot:
		return UninstallRoot(opts, hooks)
	default:
		return fmt.Errorf("unknown install mode")
	}
}

// UninstallSudo removes the LaunchAgent and sudoers file.
// The sudoers file is removed via sudo internally, so this function does not
// need to be invoked with sudo.
func UninstallSudo(opts UninstallOptions, hooks Hooks) error {
	invoker, err := ResolveInvokerUser()
	if err != nil {
		return fmt.Errorf("resolving invoker user: %w", err)
	}
	homeDir := invoker.HomeDir
	if homeDir == "" {
		return fmt.Errorf("could not determine home directory for %s", invoker.Username)
	}
	uidStr := invoker.Uid

	// Unload service (ignore errors: service may not be loaded).
	_ = exec.Command("launchctl", "bootout", "gui/"+uidStr+"/"+PlistLabel).Run()
	hooks.infof("Unloaded service %s", PlistLabel)

	// Remove plist.
	plistPath := filepath.Join(homeDir, "Library", "LaunchAgents", PlistLabel+".plist")
	if err := RemoveFile(plistPath); err != nil {
		return err
	}
	hooks.infof("Removed plist: %s", plistPath)

	// Remove sudoers file.
	if err := RemoveSudoers(); err != nil {
		return err
	}
	hooks.infof("Removed sudoers: %s", SudoersPath)

	if opts.Purge {
		if err := Purge(opts, homeDir, hooks); err != nil {
			return err
		}
	}

	return nil
}

// UninstallRoot removes the LaunchDaemon.
func UninstallRoot(opts UninstallOptions, hooks Hooks) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("--root mode requires root privileges (run with sudo)")
	}

	// Unload service (ignore errors: service may not be loaded).
	_ = exec.Command("launchctl", "bootout", "system/"+PlistLabel).Run()
	hooks.infof("Unloaded service %s", PlistLabel)

	// Remove plist.
	plistPath := "/Library/LaunchDaemons/" + PlistLabel + ".plist"
	if err := RemoveFile(plistPath); err != nil {
		return err
	}
	hooks.infof("Removed plist: %s", plistPath)

	if opts.Purge {
		if err := Purge(opts, "", hooks); err != nil {
			return err
		}
	}

	return nil
}

// DangerousPaths lists directories that must never be removed by purge.
var DangerousPaths = []string{
	"/",
	"/etc",
	"/usr",
	"/bin",
	"/sbin",
	"/var",
	"/tmp",
	"/System",
	"/Library",
}

// ValidatePurgeDir ensures that dir is safe to pass to os.RemoveAll.
// It rejects exact system directories and paths that do not contain
// "darwin-exporter" anywhere in the path.
func ValidatePurgeDir(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving purge path %q: %w", dir, err)
	}
	// Reject the dangerous roots themselves.
	for _, dp := range DangerousPaths {
		if abs == dp {
			return fmt.Errorf("refusing to remove system directory: %q", abs)
		}
	}
	// Require "darwin-exporter" somewhere in the path so that a stray
	// --log-dir /etc/something or --log-dir / can never blow away system files.
	if !strings.Contains(abs, "darwin-exporter") {
		return fmt.Errorf("log-dir %q does not look like a darwin-exporter directory (must contain 'darwin-exporter' in path)", abs)
	}
	return nil
}

// Purge removes config and log directory.
func Purge(opts UninstallOptions, homeDir string, hooks Hooks) error {
	logDir := opts.LogDir
	if logDir == "" {
		if homeDir != "" {
			xdgState := os.Getenv("XDG_STATE_HOME")
			if xdgState == "" {
				xdgState = filepath.Join(homeDir, ".local", "state")
			}
			logDir = filepath.Join(xdgState, "darwin-exporter")
		} else {
			logDir = "/var/log/darwin-exporter"
		}
	}

	if logDir != "" {
		if err := ValidatePurgeDir(logDir); err != nil {
			return fmt.Errorf("purge validation: %w", err)
		}
		if err := os.RemoveAll(logDir); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing log dir %s: %w", logDir, err)
		}
		hooks.infof("Removed log dir: %s", logDir)
	}

	if opts.Config != "" {
		if err := RemoveFile(opts.Config); err != nil {
			return err
		}
		hooks.infof("Removed config: %s", opts.Config)
	}

	return nil
}

// RemoveFile removes a file, ignoring "not found" errors.
func RemoveFile(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing %s: %w", path, err)
	}
	return nil
}
