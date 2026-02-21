package config

import "time"

// CLIFlags holds all CLI flag values for configuration override.
type CLIFlags struct {
	Server     ServerCLIFlags
	Logging    LoggingCLIFlags
	Collectors CollectorsCLIFlags
	Instance   InstanceCLIFlags
	Color      string
	ColorSet   bool
}

// ServerCLIFlags holds server-related CLI flags.
type ServerCLIFlags struct {
	ListenAddress    string
	ListenAddressSet bool
	MetricsPath      string
	MetricsPathSet   bool
	HealthPath       string
	HealthPathSet    bool
	ReadyPath        string
	ReadyPathSet     bool
	ReadTimeout      time.Duration
	ReadTimeoutSet   bool
	WriteTimeout     time.Duration
	WriteTimeoutSet  bool
}

// LoggingCLIFlags holds logging-related CLI flags.
type LoggingCLIFlags struct {
	Level     string
	LevelSet  bool
	Format    string
	FormatSet bool
}

// CollectorsCLIFlags holds collector-related CLI flags.
type CollectorsCLIFlags struct {
	WiFi    CollectorBoolFlag
	Battery CollectorBoolFlag
	Thermal CollectorBoolFlag
	Wdutil  CollectorBoolFlag
}

// CollectorBoolFlag is a flag wrapper for collector enabled status.
type CollectorBoolFlag struct {
	Value    bool
	HasValue bool
}

// InstanceCLIFlags holds instance-related CLI flags.
type InstanceCLIFlags struct {
	Name            string
	NameSet         bool
	InstanceFile    string
	InstanceFileSet bool
}
