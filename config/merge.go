package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

func loadLabelsFromEnv(raw string) (map[string]string, error) {
	labels := make(map[string]string)
	if err := json.Unmarshal([]byte(raw), &labels); err != nil {
		return nil, fmt.Errorf("invalid DARWIN_EXPORTER_INSTANCE_LABELS JSON: %w", err)
	}
	return labels, nil
}

func applyEnvOverrides(cfg *Config) error {
	if cfg == nil {
		return nil
	}

	if v, ok := os.LookupEnv("DARWIN_EXPORTER_SERVER_LISTEN_ADDRESS"); ok && v != "" {
		cfg.Server.ListenAddress = v
	}
	if v, ok := os.LookupEnv("DARWIN_EXPORTER_SERVER_METRICS_PATH"); ok && v != "" {
		cfg.Server.MetricsPath = v
	}
	if v, ok := os.LookupEnv("DARWIN_EXPORTER_SERVER_HEALTH_PATH"); ok && v != "" {
		cfg.Server.HealthPath = v
	}
	if v, ok := os.LookupEnv("DARWIN_EXPORTER_SERVER_READY_PATH"); ok && v != "" {
		cfg.Server.ReadyPath = v
	}
	if v, ok := os.LookupEnv("DARWIN_EXPORTER_SERVER_READ_TIMEOUT"); ok {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Server.ReadTimeout = d
		}
	}
	if v, ok := os.LookupEnv("DARWIN_EXPORTER_SERVER_WRITE_TIMEOUT"); ok {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Server.WriteTimeout = d
		}
	}

	if v, ok := os.LookupEnv("DARWIN_EXPORTER_LOGGING_LEVEL"); ok && v != "" {
		cfg.Logging.Level = v
	}
	if v, ok := os.LookupEnv("DARWIN_EXPORTER_LOGGING_FORMAT"); ok && v != "" {
		cfg.Logging.Format = v
	}
	if v, ok := os.LookupEnv("DARWIN_EXPORTER_COLOR"); ok && v != "" {
		cfg.Color = v
	}

	if v, ok := os.LookupEnv("DARWIN_EXPORTER_COLLECTORS_WIFI_ENABLED"); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Collectors.WiFi.Enabled = b
		}
	}
	if v, ok := os.LookupEnv("DARWIN_EXPORTER_COLLECTORS_BATTERY_ENABLED"); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Collectors.Battery.Enabled = b
		}
	}
	if v, ok := os.LookupEnv("DARWIN_EXPORTER_COLLECTORS_THERMAL_ENABLED"); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Collectors.Thermal.Enabled = b
		}
	}
	if v, ok := os.LookupEnv("DARWIN_EXPORTER_COLLECTORS_WDUTIL_ENABLED"); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Collectors.Wdutil.Enabled = b
		}
	}

	if v, ok := os.LookupEnv("DARWIN_EXPORTER_INSTANCE_NAME"); ok && v != "" {
		cfg.Instance.Name = v
	}
	if v, ok := os.LookupEnv("DARWIN_EXPORTER_INSTANCE_INSTANCE_FILE"); ok && v != "" {
		cfg.Instance.InstanceFile = v
	}
	if v, ok := os.LookupEnv("DARWIN_EXPORTER_INSTANCE_LABELS"); ok && v != "" {
		labels, err := loadLabelsFromEnv(v)
		if err != nil {
			return err
		}
		cfg.Instance.Labels = labels
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
