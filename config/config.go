// Package config provides configuration loading and validation for darwin-exporter.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config is the top-level configuration structure.
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Logging    LoggingConfig    `yaml:"logging"`
	Collectors CollectorsConfig `yaml:"collectors"`
	Instance   InstanceConfig   `yaml:"instance"`
	Color      string           `yaml:"color"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	ListenAddress string        `yaml:"listen_address"`
	MetricsPath   string        `yaml:"metrics_path"`
	HealthPath    string        `yaml:"health_path"`
	ReadyPath     string        `yaml:"ready_path"`
	ReadTimeout   time.Duration `yaml:"read_timeout"`
	WriteTimeout  time.Duration `yaml:"write_timeout"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// CollectorsConfig holds per-collector settings.
type CollectorsConfig struct {
	WiFi    WiFiCollectorConfig    `yaml:"wifi"`
	Battery BatteryCollectorConfig `yaml:"battery"`
	Thermal ThermalCollectorConfig `yaml:"thermal"`
	Wdutil  WdutilCollectorConfig  `yaml:"wdutil"`
}

// WiFiCollectorConfig holds WiFi collector settings.
type WiFiCollectorConfig struct {
	Enabled bool `yaml:"enabled"`
}

// BatteryCollectorConfig holds Battery collector settings.
type BatteryCollectorConfig struct {
	Enabled bool `yaml:"enabled"`
}

// ThermalCollectorConfig holds Thermal collector settings.
type ThermalCollectorConfig struct {
	Enabled bool `yaml:"enabled"`
}

// WdutilCollectorConfig holds wdutil collector settings.
type WdutilCollectorConfig struct {
	Enabled bool `yaml:"enabled"`
}

// InstanceConfig holds instance-specific settings.
type InstanceConfig struct {
	Name         string            `yaml:"name"`
	InstanceFile string            `yaml:"instance_file"`
	Labels       map[string]string `yaml:"labels"`
}

// DefaultConfigPath is the default YAML config path shown in CLI help.
const DefaultConfigPath = "~/.config/darwin-exporter/config.yml"

// Load reads configuration from a YAML file. If path is empty, returns defaults.
// If the config file does not exist, returns defaults without error.
func Load(path string) (*Config, error) {
	cfg := defaultConfig()

	if path == "" {
		return cfg, nil
	}

	resolvedPath, err := expandUserPath(path)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(resolvedPath); err != nil {
		// If file does not exist and path is the default, return defaults silently.
		if os.IsNotExist(err) && path == DefaultConfigPath {
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config file %s: %w", resolvedPath, err)
	}
	if err := loadYAMLWithKoanf(resolvedPath, cfg); err != nil {
		return nil, err
	}

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Resolve instance name from file if not set directly in config.
	if cfg.Instance.Name == "" {
		cfg.Instance.Name = readInstanceName(cfg.Instance.InstanceFile)
	}

	return cfg, nil
}

// expandUserPath resolves "~" and "~/" to the current user's home directory.
func expandUserPath(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolving home directory for %q: %w", path, err)
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

// validate checks configuration for required fields and valid values.
func validate(cfg *Config) error {
	if cfg.Server.ListenAddress == "" {
		return fmt.Errorf("server.listen_address must not be empty")
	}
	if cfg.Server.MetricsPath == "" {
		return fmt.Errorf("server.metrics_path must not be empty")
	}
	switch cfg.Logging.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("logging.level must be one of: debug, info, warn, error; got %q", cfg.Logging.Level)
	}
	switch cfg.Logging.Format {
	case "logfmt", "json":
	default:
		return fmt.Errorf("logging.format must be one of: logfmt, json; got %q", cfg.Logging.Format)
	}
	switch cfg.Color {
	case "auto", "always", "never":
	default:
		return fmt.Errorf("color must be one of: auto, always, never; got %q", cfg.Color)
	}
	return nil
}

// readInstanceName reads the machine instance name from the specified file.
// If filePath is empty, it falls back to ~/.instance (default behavior).
// Returns an empty string if the file does not exist, cannot be read, or the
// path contains a directory traversal sequence.
func readInstanceName(filePath string) string {
	if filePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		filePath = home + "/.instance"
	}
	// Reject path traversal attempts: check the original path for ".." segments
	// before cleaning, because filepath.Clean removes ".." and the cleaned path
	// would no longer reveal the traversal.
	if strings.Contains(filePath, "..") {
		return ""
	}
	clean := filepath.Clean(filePath)
	data, err := os.ReadFile(clean)
	if err != nil {
		return ""
	}
	// Trim trailing newlines.
	name := string(data)
	for len(name) > 0 && (name[len(name)-1] == '\n' || name[len(name)-1] == '\r') {
		name = name[:len(name)-1]
	}
	return name
}
