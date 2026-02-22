package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// =============================================================================
// Unit Tests for LoadWithOverrides
// =============================================================================

// TestLoadWithOverrides_Defaults tests that defaults are applied when no
// config file, env vars, or CLI flags are provided.
func TestLoadWithOverrides_Defaults(t *testing.T) {
	cfg, err := LoadWithOverrides("", nil)
	if err != nil {
		t.Fatalf("LoadWithOverrides returned error: %v", err)
	}

	// Server defaults
	if cfg.Server.ListenAddress != "127.0.0.1:10102" {
		t.Errorf("expected default listen address 127.0.0.1:10102, got %q", cfg.Server.ListenAddress)
	}
	if cfg.Server.MetricsPath != "/metrics" {
		t.Errorf("expected default metrics path /metrics, got %q", cfg.Server.MetricsPath)
	}
	if cfg.Server.HealthPath != "/health" {
		t.Errorf("expected default health path /health, got %q", cfg.Server.HealthPath)
	}
	if cfg.Server.ReadyPath != "/ready" {
		t.Errorf("expected default ready path /ready, got %q", cfg.Server.ReadyPath)
	}
	if cfg.Server.ReadTimeout != 30*time.Second {
		t.Errorf("expected default read timeout 30s, got %v", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != 30*time.Second {
		t.Errorf("expected default write timeout 30s, got %v", cfg.Server.WriteTimeout)
	}

	// Logging defaults
	if cfg.Logging.Level != "info" {
		t.Errorf("expected default log level info, got %q", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "logfmt" {
		t.Errorf("expected default log format logfmt, got %q", cfg.Logging.Format)
	}
	if cfg.Color != "auto" {
		t.Errorf("expected default color auto, got %q", cfg.Color)
	}

	// Collectors defaults
	if !cfg.Collectors.WiFi.Enabled {
		t.Error("expected wifi collector enabled by default")
	}
	if !cfg.Collectors.Battery.Enabled {
		t.Error("expected battery collector enabled by default")
	}
	if !cfg.Collectors.Thermal.Enabled {
		t.Error("expected thermal collector enabled by default")
	}
	if !cfg.Collectors.Wdutil.Enabled {
		t.Error("expected wdutil collector enabled by default")
	}
}

func TestLoadWithOverrides_MissingCustomPathReturnsError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yml")

	if _, err := LoadWithOverrides(missing, nil); err == nil {
		t.Fatal("expected error for missing custom config path")
	}
}

// TestLoadWithOverrides_YAML tests that YAML config is loaded correctly.
func TestLoadWithOverrides_YAML(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "test.yml")
	content := `
server:
  listen_address: ":9090"
  metrics_path: "/custom-metrics"
  health_path: "/healthz"
  ready_path: "/readyz"
  read_timeout: 15s
  write_timeout: 45s
logging:
  level: "debug"
  format: "json"
color: "always"
collectors:
  wifi:
    enabled: false
  battery:
    enabled: false
  thermal:
    enabled: true
  wdutil:
    enabled: true
instance:
  name: "test-instance"
  instance_file: "/tmp/test-instance"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := LoadWithOverrides(cfgPath, nil)
	if err != nil {
		t.Fatalf("LoadWithOverrides returned error: %v", err)
	}

	// Server config from YAML
	if cfg.Server.ListenAddress != ":9090" {
		t.Errorf("expected listen address :9090, got %q", cfg.Server.ListenAddress)
	}
	if cfg.Server.MetricsPath != "/custom-metrics" {
		t.Errorf("expected metrics path /custom-metrics, got %q", cfg.Server.MetricsPath)
	}
	if cfg.Server.HealthPath != "/healthz" {
		t.Errorf("expected health path /healthz, got %q", cfg.Server.HealthPath)
	}
	if cfg.Server.ReadyPath != "/readyz" {
		t.Errorf("expected ready path /readyz, got %q", cfg.Server.ReadyPath)
	}
	if cfg.Server.ReadTimeout != 15*time.Second {
		t.Errorf("expected read timeout 15s, got %v", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != 45*time.Second {
		t.Errorf("expected write timeout 45s, got %v", cfg.Server.WriteTimeout)
	}

	// Logging config from YAML
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected log level debug, got %q", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("expected log format json, got %q", cfg.Logging.Format)
	}
	if cfg.Color != "always" {
		t.Errorf("expected color always, got %q", cfg.Color)
	}

	// Collectors config from YAML
	if cfg.Collectors.WiFi.Enabled {
		t.Error("expected wifi collector disabled")
	}
	if cfg.Collectors.Battery.Enabled {
		t.Error("expected battery collector disabled")
	}
	if !cfg.Collectors.Thermal.Enabled {
		t.Error("expected thermal collector enabled")
	}
	if !cfg.Collectors.Wdutil.Enabled {
		t.Error("expected wdutil collector enabled")
	}

	// Instance config from YAML
	if cfg.Instance.Name != "test-instance" {
		t.Errorf("expected instance name test-instance, got %q", cfg.Instance.Name)
	}
	if cfg.Instance.InstanceFile != "/tmp/test-instance" {
		t.Errorf("expected instance file /tmp/test-instance, got %q", cfg.Instance.InstanceFile)
	}
}

// TestLoadWithOverrides_ENV tests that environment variables are loaded correctly.
func TestLoadWithOverrides_ENV(t *testing.T) {
	// Set environment variables
	t.Setenv("DARWIN_EXPORTER_SERVER_LISTEN_ADDRESS", ":8080")
	t.Setenv("DARWIN_EXPORTER_SERVER_METRICS_PATH", "/env-metrics")
	t.Setenv("DARWIN_EXPORTER_SERVER_HEALTH_PATH", "/env-health")
	t.Setenv("DARWIN_EXPORTER_SERVER_READY_PATH", "/env-ready")
	t.Setenv("DARWIN_EXPORTER_SERVER_READ_TIMEOUT", "20s")
	t.Setenv("DARWIN_EXPORTER_SERVER_WRITE_TIMEOUT", "40s")
	t.Setenv("DARWIN_EXPORTER_LOGGING_LEVEL", "warn")
	t.Setenv("DARWIN_EXPORTER_LOGGING_FORMAT", "json")
	t.Setenv("DARWIN_EXPORTER_COLOR", "never")
	t.Setenv("DARWIN_EXPORTER_COLLECTORS_WIFI_ENABLED", "false")
	t.Setenv("DARWIN_EXPORTER_COLLECTORS_BATTERY_ENABLED", "true")
	t.Setenv("DARWIN_EXPORTER_COLLECTORS_THERMAL_ENABLED", "false")
	t.Setenv("DARWIN_EXPORTER_COLLECTORS_WDUTIL_ENABLED", "true")
	t.Setenv("DARWIN_EXPORTER_INSTANCE_NAME", "env-instance")
	t.Setenv("DARWIN_EXPORTER_INSTANCE_INSTANCE_FILE", "/tmp/env-instance")

	cfg, err := LoadWithOverrides("", nil)
	if err != nil {
		t.Fatalf("LoadWithOverrides returned error: %v", err)
	}

	// Server config from ENV
	if cfg.Server.ListenAddress != ":8080" {
		t.Errorf("expected listen address :8080, got %q", cfg.Server.ListenAddress)
	}
	if cfg.Server.MetricsPath != "/env-metrics" {
		t.Errorf("expected metrics path /env-metrics, got %q", cfg.Server.MetricsPath)
	}
	if cfg.Server.HealthPath != "/env-health" {
		t.Errorf("expected health path /env-health, got %q", cfg.Server.HealthPath)
	}
	if cfg.Server.ReadyPath != "/env-ready" {
		t.Errorf("expected ready path /env-ready, got %q", cfg.Server.ReadyPath)
	}
	if cfg.Server.ReadTimeout != 20*time.Second {
		t.Errorf("expected read timeout 20s, got %v", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != 40*time.Second {
		t.Errorf("expected write timeout 40s, got %v", cfg.Server.WriteTimeout)
	}

	// Logging config from ENV
	if cfg.Logging.Level != "warn" {
		t.Errorf("expected log level warn, got %q", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("expected log format json, got %q", cfg.Logging.Format)
	}
	if cfg.Color != "never" {
		t.Errorf("expected color never, got %q", cfg.Color)
	}

	// Collectors config from ENV
	if cfg.Collectors.WiFi.Enabled {
		t.Error("expected wifi collector disabled")
	}
	if !cfg.Collectors.Battery.Enabled {
		t.Error("expected battery collector enabled")
	}
	if cfg.Collectors.Thermal.Enabled {
		t.Error("expected thermal collector disabled")
	}
	if !cfg.Collectors.Wdutil.Enabled {
		t.Error("expected wdutil collector enabled")
	}

	// Instance config from ENV
	if cfg.Instance.Name != "env-instance" {
		t.Errorf("expected instance name env-instance, got %q", cfg.Instance.Name)
	}
	if cfg.Instance.InstanceFile != "/tmp/env-instance" {
		t.Errorf("expected instance file /tmp/env-instance, got %q", cfg.Instance.InstanceFile)
	}
}

// TestLoadWithOverrides_CLI tests that CLI flags are loaded correctly.
func TestLoadWithOverrides_CLI(t *testing.T) {
	cliFlags := &CLIFlags{
		Server: ServerCLIFlags{
			ListenAddress: ":7070",
			MetricsPath:   "/cli-metrics",
			HealthPath:    "/cli-health",
			ReadyPath:     "/cli-ready",
			ReadTimeout:   10 * time.Second,
			WriteTimeout:  50 * time.Second,
		},
		Logging: LoggingCLIFlags{
			Level:  "error",
			Format: "logfmt",
		},
		Collectors: CollectorsCLIFlags{
			WiFi:    CollectorBoolFlag{Value: false, HasValue: true},
			Battery: CollectorBoolFlag{Value: true, HasValue: true},
			Thermal: CollectorBoolFlag{Value: false, HasValue: true},
			Wdutil:  CollectorBoolFlag{Value: true, HasValue: true},
		},
		Instance: InstanceCLIFlags{
			Name:         "cli-instance",
			InstanceFile: "/tmp/cli-instance",
		},
		Color:    "always",
		ColorSet: true,
	}

	cfg, err := LoadWithOverrides("", cliFlags)
	if err != nil {
		t.Fatalf("LoadWithOverrides returned error: %v", err)
	}

	// Server config from CLI
	if cfg.Server.ListenAddress != ":7070" {
		t.Errorf("expected listen address :7070, got %q", cfg.Server.ListenAddress)
	}
	if cfg.Server.MetricsPath != "/cli-metrics" {
		t.Errorf("expected metrics path /cli-metrics, got %q", cfg.Server.MetricsPath)
	}
	if cfg.Server.HealthPath != "/cli-health" {
		t.Errorf("expected health path /cli-health, got %q", cfg.Server.HealthPath)
	}
	if cfg.Server.ReadyPath != "/cli-ready" {
		t.Errorf("expected ready path /cli-ready, got %q", cfg.Server.ReadyPath)
	}
	if cfg.Server.ReadTimeout != 10*time.Second {
		t.Errorf("expected read timeout 10s, got %v", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != 50*time.Second {
		t.Errorf("expected write timeout 50s, got %v", cfg.Server.WriteTimeout)
	}

	// Logging config from CLI
	if cfg.Logging.Level != "error" {
		t.Errorf("expected log level error, got %q", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "logfmt" {
		t.Errorf("expected log format logfmt, got %q", cfg.Logging.Format)
	}
	if cfg.Color != "always" {
		t.Errorf("expected color always, got %q", cfg.Color)
	}

	// Collectors config from CLI
	if cfg.Collectors.WiFi.Enabled {
		t.Error("expected wifi collector disabled")
	}
	if !cfg.Collectors.Battery.Enabled {
		t.Error("expected battery collector enabled")
	}
	if cfg.Collectors.Thermal.Enabled {
		t.Error("expected thermal collector disabled")
	}
	if !cfg.Collectors.Wdutil.Enabled {
		t.Error("expected wdutil collector enabled")
	}

	// Instance config from CLI
	if cfg.Instance.Name != "cli-instance" {
		t.Errorf("expected instance name cli-instance, got %q", cfg.Instance.Name)
	}
	if cfg.Instance.InstanceFile != "/tmp/cli-instance" {
		t.Errorf("expected instance file /tmp/cli-instance, got %q", cfg.Instance.InstanceFile)
	}
}

// =============================================================================
// Priority Tests: CLI > ENV > YAML > defaults
// =============================================================================

// TestLoadWithOverrides_Priority_CLI_Over_ENV tests that CLI flags override ENV vars.
func TestLoadWithOverrides_Priority_CLI_Over_ENV(t *testing.T) {
	// Set ENV vars
	t.Setenv("DARWIN_EXPORTER_SERVER_LISTEN_ADDRESS", ":8080")
	t.Setenv("DARWIN_EXPORTER_LOGGING_LEVEL", "warn")
	t.Setenv("DARWIN_EXPORTER_COLLECTORS_WIFI_ENABLED", "false")

	// Set CLI flags (should override ENV)
	cliFlags := &CLIFlags{
		Server: ServerCLIFlags{
			ListenAddress: ":7070",
		},
		Logging: LoggingCLIFlags{
			Level: "error",
		},
		Collectors: CollectorsCLIFlags{
			WiFi: CollectorBoolFlag{Value: true, HasValue: true},
		},
	}

	cfg, err := LoadWithOverrides("", cliFlags)
	if err != nil {
		t.Fatalf("LoadWithOverrides returned error: %v", err)
	}

	// CLI should override ENV
	if cfg.Server.ListenAddress != ":7070" {
		t.Errorf("expected CLI to override ENV for listen address, got %q", cfg.Server.ListenAddress)
	}
	if cfg.Logging.Level != "error" {
		t.Errorf("expected CLI to override ENV for log level, got %q", cfg.Logging.Level)
	}
	if !cfg.Collectors.WiFi.Enabled {
		t.Error("expected CLI to override ENV for wifi collector")
	}
}

// TestLoadWithOverrides_Priority_ENV_Over_YAML tests that ENV vars override YAML config.
func TestLoadWithOverrides_Priority_ENV_Over_YAML(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "test.yml")
	content := `
server:
  listen_address: ":9090"
logging:
  level: "debug"
collectors:
  wifi:
    enabled: true
`
	if err := os.WriteFile(cfgPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Set ENV vars (should override YAML)
	t.Setenv("DARWIN_EXPORTER_SERVER_LISTEN_ADDRESS", ":8080")
	t.Setenv("DARWIN_EXPORTER_LOGGING_LEVEL", "warn")
	t.Setenv("DARWIN_EXPORTER_COLLECTORS_WIFI_ENABLED", "false")

	cfg, err := LoadWithOverrides(cfgPath, nil)
	if err != nil {
		t.Fatalf("LoadWithOverrides returned error: %v", err)
	}

	// ENV should override YAML
	if cfg.Server.ListenAddress != ":8080" {
		t.Errorf("expected ENV to override YAML for listen address, got %q", cfg.Server.ListenAddress)
	}
	if cfg.Logging.Level != "warn" {
		t.Errorf("expected ENV to override YAML for log level, got %q", cfg.Logging.Level)
	}
	if cfg.Collectors.WiFi.Enabled {
		t.Error("expected ENV to override YAML for wifi collector")
	}
}

// TestLoadWithOverrides_Priority_YAML_Over_Defaults tests that YAML config overrides defaults.
func TestLoadWithOverrides_Priority_YAML_Over_Defaults(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "test.yml")
	content := `
server:
  listen_address: ":9090"
logging:
  level: "debug"
collectors:
  wifi:
    enabled: false
`
	if err := os.WriteFile(cfgPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := LoadWithOverrides(cfgPath, nil)
	if err != nil {
		t.Fatalf("LoadWithOverrides returned error: %v", err)
	}

	// YAML should override defaults
	if cfg.Server.ListenAddress != ":9090" {
		t.Errorf("expected YAML to override defaults for listen address, got %q", cfg.Server.ListenAddress)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected YAML to override defaults for log level, got %q", cfg.Logging.Level)
	}
	if cfg.Collectors.WiFi.Enabled {
		t.Error("expected YAML to override defaults for wifi collector")
	}
}

// TestLoadWithOverrides_Priority_FullChain tests the full priority chain: CLI > ENV > YAML > defaults.
func TestLoadWithOverrides_Priority_FullChain(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "test.yml")
	content := `
server:
  listen_address: ":9090"
  metrics_path: "/yaml-metrics"
logging:
  level: "debug"
  format: "json"
collectors:
  wifi:
    enabled: true
  battery:
    enabled: true
`
	if err := os.WriteFile(cfgPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Set ENV vars (some override YAML)
	t.Setenv("DARWIN_EXPORTER_SERVER_LISTEN_ADDRESS", ":8080")
	t.Setenv("DARWIN_EXPORTER_LOGGING_LEVEL", "warn")
	t.Setenv("DARWIN_EXPORTER_COLLECTORS_WIFI_ENABLED", "false")

	// Set CLI flags (some override ENV)
	cliFlags := &CLIFlags{
		Server: ServerCLIFlags{
			ListenAddress: ":7070",
		},
		Logging: LoggingCLIFlags{
			Level: "error",
		},
	}

	cfg, err := LoadWithOverrides(cfgPath, cliFlags)
	if err != nil {
		t.Fatalf("LoadWithOverrides returned error: %v", err)
	}

	// CLI > ENV > YAML > defaults
	// ListenAddress: CLI overrides ENV and YAML
	if cfg.Server.ListenAddress != ":7070" {
		t.Errorf("expected CLI to win for listen address, got %q", cfg.Server.ListenAddress)
	}

	// MetricsPath: YAML value (not overridden by ENV or CLI)
	if cfg.Server.MetricsPath != "/yaml-metrics" {
		t.Errorf("expected YAML value for metrics path, got %q", cfg.Server.MetricsPath)
	}

	// LogLevel: CLI > ENV > YAML
	if cfg.Logging.Level != "error" {
		t.Errorf("expected CLI to win for log level, got %q", cfg.Logging.Level)
	}

	// LogFormat: YAML value (not overridden)
	if cfg.Logging.Format != "json" {
		t.Errorf("expected YAML value for log format, got %q", cfg.Logging.Format)
	}

	// WiFi: ENV > YAML (CLI not set for this test)
	if cfg.Collectors.WiFi.Enabled {
		t.Error("expected ENV to win for wifi collector")
	}

	// Battery: YAML value (not overridden)
	if !cfg.Collectors.Battery.Enabled {
		t.Error("expected YAML value for battery collector")
	}
}

// =============================================================================
// Integration Tests
// =============================================================================

// TestLoadWithOverrides_Integration_EmptyConfigFile tests loading with an empty config file.
func TestLoadWithOverrides_Integration_EmptyConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "empty.yml")
	if err := os.WriteFile(cfgPath, []byte(""), 0600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := LoadWithOverrides(cfgPath, nil)
	if err != nil {
		t.Fatalf("LoadWithOverrides returned error: %v", err)
	}

	// Should get defaults
	if cfg.Server.ListenAddress != "127.0.0.1:10102" {
		t.Errorf("expected default listen address, got %q", cfg.Server.ListenAddress)
	}
}

// TestLoadWithOverrides_Integration_PartialYAML tests loading with partial YAML config.
func TestLoadWithOverrides_Integration_PartialYAML(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "partial.yml")
	content := `
server:
  listen_address: ":9090"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := LoadWithOverrides(cfgPath, nil)
	if err != nil {
		t.Fatalf("LoadWithOverrides returned error: %v", err)
	}

	// YAML value
	if cfg.Server.ListenAddress != ":9090" {
		t.Errorf("expected :9090, got %q", cfg.Server.ListenAddress)
	}

	// Default values for unset fields
	if cfg.Server.MetricsPath != "/metrics" {
		t.Errorf("expected default metrics path, got %q", cfg.Server.MetricsPath)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("expected default log level, got %q", cfg.Logging.Level)
	}
}

// TestLoadWithOverrides_Integration_InstanceLabels tests instance labels via ENV.
func TestLoadWithOverrides_Integration_InstanceLabels(t *testing.T) {
	t.Setenv("DARWIN_EXPORTER_INSTANCE_LABELS", `{"env":"test","region":"us-west"}`)

	cfg, err := LoadWithOverrides("", nil)
	if err != nil {
		t.Fatalf("LoadWithOverrides returned error: %v", err)
	}

	if len(cfg.Instance.Labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(cfg.Instance.Labels))
	}
	if cfg.Instance.Labels["env"] != "test" {
		t.Errorf("expected env label 'test', got %q", cfg.Instance.Labels["env"])
	}
	if cfg.Instance.Labels["region"] != "us-west" {
		t.Errorf("expected region label 'us-west', got %q", cfg.Instance.Labels["region"])
	}
}

// TestLoadWithOverrides_Integration_CollectorBoolFlags tests collector bool flag handling.
func TestLoadWithOverrides_Integration_CollectorBoolFlags(t *testing.T) {
	// Test with CLI flags where HasValue is false (should not override)
	cliFlags := &CLIFlags{
		Collectors: CollectorsCLIFlags{
			WiFi: CollectorBoolFlag{Value: false, HasValue: false},
		},
	}

	cfg, err := LoadWithOverrides("", cliFlags)
	if err != nil {
		t.Fatalf("LoadWithOverrides returned error: %v", err)
	}

	// WiFi should remain enabled (default) since HasValue is false
	if !cfg.Collectors.WiFi.Enabled {
		t.Error("expected wifi collector to remain enabled (default) when HasValue is false")
	}
}

// TestLoadWithOverrides_Integration_InvalidDuration tests handling of invalid duration in ENV.
func TestLoadWithOverrides_Integration_InvalidDuration(t *testing.T) {
	t.Setenv("DARWIN_EXPORTER_SERVER_READ_TIMEOUT", "invalid")

	cfg, err := LoadWithOverrides("", nil)
	if err != nil {
		t.Fatalf("LoadWithOverrides returned error: %v", err)
	}

	// Invalid duration should be ignored, default should be used
	if cfg.Server.ReadTimeout != 30*time.Second {
		t.Errorf("expected default read timeout 30s, got %v", cfg.Server.ReadTimeout)
	}
}

// TestLoadWithOverrides_Integration_InvalidBool tests handling of invalid bool in ENV.
func TestLoadWithOverrides_Integration_InvalidBool(t *testing.T) {
	t.Setenv("DARWIN_EXPORTER_COLLECTORS_WIFI_ENABLED", "invalid")

	cfg, err := LoadWithOverrides("", nil)
	if err != nil {
		t.Fatalf("LoadWithOverrides returned error: %v", err)
	}

	// Invalid bool should be ignored, default should be used
	if !cfg.Collectors.WiFi.Enabled {
		t.Error("expected wifi collector to remain enabled (default) with invalid ENV value")
	}
}

// TestLoadWithOverrides_Integration_InvalidLabelsJSON tests handling of invalid JSON for labels.
func TestLoadWithOverrides_Integration_InvalidLabelsJSON(t *testing.T) {
	t.Setenv("DARWIN_EXPORTER_INSTANCE_LABELS", `{"invalid": json}`)

	_, err := LoadWithOverrides("", nil)
	if err == nil {
		t.Fatal("expected error for invalid DARWIN_EXPORTER_INSTANCE_LABELS JSON")
	}
}

// TestLoadWithOverrides_Validation tests that validation is performed.
func TestLoadWithOverrides_Validation(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "invalid.yml")
	content := `
server:
  listen_address: ""
  metrics_path: "/metrics"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	_, err := LoadWithOverrides(cfgPath, nil)
	if err == nil {
		t.Fatal("expected validation error for empty listen_address, got nil")
	}
}

// TestLoadWithOverrides_InstanceNameFromFile tests reading instance name from file.
func TestLoadWithOverrides_InstanceNameFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	instanceFile := filepath.Join(tmpDir, "instance")
	if err := os.WriteFile(instanceFile, []byte("file-instance\n"), 0600); err != nil {
		t.Fatalf("failed to write instance file: %v", err)
	}

	cfg, err := LoadWithOverrides("", &CLIFlags{
		Instance: InstanceCLIFlags{
			InstanceFile: instanceFile,
		},
	})
	if err != nil {
		t.Fatalf("LoadWithOverrides returned error: %v", err)
	}

	if cfg.Instance.Name != "file-instance" {
		t.Errorf("expected instance name from file 'file-instance', got %q", cfg.Instance.Name)
	}
}
