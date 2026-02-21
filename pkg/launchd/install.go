//go:build darwin

package launchd

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
)

// ResolveInvokerUser returns the *user.User for the real invoker of the process.
// When running as root via sudo, it reads $SUDO_UID and looks up the user by
// numeric UID — this is resistant to $SUDO_USER env var tampering.
// Fallback chain:
//  1. SUDO_UID (numeric, set by sudo itself, not injectable by user)
//  2. SUDO_USER (username string, used as last resort with a warning)
//  3. os.Getuid() (own UID, for non-sudo context)
func ResolveInvokerUser() (*user.User, error) {
	if sudoUID := os.Getenv("SUDO_UID"); sudoUID != "" {
		u, err := user.LookupId(sudoUID)
		if err != nil {
			return nil, fmt.Errorf("lookup user by SUDO_UID %q: %w", sudoUID, err)
		}
		return u, nil
	}
	// Fallback: SUDO_USER env var (less secure, but maintains compat with
	// environments where SUDO_UID is not set).
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		u, err := user.Lookup(sudoUser)
		if err != nil {
			return nil, fmt.Errorf("lookup user %q from SUDO_USER: %w", sudoUser, err)
		}
		return u, nil
	}
	// Non-sudo context: use own UID.
	u, err := user.LookupId(strconv.Itoa(os.Getuid()))
	if err != nil {
		return nil, fmt.Errorf("lookup user by uid %d: %w", os.Getuid(), err)
	}
	return u, nil
}

// InstallMode determines how the service is installed.
type InstallMode int

const (
	// ModeSudo installs as a LaunchAgent for the current user,
	// with sudoers NOPASSWD for wdutil/ipconfig/powermetrics.
	ModeSudo InstallMode = iota + 1

	// ModeRoot installs as a LaunchDaemon running as root.
	ModeRoot
)

// InstallOptions holds parameters for the install subcommand.
type InstallOptions struct {
	SudoFlag bool
	RootFlag bool
	Config   string
	LogDir   string
	BinPath  string
}

// DetectInstallMode returns the appropriate InstallMode based on flags and
// the effective UID of the running process.
// --sudo flag takes priority over euid auto-detection so that
// "sudo darwin-exporter install --sudo" selects ModeSudo even when euid==0.
// Returns an error if neither mode can be determined.
func DetectInstallMode(sudoFlag, rootFlag bool) (InstallMode, error) {
	// --sudo has priority: explicit flag beats euid auto-detection.
	if sudoFlag {
		return ModeSudo, nil
	}
	// --root flag or running as root (no --sudo flag).
	if rootFlag || os.Geteuid() == 0 {
		return ModeRoot, nil
	}
	return 0, fmt.Errorf("specify --sudo or --root (or run with sudo)")
}

// Install performs the launchd service installation.
func Install(opts InstallOptions, hooks Hooks) error {
	mode, err := DetectInstallMode(opts.SudoFlag, opts.RootFlag)
	if err != nil {
		return err
	}

	binPath := opts.BinPath
	if binPath == "" {
		binPath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("detecting binary path: %w", err)
		}
	}

	switch mode {
	case ModeSudo:
		return InstallSudo(binPath, opts, hooks)
	case ModeRoot:
		return InstallRoot(binPath, opts, hooks)
	default:
		return fmt.Errorf("unknown install mode")
	}
}

// InstallSudo performs a LaunchAgent installation with sudoers NOPASSWD.
// The sudoers file is written via sudo internally, so this function does not
// need to be invoked with sudo.
func InstallSudo(binPath string, opts InstallOptions, hooks Hooks) error {
	// Determine the invoking user. When run without sudo, ResolveInvokerUser
	// falls back to the current process UID (the real user).
	invoker, err := ResolveInvokerUser()
	if err != nil {
		return fmt.Errorf("resolving invoker user: %w", err)
	}
	username := invoker.Username
	uid64, err := strconv.ParseInt(invoker.Uid, 10, 64)
	if err != nil {
		return fmt.Errorf("parsing invoker UID %q: %w", invoker.Uid, err)
	}
	uid := int(uid64)
	gid64, err := strconv.ParseInt(invoker.Gid, 10, 64)
	if err != nil {
		return fmt.Errorf("parsing invoker GID %q: %w", invoker.Gid, err)
	}
	gid := int(gid64)
	homeDir := invoker.HomeDir
	if homeDir == "" {
		return fmt.Errorf("could not determine home directory for %s", username)
	}

	logDir := opts.LogDir
	if logDir == "" {
		xdgState := os.Getenv("XDG_STATE_HOME")
		if xdgState == "" {
			xdgState = filepath.Join(homeDir, ".local", "state")
		}
		logDir = filepath.Join(xdgState, "darwin-exporter")
	}

	// Write sudoers file.
	if err := WriteSudoers(username); err != nil {
		return fmt.Errorf("writing sudoers: %w", err)
	}
	hooks.infof("Installed sudoers: %s", SudoersPath)

	// Generate plist.
	plistContent, err := GeneratePlist(PlistParams{
		BinaryPath: binPath,
		Config:     opts.Config,
		LogDir:     logDir,
		RunAsRoot:  false,
	})
	if err != nil {
		return err
	}

	// Write plist to user's LaunchAgents.
	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	if err := os.MkdirAll(launchAgentsDir, 0755); err != nil {
		return fmt.Errorf("creating LaunchAgents dir: %w", err)
	}
	// When running as root (e.g. sudo darwin-exporter install --sudo), fix
	// ownership of the directory so the real user owns it.
	if os.Geteuid() == 0 {
		if err := os.Chown(launchAgentsDir, uid, gid); err != nil {
			return fmt.Errorf("chown LaunchAgents dir: %w", err)
		}
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("creating log dir: %w", err)
	}
	// Same ownership fix for log directory when running as root.
	if os.Geteuid() == 0 {
		if err := os.Chown(logDir, uid, gid); err != nil {
			return fmt.Errorf("chown log dir: %w", err)
		}
	}

	plistPath := filepath.Join(launchAgentsDir, PlistLabel+".plist")
	if err := os.WriteFile(plistPath, plistContent, 0600); err != nil {
		return fmt.Errorf("writing plist: %w", err)
	}
	// Fix ownership of plist file when running as root so the real user can
	// read, load and unload the LaunchAgent without sudo.
	if os.Geteuid() == 0 {
		if err := os.Chown(plistPath, uid, gid); err != nil {
			return fmt.Errorf("chown plist: %w", err)
		}
	}
	hooks.infof("Installed plist: %s", plistPath)

	uidStr := strconv.Itoa(uid)
	if err := StartWithContext(LaunchdContext{
		Domain:    "gui/" + uidStr,
		Target:    "gui/" + uidStr + "/" + PlistLabel,
		PlistPath: plistPath,
	}); err != nil {
		return err
	}

	hooks.successf("Service %s started", PlistLabel)
	hooks.infof("Logs: %s/darwin-exporter.log", logDir)
	return nil
}

// InstallRoot performs a LaunchDaemon installation running as root.
func InstallRoot(binPath string, opts InstallOptions, hooks Hooks) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("--root mode requires root privileges (run with sudo)")
	}

	logDir := opts.LogDir
	if logDir == "" {
		logDir = "/var/log/darwin-exporter"
	}

	// Generate plist.
	plistContent, err := GeneratePlist(PlistParams{
		BinaryPath: binPath,
		Config:     opts.Config,
		LogDir:     logDir,
		RunAsRoot:  true,
	})
	if err != nil {
		return err
	}

	// Write plist to LaunchDaemons.
	daemonsDir := "/Library/LaunchDaemons"
	plistPath := filepath.Join(daemonsDir, PlistLabel+".plist")

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("creating log dir: %w", err)
	}

	if err := os.WriteFile(plistPath, plistContent, 0600); err != nil {
		return fmt.Errorf("writing plist: %w", err)
	}
	hooks.infof("Installed plist: %s", plistPath)

	if err := StartWithContext(LaunchdContext{
		Domain:    "system",
		Target:    "system/" + PlistLabel,
		PlistPath: plistPath,
	}); err != nil {
		return err
	}

	hooks.successf("Service %s started", PlistLabel)
	hooks.infof("Logs: %s/darwin-exporter.log", logDir)
	return nil
}
