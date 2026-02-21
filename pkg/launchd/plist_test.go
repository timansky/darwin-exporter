//go:build darwin

package launchd

import (
	"strings"
	"testing"
)

func TestGeneratePlist_LaunchAgent(t *testing.T) {
	p := PlistParams{
		BinaryPath: "/usr/local/bin/darwin-exporter",
		LogDir:     "/Users/alice/.local/state/darwin-exporter",
		RunAsRoot:  false,
	}
	out, err := GeneratePlist(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)

	if !strings.Contains(s, "kz.neko.darwin-exporter") {
		t.Error("plist should contain label kz.neko.darwin-exporter")
	}
	if !strings.Contains(s, "/usr/local/bin/darwin-exporter") {
		t.Error("plist should contain BinaryPath")
	}
	if !strings.Contains(s, "darwin-exporter.log") {
		t.Error("plist should contain log path")
	}
	if strings.Contains(s, "UserName") {
		t.Error("LaunchAgent plist should NOT contain UserName key")
	}
	if !strings.Contains(s, "<key>KeepAlive</key>") {
		t.Error("plist should contain KeepAlive key")
	}
	if !strings.Contains(s, "<key>SuccessfulExit</key>") || !strings.Contains(s, "<false/>") {
		t.Error("plist should restart only on failure (KeepAlive.SuccessfulExit=false)")
	}
	if strings.Contains(s, "<key>KeepAlive</key>\n\t<true/>") {
		t.Error("plist should not use unconditional KeepAlive=true")
	}
}

func TestGeneratePlist_LaunchDaemon(t *testing.T) {
	p := PlistParams{
		BinaryPath: "/usr/local/bin/darwin-exporter",
		LogDir:     "/var/log/darwin-exporter",
		RunAsRoot:  true,
	}
	out, err := GeneratePlist(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)

	if !strings.Contains(s, "UserName") {
		t.Error("LaunchDaemon plist should contain UserName key")
	}
	if !strings.Contains(s, "<string>root</string>") {
		t.Error("LaunchDaemon plist should contain UserName=root")
	}
}

func TestGeneratePlist_WithConfig(t *testing.T) {
	p := PlistParams{
		BinaryPath: "/usr/local/bin/darwin-exporter",
		Config:     "/etc/darwin-exporter.yml",
	}
	out, err := GeneratePlist(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)

	if !strings.Contains(s, "--config=/etc/darwin-exporter.yml") {
		t.Error("plist should contain --config flag")
	}
}

func TestGeneratePlist_WithoutOptional(t *testing.T) {
	p := PlistParams{
		BinaryPath: "/usr/local/bin/darwin-exporter",
	}
	out, err := GeneratePlist(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)

	if strings.Contains(s, "--config") {
		t.Error("plist should NOT contain --config when Config is empty")
	}
	if strings.Contains(s, "--web.listen-address") {
		t.Error("plist should NOT contain --web.listen-address when Port is 0")
	}
	if strings.Contains(s, "StandardOutPath") {
		t.Error("plist should NOT contain log paths when LogDir is empty")
	}
}

func TestGeneratePlist_EmptyBinaryPath(t *testing.T) {
	p := PlistParams{}
	_, err := GeneratePlist(p)
	if err == nil {
		t.Error("expected error when BinaryPath is empty")
	}
}

func TestGeneratePlist_CustomLabel(t *testing.T) {
	p := PlistParams{
		Label:      "com.example.myservice",
		BinaryPath: "/usr/local/bin/myservice",
	}
	out, err := GeneratePlist(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), "com.example.myservice") {
		t.Error("plist should use custom label")
	}
}
