package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()

	if cfg.Server.ListenAddress != "127.0.0.1:10102" {
		t.Errorf("expected listen_address 127.0.0.1:10102, got %q", cfg.Server.ListenAddress)
	}
	if cfg.Server.MetricsPath != "/metrics" {
		t.Errorf("expected metrics_path /metrics, got %q", cfg.Server.MetricsPath)
	}
	if cfg.Server.HealthPath != "/health" {
		t.Errorf("expected health_path /health, got %q", cfg.Server.HealthPath)
	}
	if cfg.Server.ReadyPath != "/ready" {
		t.Errorf("expected ready_path /ready, got %q", cfg.Server.ReadyPath)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("expected log level info, got %q", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "logfmt" {
		t.Errorf("expected log format logfmt, got %q", cfg.Logging.Format)
	}
	if cfg.Color != "auto" {
		t.Errorf("expected color auto, got %q", cfg.Color)
	}
	if !cfg.Collectors.WiFi.Enabled {
		t.Error("expected wifi collector enabled by default")
	}
	if !cfg.Collectors.Battery.Enabled {
		t.Error("expected battery collector enabled by default")
	}
	if !cfg.Collectors.Thermal.Enabled {
		t.Error("expected thermal collector enabled by default")
	}
}

func TestLoadEmptyPath(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") returned error: %v", err)
	}
	if cfg.Server.ListenAddress != "127.0.0.1:10102" {
		t.Errorf("expected default listen address 127.0.0.1:10102, got %q", cfg.Server.ListenAddress)
	}
	if cfg.Color != "auto" {
		t.Errorf("expected default color auto, got %q", cfg.Color)
	}
}

func TestLoad_WithTildePath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfgDir := filepath.Join(tmp, ".config", "darwin-exporter")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	cfgPath := filepath.Join(cfgDir, "config.yml")
	content := `
server:
  listen_address: ":19999"
  metrics_path: "/metrics"
logging:
  level: "info"
  format: "logfmt"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0600); err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	cfg, err := Load(DefaultConfigPath)
	if err != nil {
		t.Fatalf("Load(DefaultConfigPath) returned error: %v", err)
	}
	if cfg.Server.ListenAddress != ":19999" {
		t.Errorf("expected :19999, got %q", cfg.Server.ListenAddress)
	}
}

// TestDefaultListenAddress_LocalhostOnly is a regression test for BUG-004.
// The default listen address must bind only to localhost to prevent exposing
// sensitive metrics (WiFi BSSID, DNS server, battery details) to LAN peers.
func TestDefaultListenAddress_LocalhostOnly(t *testing.T) {
	cfg := defaultConfig()
	addr := cfg.Server.ListenAddress
	if addr == ":10102" {
		t.Fatal("BUG-004 regression: default ListenAddress must not be ':10102' (all interfaces); use '127.0.0.1:10102'")
	}
	if addr != "127.0.0.1:10102" {
		t.Errorf("BUG-004 regression: expected default '127.0.0.1:10102', got %q", addr)
	}
}

func TestLoadYAML(t *testing.T) {
	content := `
server:
  listen_address: ":19999"
  metrics_path: "/prom"
logging:
  level: "debug"
  format: "json"
color: "always"
collectors:
  wifi:
    enabled: false
  battery:
    enabled: true
  thermal:
    enabled: false
instance:
  name: "test-machine"
  labels:
    env: "testing"
`
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Server.ListenAddress != ":19999" {
		t.Errorf("expected :19999, got %q", cfg.Server.ListenAddress)
	}
	if cfg.Server.MetricsPath != "/prom" {
		t.Errorf("expected /prom, got %q", cfg.Server.MetricsPath)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected debug, got %q", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("expected json, got %q", cfg.Logging.Format)
	}
	if cfg.Color != "always" {
		t.Errorf("expected color always, got %q", cfg.Color)
	}
	if cfg.Collectors.WiFi.Enabled {
		t.Error("expected wifi disabled")
	}
	if cfg.Collectors.Thermal.Enabled {
		t.Error("expected thermal disabled")
	}
	if cfg.Instance.Name != "test-machine" {
		t.Errorf("expected test-machine, got %q", cfg.Instance.Name)
	}
	if cfg.Instance.Labels["env"] != "testing" {
		t.Errorf("expected env=testing, got %q", cfg.Instance.Labels["env"])
	}
}

func TestLoadDefaults_ReadTimeouts(t *testing.T) {
	cfg := defaultConfig()
	if cfg.Server.ReadTimeout != 30*time.Second {
		t.Errorf("expected 30s read timeout, got %v", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != 30*time.Second {
		t.Errorf("expected 30s write timeout, got %v", cfg.Server.WriteTimeout)
	}
}

func TestValidate_InvalidLevel(t *testing.T) {
	cfg := defaultConfig()
	cfg.Logging.Level = "verbose"
	if err := validate(cfg); err == nil {
		t.Error("expected error for invalid log level")
	}
}

func TestValidate_InvalidFormat(t *testing.T) {
	cfg := defaultConfig()
	cfg.Logging.Format = "xml"
	if err := validate(cfg); err == nil {
		t.Error("expected error for invalid log format")
	}
}

func TestValidate_InvalidColor(t *testing.T) {
	cfg := defaultConfig()
	cfg.Color = "rainbow"
	if err := validate(cfg); err == nil {
		t.Error("expected error for invalid color")
	}
}

func TestValidate_EmptyListenAddress(t *testing.T) {
	cfg := defaultConfig()
	cfg.Server.ListenAddress = ""
	if err := validate(cfg); err == nil {
		t.Error("expected error for empty listen address")
	}
}

func TestValidate_EmptyMetricsPath(t *testing.T) {
	cfg := defaultConfig()
	cfg.Server.MetricsPath = ""
	if err := validate(cfg); err == nil {
		t.Error("expected error for empty metrics_path")
	}
}

// TestReadInstanceName_FileExists verifies that the instance name is read and
// trimmed correctly from an existing file.
func TestReadInstanceName_FileExists(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "instance")
	if err := os.WriteFile(path, []byte("mymachine\n"), 0600); err != nil {
		t.Fatalf("writing instance file: %v", err)
	}

	name := readInstanceName(path)
	if name != "mymachine" {
		t.Errorf("expected 'mymachine', got %q", name)
	}
}

// TestReadInstanceName_WithTrailingNewlines verifies that multiple trailing
// newlines are stripped.
func TestReadInstanceName_WithTrailingNewlines(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "instance")
	if err := os.WriteFile(path, []byte("myhost\r\n\n"), 0600); err != nil {
		t.Fatalf("writing instance file: %v", err)
	}

	name := readInstanceName(path)
	if name != "myhost" {
		t.Errorf("expected 'myhost', got %q", name)
	}
}

// TestReadInstanceName_FileNotExist verifies that a missing file returns an
// empty string without error.
func TestReadInstanceName_FileNotExist(t *testing.T) {
	name := readInstanceName("/nonexistent/path/to/instance")
	if name != "" {
		t.Errorf("expected empty string for missing file, got %q", name)
	}
}

// TestReadInstanceName_DefaultPath verifies that passing an empty filePath
// falls back to the default ~/.instance logic (does not error out).
func TestReadInstanceName_DefaultPath(t *testing.T) {
	// Just verify it does not panic; actual result depends on test environment.
	_ = readInstanceName("")
}

// TestLoad_InstanceFile_CustomPath verifies that instance_file in YAML config
// causes the instance name to be read from the specified path.
func TestLoad_InstanceFile_CustomPath(t *testing.T) {
	tmp := t.TempDir()
	instanceFile := filepath.Join(tmp, "myinstance")
	if err := os.WriteFile(instanceFile, []byte("custom-host\n"), 0600); err != nil {
		t.Fatalf("writing instance file: %v", err)
	}

	content := `
server:
  listen_address: ":10102"
  metrics_path: "/metrics"
logging:
  level: "info"
  format: "logfmt"
instance:
  instance_file: "` + instanceFile + `"
`
	cfgFile := filepath.Join(tmp, "config.yml")
	if err := os.WriteFile(cfgFile, []byte(content), 0600); err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Instance.Name != "custom-host" {
		t.Errorf("expected instance name 'custom-host', got %q", cfg.Instance.Name)
	}
}

// TestReadInstanceName_PathTraversal verifies that path traversal sequences in
// filePath are rejected and the function returns an empty string.
func TestReadInstanceName_PathTraversal(t *testing.T) {
	traversalPaths := []string{
		"../../etc/passwd",
		"../../../etc/shadow",
		"/tmp/../etc/passwd",
		"/home/user/../../etc/hosts",
	}
	for _, p := range traversalPaths {
		name := readInstanceName(p)
		if name != "" {
			t.Errorf("readInstanceName(%q): expected empty string for path traversal, got %q", p, name)
		}
	}
}

// TestReadInstanceName_CleanPath verifies that normal paths (without ..)
// are still processed after filepath.Clean.
func TestReadInstanceName_CleanPath(t *testing.T) {
	tmp := t.TempDir()
	// Write instance file via a path that has redundant separators (benign).
	instanceFile := tmp + "//instance"
	if err := os.WriteFile(tmp+"/instance", []byte("cleanhost\n"), 0600); err != nil {
		t.Fatalf("writing instance file: %v", err)
	}
	name := readInstanceName(instanceFile)
	if name != "cleanhost" {
		t.Errorf("expected 'cleanhost', got %q", name)
	}
}

// TestLoad_InstanceFile_NameOverridesFile verifies that when instance.name is
// set explicitly in config, instance_file is not consulted.
func TestLoad_InstanceFile_NameOverridesFile(t *testing.T) {
	tmp := t.TempDir()
	instanceFile := filepath.Join(tmp, "myinstance")
	if err := os.WriteFile(instanceFile, []byte("file-host\n"), 0600); err != nil {
		t.Fatalf("writing instance file: %v", err)
	}

	content := `
server:
  listen_address: ":10102"
  metrics_path: "/metrics"
logging:
  level: "info"
  format: "logfmt"
instance:
  name: "explicit-host"
  instance_file: "` + instanceFile + `"
`
	cfgFile := filepath.Join(tmp, "config.yml")
	if err := os.WriteFile(cfgFile, []byte(content), 0600); err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	// Explicit name takes precedence.
	if cfg.Instance.Name != "explicit-host" {
		t.Errorf("expected 'explicit-host', got %q", cfg.Instance.Name)
	}
}
