// darwin-exporter: Prometheus exporter for macOS-specific metrics.
//
// Exports WiFi, Battery, and Thermal metrics that are not available
// in node_exporter. Runs on port 10102 by default.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/timansky/darwin-exporter/cmd"
	"github.com/timansky/darwin-exporter/collector"
	"github.com/timansky/darwin-exporter/config"
	"github.com/timansky/darwin-exporter/server"
	"github.com/timansky/darwin-exporter/version"
)

type cliApp struct {
	configFile   *string
	cliFlags     *config.CLIFlags
	colorMode    string
	colorSet     bool
	verifyConfig bool

	rootCmd               *cobra.Command
	runCmd                *cobra.Command
	completionCmd         *cobra.Command
	serviceCmd            *cobra.Command
	serviceInstallCmd     *cobra.Command
	serviceInstallType    *string
	serviceInstallConfig  *string
	serviceInstallLogDir  *string
	serviceInstallBinPath *string

	serviceUninstallCmd    *cobra.Command
	serviceUninstallType   *string
	serviceUninstallPurge  *bool
	serviceUninstallConfig *string
	serviceUninstallLogDir *string

	serviceStartCmd    *cobra.Command
	serviceStartType   *string
	serviceStatusCmd   *cobra.Command
	serviceStatusType  *string
	serviceLogsCmd     *cobra.Command
	serviceLogsType    *string
	serviceLogsLines   *int
	serviceStopCmd     *cobra.Command
	serviceStopType    *string
	serviceRestartCmd  *cobra.Command
	serviceRestartType *string
	serviceEnableCmd   *cobra.Command
	serviceEnableType  *string
	serviceDisableCmd  *cobra.Command
	serviceDisableType *string
}

func modeFlagsFromType(modeType, defaultType string) (bool, bool, error) {
	t := strings.ToLower(strings.TrimSpace(modeType))
	if t == "" {
		t = defaultType
	}
	switch t {
	case "sudo":
		return true, false, nil
	case "root":
		return false, true, nil
	default:
		return false, false, fmt.Errorf("invalid --type=%q (allowed: sudo, root)", modeType)
	}
}

func newCLIApp() *cliApp {
	return &cliApp{
		cliFlags:   &config.CLIFlags{},
		configFile: ptr(config.DefaultConfigPath),
	}
}

func buildCLI() (*cobra.Command, *cliApp) {
	cli := newCLIApp()
	var showVersion bool

	rootCmd := buildRootCommand(cli, &showVersion)
	runCmd := buildRunCommand(cli)
	completionCmd := buildCompletionCommand(cli, rootCmd)
	serviceCmd := buildServiceCommand(cli, rootCmd)

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(completionCmd)
	rootCmd.AddCommand(serviceCmd)

	cli.rootCmd = rootCmd
	cli.runCmd = runCmd
	cli.completionCmd = completionCmd
	cli.serviceCmd = serviceCmd
	return rootCmd, cli
}

func buildRootCommand(cli *cliApp, showVersion *bool) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "darwin-exporter",
		Short: "Prometheus exporter for macOS-specific metrics.",
		RunE: func(c *cobra.Command, _ []string) error {
			applyColorMode(cli, *cli.configFile, c.Flags().Lookup("color").Changed)
			if showVersion != nil && *showVersion {
				fmt.Fprintf(os.Stdout, "darwin-exporter %s commit=%s built=%s\n", version.Version, version.Commit, version.BuildDate)
				return nil
			}
			captureRunFlagChanges(c, cli.cliFlags)
			if cli.verifyConfig {
				return verifyConfigOnly(*cli.configFile, cli.cliFlags)
			}
			runExporter(*cli.configFile, cli.cliFlags)
			return nil
		},
		SilenceUsage: true,
	}
	rootCmd.PersistentFlags().StringVar(&cli.colorMode, "color", "auto", "Color output: auto|always|never.")
	rootCmd.PersistentFlags().StringVarP(cli.configFile, "config", "c", config.DefaultConfigPath, "Path to YAML config file.")
	rootCmd.PersistentFlags().BoolVar(&cli.verifyConfig, "verify-config", false, "Validate configuration and exit.")
	if showVersion != nil {
		rootCmd.PersistentFlags().BoolVarP(showVersion, "version", "v", false, "Show version and exit.")
	}
	return rootCmd
}

func buildRunCommand(cli *cliApp) *cobra.Command {
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run the exporter (default).",
		RunE: func(c *cobra.Command, _ []string) error {
			applyColorMode(cli, *cli.configFile, c.Flags().Lookup("color").Changed)
			captureRunFlagChanges(c, cli.cliFlags)
			if cli.verifyConfig {
				return verifyConfigOnly(*cli.configFile, cli.cliFlags)
			}
			runExporter(*cli.configFile, cli.cliFlags)
			return nil
		},
	}
	addRunFlags(runCmd, cli.cliFlags)
	return runCmd
}

func buildCompletionCommand(cli *cliApp, rootCmd *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "completion [bash|zsh]",
		Short: "Generate shell completion script.",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			applyRootColorMode(cli, rootCmd)
			return emitCompletionScript(rootCmd, args[0])
		},
	}
}

func buildServiceCommand(cli *cliApp, rootCmd *cobra.Command) *cobra.Command {
	serviceCmd := &cobra.Command{Use: "service", Short: "Manage launchd service."}

	serviceInstallCmd := buildServiceInstallCommand(cli, rootCmd)
	serviceUninstallCmd := buildServiceUninstallCommand(cli, rootCmd)

	cli.serviceStartType = ptr("sudo")
	serviceStartCmd := buildServiceControlCommand(cli, rootCmd, "start", "Start service.", cli.serviceStartType, cmd.ServiceStart)
	cli.serviceStartCmd = serviceStartCmd

	cli.serviceStatusType = ptr("sudo")
	serviceStatusCmd := buildServiceControlCommand(cli, rootCmd, "status", "Show service status.", cli.serviceStatusType, cmd.ServiceStatus)
	cli.serviceStatusCmd = serviceStatusCmd

	cli.serviceStopType = ptr("sudo")
	serviceStopCmd := buildServiceControlCommand(cli, rootCmd, "stop", "Stop service.", cli.serviceStopType, cmd.ServiceStop)
	cli.serviceStopCmd = serviceStopCmd

	cli.serviceRestartType = ptr("sudo")
	serviceRestartCmd := buildServiceControlCommand(cli, rootCmd, "restart", "Restart service.", cli.serviceRestartType, cmd.ServiceRestart)
	cli.serviceRestartCmd = serviceRestartCmd

	cli.serviceEnableType = ptr("sudo")
	serviceEnableCmd := buildServiceControlCommand(cli, rootCmd, "enable", "Enable service autostart.", cli.serviceEnableType, cmd.ServiceEnable)
	cli.serviceEnableCmd = serviceEnableCmd

	cli.serviceDisableType = ptr("sudo")
	serviceDisableCmd := buildServiceControlCommand(cli, rootCmd, "disable", "Disable service autostart.", cli.serviceDisableType, cmd.ServiceDisable)
	cli.serviceDisableCmd = serviceDisableCmd

	serviceLogsCmd := buildServiceLogsCommand(cli, rootCmd)

	serviceCmd.AddCommand(serviceInstallCmd)
	serviceCmd.AddCommand(serviceUninstallCmd)
	serviceCmd.AddCommand(serviceStartCmd)
	serviceCmd.AddCommand(serviceStatusCmd)
	serviceCmd.AddCommand(serviceStopCmd)
	serviceCmd.AddCommand(serviceRestartCmd)
	serviceCmd.AddCommand(serviceEnableCmd)
	serviceCmd.AddCommand(serviceDisableCmd)
	serviceCmd.AddCommand(serviceLogsCmd)

	cli.serviceInstallCmd = serviceInstallCmd
	cli.serviceUninstallCmd = serviceUninstallCmd
	cli.serviceLogsCmd = serviceLogsCmd
	return serviceCmd
}

func buildServiceInstallCommand(cli *cliApp, rootCmd *cobra.Command) *cobra.Command {
	serviceInstallCmd := &cobra.Command{
		Use:   "install",
		Short: "Install as launchd service.",
		RunE: func(_ *cobra.Command, _ []string) error {
			applyRootColorMode(cli, rootCmd)
			sudoMode, rootMode, err := modeFlagsFromType(*cli.serviceInstallType, "sudo")
			if err != nil {
				return fmt.Errorf("darwin-exporter service install: %w", err)
			}
			if err := cmd.Install(cmd.InstallOptions{
				SudoFlag: sudoMode,
				RootFlag: rootMode,
				Config:   *cli.serviceInstallConfig,
				LogDir:   *cli.serviceInstallLogDir,
				BinPath:  *cli.serviceInstallBinPath,
			}); err != nil {
				return fmt.Errorf("darwin-exporter service install: %w", err)
			}
			return nil
		},
	}
	cli.serviceInstallType = serviceInstallCmd.Flags().String("type", "sudo", "Install mode: sudo|root.")
	cli.serviceInstallConfig = serviceInstallCmd.Flags().String("config", "", "Path to config YAML (embedded in service plist).")
	cli.serviceInstallLogDir = serviceInstallCmd.Flags().String("log-dir", "", "Log directory (default: mode-specific).")
	cli.serviceInstallBinPath = serviceInstallCmd.Flags().String("bin-path", "", "Path to darwin-exporter binary (default: current executable).")
	return serviceInstallCmd
}

func buildServiceUninstallCommand(cli *cliApp, rootCmd *cobra.Command) *cobra.Command {
	serviceUninstallCmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall launchd service.",
		RunE: func(_ *cobra.Command, _ []string) error {
			applyRootColorMode(cli, rootCmd)
			sudoMode, rootMode, err := modeFlagsFromType(*cli.serviceUninstallType, "sudo")
			if err != nil {
				return fmt.Errorf("darwin-exporter service uninstall: %w", err)
			}
			if err := cmd.Uninstall(cmd.UninstallOptions{
				SudoFlag: sudoMode,
				RootFlag: rootMode,
				Purge:    *cli.serviceUninstallPurge,
				Config:   *cli.serviceUninstallConfig,
				LogDir:   *cli.serviceUninstallLogDir,
			}); err != nil {
				return fmt.Errorf("darwin-exporter service uninstall: %w", err)
			}
			return nil
		},
	}
	cli.serviceUninstallType = serviceUninstallCmd.Flags().String("type", "sudo", "Uninstall mode: sudo|root.")
	cli.serviceUninstallPurge = serviceUninstallCmd.Flags().Bool("purge", false, "Also remove config file and log directory.")
	cli.serviceUninstallConfig = serviceUninstallCmd.Flags().String("config", "", "Config file path to remove (used with --purge).")
	cli.serviceUninstallLogDir = serviceUninstallCmd.Flags().String("log-dir", "", "Log directory to remove (used with --purge).")
	return serviceUninstallCmd
}

func buildServiceControlCommand(
	cli *cliApp,
	rootCmd *cobra.Command,
	use, desc string,
	mode *string,
	fn func(cmd.ServiceControlOptions) error,
) *cobra.Command {
	c := &cobra.Command{
		Use:   use,
		Short: desc,
		RunE: func(_ *cobra.Command, _ []string) error {
			applyRootColorMode(cli, rootCmd)
			sudoMode, rootMode, err := modeFlagsFromType(*mode, "sudo")
			if err != nil {
				return fmt.Errorf("darwin-exporter service %s: %w", use, err)
			}
			if err := fn(cmd.ServiceControlOptions{SudoFlag: sudoMode, RootFlag: rootMode}); err != nil {
				return fmt.Errorf("darwin-exporter service %s: %w", use, err)
			}
			return nil
		},
	}
	*mode = "sudo"
	modeLabel := use
	if modeLabel != "" {
		modeLabel = strings.ToUpper(modeLabel[:1]) + modeLabel[1:]
	}
	c.Flags().StringVar(mode, "type", "sudo", modeLabel+" mode: sudo|root.")
	return c
}

func buildServiceLogsCommand(cli *cliApp, rootCmd *cobra.Command) *cobra.Command {
	serviceLogsCmd := &cobra.Command{
		Use:   "logs",
		Short: "Show service logs.",
		RunE: func(_ *cobra.Command, _ []string) error {
			applyRootColorMode(cli, rootCmd)
			sudoMode, rootMode, err := modeFlagsFromType(*cli.serviceLogsType, "sudo")
			if err != nil {
				return fmt.Errorf("darwin-exporter service logs: %w", err)
			}
			if err := cmd.ServiceLogs(cmd.ServiceLogsOptions{
				ServiceControlOptions: cmd.ServiceControlOptions{SudoFlag: sudoMode, RootFlag: rootMode},
				Lines:                 *cli.serviceLogsLines,
			}); err != nil {
				return fmt.Errorf("darwin-exporter service logs: %w", err)
			}
			return nil
		},
	}
	cli.serviceLogsType = serviceLogsCmd.Flags().String("type", "sudo", "Logs mode: sudo|root.")
	cli.serviceLogsLines = serviceLogsCmd.Flags().Int("lines", 100, "Show last N lines from each log file.")
	return serviceLogsCmd
}

func applyColorMode(cli *cliApp, configPath string, colorChanged bool) {
	cli.colorSet = colorChanged
	cmd.SetColorMode(resolveColorMode(configPath, cli.colorMode, cli.colorSet))
}

func applyRootColorMode(cli *cliApp, rootCmd *cobra.Command) {
	colorChanged := false
	if rootCmd != nil {
		if colorFlag := rootCmd.PersistentFlags().Lookup("color"); colorFlag != nil {
			colorChanged = colorFlag.Changed
		}
	}
	applyColorMode(cli, config.DefaultConfigPath, colorChanged)
}

func main() {
	root, _ := buildCLI()
	root.SetArgs(os.Args[1:])
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func ptr[T any](v T) *T {
	return &v
}

func addRunFlags(runCmd *cobra.Command, flags *config.CLIFlags) {
	runCmd.Flags().StringVar(&flags.Server.ListenAddress, "server.listen-address", "", "Address to listen on for metrics.")
	runCmd.Flags().StringVar(&flags.Server.MetricsPath, "server.metrics-path", "", "Path under which to expose metrics.")
	runCmd.Flags().StringVar(&flags.Server.HealthPath, "server.health-path", "", "Path under which to expose health endpoint.")
	runCmd.Flags().StringVar(&flags.Server.ReadyPath, "server.ready-path", "", "Path under which to expose ready endpoint.")
	runCmd.Flags().DurationVar(&flags.Server.ReadTimeout, "server.read-timeout", 0, "Read timeout for HTTP server.")
	runCmd.Flags().DurationVar(&flags.Server.WriteTimeout, "server.write-timeout", 0, "Write timeout for HTTP server.")
	runCmd.Flags().StringVar(&flags.Logging.Level, "logging.level", "", "Log level (debug, info, warn, error).")
	runCmd.Flags().StringVar(&flags.Logging.Format, "logging.format", "", "Log format (logfmt, json).")
	runCmd.Flags().BoolVar(&flags.Collectors.WiFi.Value, "collectors.wifi.enabled", false, "Enable WiFi collector.")
	runCmd.Flags().BoolVar(&flags.Collectors.Battery.Value, "collectors.battery.enabled", false, "Enable Battery collector.")
	runCmd.Flags().BoolVar(&flags.Collectors.Thermal.Value, "collectors.thermal.enabled", false, "Enable Thermal collector.")
	runCmd.Flags().BoolVar(&flags.Collectors.Wdutil.Value, "collectors.wdutil.enabled", false, "Enable Wdutil collector (requires root/sudo).")
	runCmd.Flags().StringVar(&flags.Instance.Name, "instance.name", "", "Instance name (overrides file-based name).")
	runCmd.Flags().StringVar(&flags.Instance.InstanceFile, "instance.instance-file", "", "Path to file containing instance name.")
}

func verifyConfigOnly(configFile string, cliFlags *config.CLIFlags) error {
	if _, err := config.LoadWithOverrides(configFile, cliFlags); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Configuration is valid: %s\n", configFile)
	return nil
}

func captureRunFlagChanges(c *cobra.Command, flags *config.CLIFlags) {
	if flags == nil || c == nil {
		return
	}
	f := c.Flags()
	flags.Server.ListenAddressSet = f.Changed("server.listen-address")
	flags.Server.MetricsPathSet = f.Changed("server.metrics-path")
	flags.Server.HealthPathSet = f.Changed("server.health-path")
	flags.Server.ReadyPathSet = f.Changed("server.ready-path")
	flags.Server.ReadTimeoutSet = f.Changed("server.read-timeout")
	flags.Server.WriteTimeoutSet = f.Changed("server.write-timeout")
	flags.Logging.LevelSet = f.Changed("logging.level")
	flags.Logging.FormatSet = f.Changed("logging.format")
	flags.Collectors.WiFi.HasValue = f.Changed("collectors.wifi.enabled")
	flags.Collectors.Battery.HasValue = f.Changed("collectors.battery.enabled")
	flags.Collectors.Thermal.HasValue = f.Changed("collectors.thermal.enabled")
	flags.Collectors.Wdutil.HasValue = f.Changed("collectors.wdutil.enabled")
	flags.Instance.NameSet = f.Changed("instance.name")
	flags.Instance.InstanceFileSet = f.Changed("instance.instance-file")
}

func resolveColorMode(configPath, explicit string, explicitSet bool) string {
	if explicitSet && explicit != "" {
		return explicit
	}
	cfg, err := config.LoadWithOverrides(configPath, nil)
	if err == nil && cfg != nil && cfg.Color != "" {
		return cfg.Color
	}
	return "auto"
}

func emitCompletionScript(root *cobra.Command, shell string) error {
	switch strings.ToLower(strings.TrimSpace(shell)) {
	case "bash":
		return root.GenBashCompletion(os.Stdout)
	case "zsh":
		return root.GenZshCompletion(os.Stdout)
	default:
		return fmt.Errorf("unsupported shell %q (use bash or zsh)", shell)
	}
}

// runExporter starts the Prometheus metrics server.
func runExporter(configFile string, cliFlags *config.CLIFlags) {
	// Bootstrap logger for early startup errors (before config is parsed).
	bootstrapLog := logrus.New()

	// Load configuration with overrides (CLI > ENV > YAML > defaults).
	cfg, err := config.LoadWithOverrides(configFile, cliFlags)
	if err != nil {
		bootstrapLog.WithError(err).Fatal("failed to load configuration")
	}

	// Set up logger.
	log := newLogger(cfg)

	log.WithFields(logrus.Fields{
		"version": version.Version,
		"commit":  version.Commit,
		"config":  configFile,
	}).Info("starting darwin-exporter")

	// Create Prometheus registry.
	reg := prometheus.NewRegistry()

	// Register standard Go and process collectors.
	reg.MustRegister(
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
	)

	// Build collector registry.
	collReg := collector.NewRegistry(log)

	if cfg.Collectors.WiFi.Enabled {
		wifiCollector := collector.NewWiFiCollector(log)
		collReg.Register("wifi", wifiCollector)
		log.Debug("registered wifi collector")
	}

	if cfg.Collectors.Battery.Enabled {
		batteryCollector := collector.NewBatteryCollector(log)
		collReg.Register("battery", batteryCollector)
		log.Debug("registered battery collector")
	}

	if cfg.Collectors.Thermal.Enabled {
		thermalCollector := collector.NewThermalCollector(log)
		collReg.Register("thermal", thermalCollector)
		log.Debug("registered thermal collector")
	}

	if cfg.Collectors.Wdutil.Enabled {
		wdutilCollector := collector.NewWdutilCollector(log)
		collReg.Register("wdutil", wdutilCollector)
		log.Debug("registered wdutil collector")
	}

	// Register the collector registry with Prometheus (unchecked).
	if err := reg.Register(collReg); err != nil {
		log.WithError(err).Fatal("failed to register collectors")
	}

	// Create and start HTTP server.
	srv := server.New(cfg, log, reg)

	// Handle shutdown signals.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.WithError(err).Fatal("server stopped unexpectedly")
		}
	}()

	log.WithField("address", cfg.Server.ListenAddress).
		Info("darwin-exporter running")

	// Block until shutdown signal.
	<-quit
	log.Info("received shutdown signal")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.WithError(err).Error("graceful shutdown failed")
	}

	collReg.Close()

	log.Info("darwin-exporter stopped")
}

// newLogger configures and returns a logrus logger based on the config.
func newLogger(cfg *config.Config) *logrus.Logger {
	log := logrus.New()
	log.SetOutput(os.Stdout)

	level, err := logrus.ParseLevel(cfg.Logging.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	log.SetLevel(level)

	switch cfg.Logging.Format {
	case "json":
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime: "ts",
			},
		})
	default:
		log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: time.RFC3339,
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime: "ts",
			},
		})
	}

	return log
}
