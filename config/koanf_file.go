package config

import (
	"fmt"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

func loadYAMLWithKoanf(path string, cfg *Config) error {
	k := koanf.New(".")
	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		return fmt.Errorf("parsing config file %s: %w", path, err)
	}

	if k.Exists("server.listen_address") {
		cfg.Server.ListenAddress = k.String("server.listen_address")
	}
	if k.Exists("server.metrics_path") {
		cfg.Server.MetricsPath = k.String("server.metrics_path")
	}
	if k.Exists("server.health_path") {
		cfg.Server.HealthPath = k.String("server.health_path")
	}
	if k.Exists("server.ready_path") {
		cfg.Server.ReadyPath = k.String("server.ready_path")
	}
	if k.Exists("server.read_timeout") {
		if d, err := time.ParseDuration(k.String("server.read_timeout")); err == nil {
			cfg.Server.ReadTimeout = d
		}
	}
	if k.Exists("server.write_timeout") {
		if d, err := time.ParseDuration(k.String("server.write_timeout")); err == nil {
			cfg.Server.WriteTimeout = d
		}
	}

	if k.Exists("logging.level") {
		cfg.Logging.Level = k.String("logging.level")
	}
	if k.Exists("logging.format") {
		cfg.Logging.Format = k.String("logging.format")
	}

	if k.Exists("collectors.wifi.enabled") {
		cfg.Collectors.WiFi.Enabled = k.Bool("collectors.wifi.enabled")
	}
	if k.Exists("collectors.battery.enabled") {
		cfg.Collectors.Battery.Enabled = k.Bool("collectors.battery.enabled")
	}
	if k.Exists("collectors.thermal.enabled") {
		cfg.Collectors.Thermal.Enabled = k.Bool("collectors.thermal.enabled")
	}
	if k.Exists("collectors.wdutil.enabled") {
		cfg.Collectors.Wdutil.Enabled = k.Bool("collectors.wdutil.enabled")
	}

	if k.Exists("instance.name") {
		cfg.Instance.Name = k.String("instance.name")
	}
	if k.Exists("instance.instance_file") {
		cfg.Instance.InstanceFile = k.String("instance.instance_file")
	}
	if labels := k.StringMap("instance.labels"); len(labels) > 0 {
		cfg.Instance.Labels = labels
	}
	if k.Exists("color") {
		cfg.Color = k.String("color")
	}
	return nil
}
