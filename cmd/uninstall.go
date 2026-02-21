//go:build darwin

package cmd

import launchd "github.com/timansky/darwin-exporter/pkg/launchd"

// UninstallOptions holds parameters for the uninstall subcommand.
type UninstallOptions = launchd.UninstallOptions

// Uninstall removes the launchd service and associated files.
func Uninstall(opts UninstallOptions) error {
	return launchd.Uninstall(opts, serviceHooks())
}
