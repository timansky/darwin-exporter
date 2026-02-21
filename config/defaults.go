package config

import "time"

// defaultConfig returns a Config populated with default values.
func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			ListenAddress: "127.0.0.1:10102",
			MetricsPath:   "/metrics",
			HealthPath:    "/health",
			ReadyPath:     "/ready",
			ReadTimeout:   30 * time.Second,
			WriteTimeout:  30 * time.Second,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "logfmt",
		},
		Collectors: CollectorsConfig{
			WiFi: WiFiCollectorConfig{
				Enabled: true,
			},
			Battery: BatteryCollectorConfig{
				Enabled: true,
			},
			Thermal: ThermalCollectorConfig{
				Enabled: true,
			},
			Wdutil: WdutilCollectorConfig{
				Enabled: false,
			},
		},
		Instance: InstanceConfig{
			Labels: make(map[string]string),
		},
		Color: "auto",
	}
}
