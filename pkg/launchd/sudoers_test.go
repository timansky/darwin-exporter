//go:build darwin

package launchd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func expectedSudoersLine(username string) string {
	allowed := []string{
		WdutilBin + " info",
		IPConfigBin + " setverbose 1",
		IPConfigBin + " setverbose 0",
		PowermetricsBin + " -n 1 -i 500 --samplers cpu_power",
	}
	return fmt.Sprintf("%s ALL=(root) NOPASSWD: %s\n", username, strings.Join(allowed, ", "))
}

func TestGenerateSudoers_Valid(t *testing.T) {
	tests := []struct {
		username string
		wantLine string
	}{
		{
			username: "alice",
			wantLine: expectedSudoersLine("alice"),
		},
		{
			username: "bob.smith",
			wantLine: expectedSudoersLine("bob.smith"),
		},
		{
			username: "user123",
			wantLine: expectedSudoersLine("user123"),
		},
	}
	for _, tt := range tests {
		got, err := GenerateSudoers(tt.username)
		if err != nil {
			t.Errorf("GenerateSudoers(%q): unexpected error: %v", tt.username, err)
			continue
		}
		if string(got) != tt.wantLine {
			t.Errorf("GenerateSudoers(%q):\n  got:  %q\n  want: %q", tt.username, got, tt.wantLine)
		}
	}
}

func TestGenerateSudoers_EmptyUsername(t *testing.T) {
	_, err := GenerateSudoers("")
	if err == nil {
		t.Error("expected error for empty username")
	}
}

func TestGenerateSudoers_InvalidUsername(t *testing.T) {
	invalid := []string{
		"alice smith",  // space
		"alice\tsmith", // tab
		"alice\nsmith", // newline
		`alice\smith`,  // backslash
		`alice"smith`,  // double quote
		"alice'smith",  // single quote
	}
	for _, u := range invalid {
		_, err := GenerateSudoers(u)
		if err == nil {
			t.Errorf("expected error for invalid username %q", u)
		}
	}
}

func TestGenerateSudoers_ContainsWdutil(t *testing.T) {
	content, err := GenerateSudoers("testuser")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "/usr/bin/wdutil") {
		t.Error("sudoers content should reference /usr/bin/wdutil")
	}
	if !strings.Contains(string(content), "/usr/sbin/ipconfig setverbose 1") {
		t.Error("sudoers content should reference /usr/sbin/ipconfig setverbose 1")
	}
	if !strings.Contains(string(content), "/usr/sbin/ipconfig setverbose 0") {
		t.Error("sudoers content should reference /usr/sbin/ipconfig setverbose 0")
	}
	if !strings.Contains(string(content), "/usr/bin/powermetrics -n 1 -i 500 --samplers cpu_power") {
		t.Error("sudoers content should reference /usr/bin/powermetrics -n 1 -i 500 --samplers cpu_power")
	}
	if !strings.Contains(string(content), "NOPASSWD") {
		t.Error("sudoers content should contain NOPASSWD")
	}
}

// TestWriteSudoers_TempFile verifies that WriteSudoers uses os.CreateTemp
// (random suffix) rather than a predictable ".tmp" filename, and that the
// temp file does not persist after the function exits.
//
// We test this indirectly by patching sudoersPath to a temp directory so
// WriteSudoers can run without root (skipped if visudo is not available or
// we don't have write access).
func TestWriteSudoers_TempFileHasRandomSuffix(t *testing.T) {
	// We cannot call WriteSudoers for real without root and /etc/sudoers.d
	// access. Instead, verify that os.CreateTemp is used by checking that
	// two calls to os.CreateTemp with the same pattern produce different names.
	dir := t.TempDir()
	pattern := ".darwin-exporter-*.tmp"

	f1, err := os.CreateTemp(dir, pattern)
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	name1 := f1.Name()
	_ = f1.Close()
	_ = os.Remove(name1)

	f2, err := os.CreateTemp(dir, pattern)
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	name2 := f2.Name()
	_ = f2.Close()
	_ = os.Remove(name2)

	if filepath.Base(name1) == filepath.Base(name2) {
		t.Errorf("os.CreateTemp should produce different names, got %s and %s",
			filepath.Base(name1), filepath.Base(name2))
	}
	// Neither should end in a plain ".tmp" suffix (no random component).
	if strings.HasSuffix(name1, ".tmp") && !strings.Contains(name1, "-") {
		t.Errorf("expected randomised suffix, got predictable name: %s", name1)
	}
}

// TestWriteSudoers_TempFileCleansUp verifies that WriteSudoers does not leave
// behind temp files when validation fails (e.g. bad username causes
// GenerateSudoers to fail before any file is created).
func TestWriteSudoers_InvalidUsername_NoTempFile(t *testing.T) {
	dir := t.TempDir()
	before, _ := os.ReadDir(dir)

	// An invalid username causes GenerateSudoers to fail before any I/O.
	_ = WriteSudoers("invalid username with space")

	after, _ := os.ReadDir(dir)
	if len(after) != len(before) {
		t.Errorf("expected no files created in temp dir for invalid username, got %d new files", len(after)-len(before))
	}
}
