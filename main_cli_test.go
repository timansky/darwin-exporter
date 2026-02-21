package main

import "testing"

func parseCLI(t *testing.T, args []string) (*cliApp, string) {
	t.Helper()
	root, cli := buildCLI()
	c, rem, err := root.Find(args)
	if err != nil {
		t.Fatalf("find command: %v", err)
	}
	if err := c.ParseFlags(rem); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	return cli, c.CommandPath()
}

func TestBuildCLI_ServiceSubcommandsParse(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		verify func(t *testing.T, path string, cli *cliApp)
	}{
		{
			name: "service install root type",
			args: []string{"service", "install", "--type=root"},
			verify: func(t *testing.T, path string, cli *cliApp) {
				if path != cli.serviceInstallCmd.CommandPath() {
					t.Fatalf("parsed command = %q, want %q", path, cli.serviceInstallCmd.CommandPath())
				}
				if *cli.serviceInstallType != "root" {
					t.Fatalf("install type = %q, want root", *cli.serviceInstallType)
				}
			},
		},
		{
			name: "service install default type",
			args: []string{"service", "install"},
			verify: func(t *testing.T, path string, cli *cliApp) {
				if path != cli.serviceInstallCmd.CommandPath() {
					t.Fatalf("parsed command = %q, want %q", path, cli.serviceInstallCmd.CommandPath())
				}
				if *cli.serviceInstallType != "sudo" {
					t.Fatalf("install type = %q, want sudo", *cli.serviceInstallType)
				}
			},
		},
		{
			name: "service lifecycle disable",
			args: []string{"service", "disable", "--type=sudo"},
			verify: func(t *testing.T, path string, cli *cliApp) {
				if path != cli.serviceDisableCmd.CommandPath() {
					t.Fatalf("parsed command = %q, want %q", path, cli.serviceDisableCmd.CommandPath())
				}
			},
		},
		{
			name: "service lifecycle status",
			args: []string{"service", "status", "--type=root"},
			verify: func(t *testing.T, path string, cli *cliApp) {
				if path != cli.serviceStatusCmd.CommandPath() {
					t.Fatalf("parsed command = %q, want %q", path, cli.serviceStatusCmd.CommandPath())
				}
				if *cli.serviceStatusType != "root" {
					t.Fatalf("status type = %q, want root", *cli.serviceStatusType)
				}
			},
		},
		{
			name: "service lifecycle logs",
			args: []string{"service", "logs", "--type=sudo", "--lines=25"},
			verify: func(t *testing.T, path string, cli *cliApp) {
				if path != cli.serviceLogsCmd.CommandPath() {
					t.Fatalf("parsed command = %q, want %q", path, cli.serviceLogsCmd.CommandPath())
				}
				if *cli.serviceLogsType != "sudo" {
					t.Fatalf("logs type = %q, want sudo", *cli.serviceLogsType)
				}
				if *cli.serviceLogsLines != 25 {
					t.Fatalf("logs lines = %d, want 25", *cli.serviceLogsLines)
				}
			},
		},
		{
			name: "completion zsh",
			args: []string{"completion", "zsh"},
			verify: func(t *testing.T, path string, cli *cliApp) {
				if path != cli.completionCmd.CommandPath() {
					t.Fatalf("parsed command = %q, want %q", path, cli.completionCmd.CommandPath())
				}
			},
		},
		{
			name: "global color flag parse",
			args: []string{"--color=never", "service", "status"},
			verify: func(t *testing.T, path string, cli *cliApp) {
				if path != cli.serviceStatusCmd.CommandPath() {
					t.Fatalf("parsed command = %q, want %q", path, cli.serviceStatusCmd.CommandPath())
				}
				if cli.colorMode != "never" {
					t.Fatalf("color mode = %q, want never", cli.colorMode)
				}
			},
		},
		{
			name: "global verify-config parse",
			args: []string{"--verify-config"},
			verify: func(t *testing.T, path string, cli *cliApp) {
				if path != cli.rootCmd.CommandPath() {
					t.Fatalf("parsed command = %q, want %q", path, cli.rootCmd.CommandPath())
				}
				if !cli.verifyConfig {
					t.Fatal("expected verify-config flag to be set")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli, path := parseCLI(t, tt.args)
			tt.verify(t, path, cli)
		})
	}
}
