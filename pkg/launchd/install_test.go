//go:build darwin

package launchd

import (
	"os/user"
	"strconv"
	"testing"
)

// TestResolveInvokerUser_SudoUID verifies that when SUDO_UID is set to the
// current process UID, ResolveInvokerUser returns the correct user.
func TestResolveInvokerUser_SudoUID(t *testing.T) {
	// Use the real current UID as SUDO_UID so the lookup always succeeds.
	u, err := user.Current()
	if err != nil {
		t.Skipf("cannot get current user: %v", err)
	}

	t.Setenv("SUDO_UID", u.Uid)
	t.Setenv("SUDO_USER", "")

	got, err := ResolveInvokerUser()
	if err != nil {
		t.Fatalf("ResolveInvokerUser() returned error: %v", err)
	}
	if got.Uid != u.Uid {
		t.Errorf("expected UID %s, got %s", u.Uid, got.Uid)
	}
}

// TestResolveInvokerUser_SudoUIDInvalid verifies that an invalid SUDO_UID
// returns an error (non-existent UID lookup fails).
func TestResolveInvokerUser_SudoUIDInvalid(t *testing.T) {
	// UID 999999999 is unlikely to exist on any macOS system.
	t.Setenv("SUDO_UID", "999999999")
	t.Setenv("SUDO_USER", "")

	_, err := ResolveInvokerUser()
	if err == nil {
		t.Error("expected error for non-existent SUDO_UID, got nil")
	}
}

// TestResolveInvokerUser_SudoUserFallback verifies that when SUDO_UID is empty,
// a valid SUDO_USER falls back correctly.
func TestResolveInvokerUser_SudoUserFallback(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Skipf("cannot get current user: %v", err)
	}

	t.Setenv("SUDO_UID", "")
	t.Setenv("SUDO_USER", u.Username)

	got, err := ResolveInvokerUser()
	if err != nil {
		t.Fatalf("ResolveInvokerUser() with SUDO_USER=%q returned error: %v", u.Username, err)
	}
	if got.Username != u.Username {
		t.Errorf("expected Username %s, got %s", u.Username, got.Username)
	}
}

// TestResolveInvokerUser_SudoUserInvalid verifies that an invalid SUDO_USER
// (non-existent username) returns an error.
func TestResolveInvokerUser_SudoUserInvalid(t *testing.T) {
	t.Setenv("SUDO_UID", "")
	t.Setenv("SUDO_USER", "this-user-does-not-exist-99999")

	_, err := ResolveInvokerUser()
	if err == nil {
		t.Error("expected error for non-existent SUDO_USER, got nil")
	}
}

// TestResolveInvokerUser_OwnUID verifies that when neither SUDO_UID nor
// SUDO_USER is set, resolveInvokerUser returns the current process user.
func TestResolveInvokerUser_OwnUID(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Skipf("cannot get current user: %v", err)
	}

	t.Setenv("SUDO_UID", "")
	t.Setenv("SUDO_USER", "")

	got, err := ResolveInvokerUser()
	if err != nil {
		t.Fatalf("ResolveInvokerUser() without SUDO vars returned error: %v", err)
	}
	// The UID should match the process's own UID.
	if got.Uid != u.Uid {
		t.Errorf("expected UID %s (own UID), got %s", u.Uid, got.Uid)
	}
}

// TestResolveInvokerUser_SudoUID_TakesPriorityOverSudoUser verifies that when
// both SUDO_UID and SUDO_USER are set, SUDO_UID wins (UID-based lookup).
func TestResolveInvokerUser_SudoUID_TakesPriorityOverSudoUser(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Skipf("cannot get current user: %v", err)
	}

	t.Setenv("SUDO_UID", u.Uid)
	// Set SUDO_USER to something that would fail if used.
	t.Setenv("SUDO_USER", "this-user-does-not-exist-99999")

	got, err := ResolveInvokerUser()
	if err != nil {
		t.Fatalf("ResolveInvokerUser() returned error: %v", err)
	}
	// Result must come from SUDO_UID, not SUDO_USER.
	if got.Uid != u.Uid {
		t.Errorf("expected UID %s from SUDO_UID, got %s", u.Uid, got.Uid)
	}
}

// TestResolveInvokerUser_SudoUIDNotNumeric verifies that a non-numeric SUDO_UID
// (e.g. injected value) causes an error.
func TestResolveInvokerUser_SudoUIDNotNumeric(t *testing.T) {
	t.Setenv("SUDO_UID", "not-a-number")
	t.Setenv("SUDO_USER", "")

	_, err := ResolveInvokerUser()
	if err == nil {
		t.Error("expected error for non-numeric SUDO_UID, got nil")
	}
}

// TestDetectInstallMode verifies flag-based mode detection.
func TestDetectInstallMode(t *testing.T) {
	// Note: we cannot test euid==0 path without root, so we test flag paths only.
	mode, err := DetectInstallMode(true, false)
	if err != nil {
		t.Fatalf("DetectInstallMode(sudo=true): unexpected error: %v", err)
	}
	if mode != ModeSudo {
		t.Errorf("expected ModeSudo, got %v", mode)
	}
}

// TestDetectInstallMode_NoFlags verifies that without flags and without root,
// an error is returned.
func TestDetectInstallMode_NoFlags(t *testing.T) {
	// This test only works when not running as root.
	u, err := user.Current()
	if err != nil {
		t.Skipf("cannot get current user: %v", err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil || uid == 0 {
		t.Skip("skipping: test must run as non-root")
	}

	_, err = DetectInstallMode(false, false)
	if err == nil {
		t.Error("expected error when no flags and not root, got nil")
	}
}
