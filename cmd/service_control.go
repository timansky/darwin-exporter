//go:build darwin

package cmd

import (
	launchd "github.com/timansky/darwin-exporter/pkg/launchd"
)

// ServiceControlOptions holds mode flags for service lifecycle commands.
type ServiceControlOptions = launchd.ServiceControlOptions

// ServiceLogsOptions controls `service logs`.
type ServiceLogsOptions = launchd.ServiceLogsOptions

// ServiceStart starts an installed launchd service.
func ServiceStart(opts ServiceControlOptions) error {
	return launchd.ServiceStart(opts, serviceHooks())
}

// ServiceStop stops a launchd service if it is currently running/loaded.
func ServiceStop(opts ServiceControlOptions) error {
	return launchd.ServiceStop(opts, serviceHooks())
}

// ServiceRestart restarts a launchd service.
func ServiceRestart(opts ServiceControlOptions) error {
	return launchd.ServiceRestart(opts, serviceHooks())
}

// ServiceEnable enables launchd autostart for a service.
func ServiceEnable(opts ServiceControlOptions) error {
	return launchd.ServiceEnable(opts, serviceHooks())
}

// ServiceDisable disables launchd autostart for a service.
func ServiceDisable(opts ServiceControlOptions) error {
	return launchd.ServiceDisable(opts, serviceHooks())
}

// ServiceStatus prints launchd service status.
func ServiceStatus(opts ServiceControlOptions) error {
	return launchd.ServiceStatus(opts, serviceHooks())
}

// ServiceLogs prints the last N lines from service log files.
func ServiceLogs(opts ServiceLogsOptions) error {
	return launchd.ServiceLogs(opts, serviceHooks())
}
