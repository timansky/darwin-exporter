package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	koanfenv "github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/v2"
)

func loadLabelsFromEnv(raw string) (map[string]string, error) {
	labels := make(map[string]string)
	if err := json.Unmarshal([]byte(raw), &labels); err != nil {
		return nil, fmt.Errorf("invalid DARWIN_EXPORTER_INSTANCE_LABELS JSON: %w", err)
	}
	return labels, nil
}

var envVarToConfigPath = map[string]string{
	"DARWIN_EXPORTER_SERVER_LISTEN_ADDRESS":      "server.listen_address",
	"DARWIN_EXPORTER_SERVER_METRICS_PATH":        "server.metrics_path",
	"DARWIN_EXPORTER_SERVER_HEALTH_PATH":         "server.health_path",
	"DARWIN_EXPORTER_SERVER_READY_PATH":          "server.ready_path",
	"DARWIN_EXPORTER_SERVER_READ_TIMEOUT":        "server.read_timeout",
	"DARWIN_EXPORTER_SERVER_WRITE_TIMEOUT":       "server.write_timeout",
	"DARWIN_EXPORTER_LOGGING_LEVEL":              "logging.level",
	"DARWIN_EXPORTER_LOGGING_FORMAT":             "logging.format",
	"DARWIN_EXPORTER_COLOR":                      "color",
	"DARWIN_EXPORTER_COLLECTORS_WIFI_ENABLED":    "collectors.wifi.enabled",
	"DARWIN_EXPORTER_COLLECTORS_BATTERY_ENABLED": "collectors.battery.enabled",
	"DARWIN_EXPORTER_COLLECTORS_THERMAL_ENABLED": "collectors.thermal.enabled",
	"DARWIN_EXPORTER_COLLECTORS_WDUTIL_ENABLED":  "collectors.wdutil.enabled",
	"DARWIN_EXPORTER_INSTANCE_NAME":              "instance.name",
	"DARWIN_EXPORTER_INSTANCE_INSTANCE_FILE":     "instance.instance_file",
	"DARWIN_EXPORTER_INSTANCE_LABELS":            "instance.labels",
}

func hasKnownEnvOverrides() bool {
	for envName := range envVarToConfigPath {
		if _, ok := os.LookupEnv(envName); ok {
			return true
		}
	}
	return false
}

func fallbackStringOverride(cfg *Config, envName string) string {
	switch envName {
	case "DARWIN_EXPORTER_SERVER_LISTEN_ADDRESS":
		return cfg.Server.ListenAddress
	case "DARWIN_EXPORTER_SERVER_METRICS_PATH":
		return cfg.Server.MetricsPath
	case "DARWIN_EXPORTER_SERVER_HEALTH_PATH":
		return cfg.Server.HealthPath
	case "DARWIN_EXPORTER_SERVER_READY_PATH":
		return cfg.Server.ReadyPath
	case "DARWIN_EXPORTER_LOGGING_LEVEL":
		return cfg.Logging.Level
	case "DARWIN_EXPORTER_LOGGING_FORMAT":
		return cfg.Logging.Format
	case "DARWIN_EXPORTER_COLOR":
		return cfg.Color
	case "DARWIN_EXPORTER_INSTANCE_NAME":
		return cfg.Instance.Name
	case "DARWIN_EXPORTER_INSTANCE_INSTANCE_FILE":
		return cfg.Instance.InstanceFile
	default:
		return ""
	}
}

func fallbackBoolOverride(cfg *Config, envName string) bool {
	switch envName {
	case "DARWIN_EXPORTER_COLLECTORS_WIFI_ENABLED":
		return cfg.Collectors.WiFi.Enabled
	case "DARWIN_EXPORTER_COLLECTORS_BATTERY_ENABLED":
		return cfg.Collectors.Battery.Enabled
	case "DARWIN_EXPORTER_COLLECTORS_THERMAL_ENABLED":
		return cfg.Collectors.Thermal.Enabled
	case "DARWIN_EXPORTER_COLLECTORS_WDUTIL_ENABLED":
		return cfg.Collectors.Wdutil.Enabled
	default:
		return false
	}
}

func transformEnvVar(cfg *Config, labelsOverride map[string]string, envName, rawValue string) (string, any) {
	path, ok := envVarToConfigPath[envName]
	if !ok {
		key := strings.TrimPrefix(envName, "DARWIN_EXPORTER_")
		key = strings.ToLower(strings.ReplaceAll(key, "_", "."))
		return key, rawValue
	}

	switch envName {
	case "DARWIN_EXPORTER_SERVER_READ_TIMEOUT":
		if d, err := time.ParseDuration(rawValue); err == nil {
			return path, d
		}
		return path, cfg.Server.ReadTimeout
	case "DARWIN_EXPORTER_SERVER_WRITE_TIMEOUT":
		if d, err := time.ParseDuration(rawValue); err == nil {
			return path, d
		}
		return path, cfg.Server.WriteTimeout
	case "DARWIN_EXPORTER_COLLECTORS_WIFI_ENABLED",
		"DARWIN_EXPORTER_COLLECTORS_BATTERY_ENABLED",
		"DARWIN_EXPORTER_COLLECTORS_THERMAL_ENABLED",
		"DARWIN_EXPORTER_COLLECTORS_WDUTIL_ENABLED":
		if b, err := strconv.ParseBool(rawValue); err == nil {
			return path, b
		}
		return path, fallbackBoolOverride(cfg, envName)
	case "DARWIN_EXPORTER_INSTANCE_LABELS":
		if labelsOverride != nil {
			return path, labelsOverride
		}
		return path, cfg.Instance.Labels
	case "DARWIN_EXPORTER_SERVER_LISTEN_ADDRESS",
		"DARWIN_EXPORTER_SERVER_METRICS_PATH",
		"DARWIN_EXPORTER_SERVER_HEALTH_PATH",
		"DARWIN_EXPORTER_SERVER_READY_PATH",
		"DARWIN_EXPORTER_LOGGING_LEVEL",
		"DARWIN_EXPORTER_LOGGING_FORMAT",
		"DARWIN_EXPORTER_COLOR",
		"DARWIN_EXPORTER_INSTANCE_NAME",
		"DARWIN_EXPORTER_INSTANCE_INSTANCE_FILE":
		if rawValue == "" {
			return path, fallbackStringOverride(cfg, envName)
		}
		return path, rawValue
	default:
		return path, rawValue
	}
}

func applyEnvOverrides(cfg *Config) error {
	if cfg == nil {
		return nil
	}

	if !hasKnownEnvOverrides() {
		return nil
	}

	var labelsOverride map[string]string
	if raw, ok := os.LookupEnv("DARWIN_EXPORTER_INSTANCE_LABELS"); ok && raw != "" {
		labels, err := loadLabelsFromEnv(raw)
		if err != nil {
			return err
		}
		labelsOverride = labels
	}

	k := koanf.New(".")
	if err := k.Load(koanfenv.Provider(".", koanfenv.Opt{
		Prefix: "DARWIN_EXPORTER_",
		TransformFunc: func(envName, rawValue string) (string, any) {
			return transformEnvVar(cfg, labelsOverride, envName, rawValue)
		},
	}), nil); err != nil {
		return fmt.Errorf("loading environment overrides: %w", err)
	}

	if err := unmarshalConfigWithKoanf(k, cfg); err != nil {
		return err
	}

	return nil
}

func applyCLIOverrides(cfg *Config, flags *CLIFlags) {
	if cfg == nil || flags == nil {
		return
	}

	if flags.Server.ListenAddressSet || flags.Server.ListenAddress != "" {
		cfg.Server.ListenAddress = flags.Server.ListenAddress
	}
	if flags.Server.MetricsPathSet || flags.Server.MetricsPath != "" {
		cfg.Server.MetricsPath = flags.Server.MetricsPath
	}
	if flags.Server.HealthPathSet || flags.Server.HealthPath != "" {
		cfg.Server.HealthPath = flags.Server.HealthPath
	}
	if flags.Server.ReadyPathSet || flags.Server.ReadyPath != "" {
		cfg.Server.ReadyPath = flags.Server.ReadyPath
	}
	if flags.Server.ReadTimeoutSet || flags.Server.ReadTimeout != 0 {
		cfg.Server.ReadTimeout = flags.Server.ReadTimeout
	}
	if flags.Server.WriteTimeoutSet || flags.Server.WriteTimeout != 0 {
		cfg.Server.WriteTimeout = flags.Server.WriteTimeout
	}

	if flags.Logging.LevelSet || flags.Logging.Level != "" {
		cfg.Logging.Level = flags.Logging.Level
	}
	if flags.Logging.FormatSet || flags.Logging.Format != "" {
		cfg.Logging.Format = flags.Logging.Format
	}

	if flags.Collectors.WiFi.HasValue {
		cfg.Collectors.WiFi.Enabled = flags.Collectors.WiFi.Value
	}
	if flags.Collectors.Battery.HasValue {
		cfg.Collectors.Battery.Enabled = flags.Collectors.Battery.Value
	}
	if flags.Collectors.Thermal.HasValue {
		cfg.Collectors.Thermal.Enabled = flags.Collectors.Thermal.Value
	}
	if flags.Collectors.Wdutil.HasValue {
		cfg.Collectors.Wdutil.Enabled = flags.Collectors.Wdutil.Value
	}

	if flags.Instance.NameSet || flags.Instance.Name != "" {
		cfg.Instance.Name = flags.Instance.Name
	}
	if flags.Instance.InstanceFileSet || flags.Instance.InstanceFile != "" {
		cfg.Instance.InstanceFile = flags.Instance.InstanceFile
	}
	if flags.ColorSet || flags.Color != "" {
		cfg.Color = flags.Color
	}
}

// LoadWithOverrides loads configuration with support for overrides.
// Priority: CLI > ENV > YAML > defaults
func LoadWithOverrides(path string, cliFlags *CLIFlags) (*Config, error) {
	cfg := defaultConfig()

	if path != "" {
		resolvedPath, err := expandUserPath(path)
		if err != nil {
			return nil, err
		}

		if _, err := os.Stat(resolvedPath); err != nil {
			if !os.IsNotExist(err) || path != DefaultConfigPath {
				return nil, fmt.Errorf("reading config file %s: %w", resolvedPath, err)
			}
		} else {
			if err := loadYAMLWithKoanf(resolvedPath, cfg); err != nil {
				return nil, err
			}
		}
	}

	if err := applyEnvOverrides(cfg); err != nil {
		return nil, err
	}
	applyCLIOverrides(cfg, cliFlags)

	if err := validate(cfg); err != nil {
		return nil, err
	}

	if cfg.Instance.Name == "" {
		cfg.Instance.Name = readInstanceName(cfg.Instance.InstanceFile)
	}

	return cfg, nil
}
