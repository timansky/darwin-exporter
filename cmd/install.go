//go:build darwin

package cmd

import launchd "github.com/timansky/darwin-exporter/pkg/launchd"

// InstallOptions holds parameters for the install subcommand.
type InstallOptions = launchd.InstallOptions

// Install performs the launchd service installation.
func Install(opts InstallOptions) error {
	return launchd.Install(opts, serviceHooks())
}
