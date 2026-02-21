//go:build darwin

package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	launchd "github.com/timansky/darwin-exporter/pkg/launchd"
)

func writeLaunchctlStub(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "launchctl-stub.sh")
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return path
}

func withLaunchctlStub(t *testing.T, script string) {
	t.Helper()
	if os.Getenv("XDG_STATE_HOME") == "" {
		t.Setenv("XDG_STATE_HOME", t.TempDir())
	}
	stub := writeLaunchctlStub(t, script)
	launchd.SetLaunchctlCommand(func(_ string, args ...string) *exec.Cmd {
		cmdArgs := append([]string{stub}, args...)
		return exec.Command("sh", cmdArgs...)
	})
	t.Cleanup(func() {
		launchd.SetLaunchctlCommand(nil)
	})
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = orig
	})

	fn()

	_ = w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return string(out)
}

func TestServiceRestart_ReturnsErrorOnBootoutFailure(t *testing.T) {
	withLaunchctlStub(t, `#!/bin/sh
if [ "$1" = "bootout" ]; then
  echo "permission denied" >&2
  exit 1
fi
exit 0
`)

	err := ServiceRestart(ServiceControlOptions{SudoFlag: true})
	if err == nil {
		t.Fatal("expected restart error")
	}
	if !strings.Contains(err.Error(), "restarting service") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceRestart_IgnoresMissingBootout(t *testing.T) {
	withLaunchctlStub(t, `#!/bin/sh
if [ "$1" = "bootout" ]; then
  echo "No such process" >&2
  exit 1
fi
if [ "$1" = "kickstart" ]; then
  exit 0
fi
exit 0
`)

	if err := ServiceRestart(ServiceControlOptions{SudoFlag: true}); err != nil {
		t.Fatalf("restart returned error: %v", err)
	}
}

func TestServiceStart_FallbackPerformsBootoutBeforeBootstrap(t *testing.T) {
	dir := t.TempDir()
	trace := filepath.Join(dir, "launchctl.trace")
	plistPath := filepath.Join(dir, "kz.neko.darwin-exporter.plist")
	if err := os.WriteFile(plistPath, []byte("<plist/>"), 0644); err != nil {
		t.Fatalf("write plist: %v", err)
	}
	withLaunchctlStub(t, fmt.Sprintf(`#!/bin/sh
echo "$@" >> %q
if [ "$1" = "kickstart" ]; then
  echo "not loaded" >&2
  exit 1
fi
if [ "$1" = "bootout" ]; then
  exit 0
fi
if [ "$1" = "bootstrap" ]; then
  exit 0
fi
exit 0
`, trace))

	if err := launchd.StartWithContext(launchd.LaunchdContext{
		Domain:    "gui/501",
		Target:    "gui/501/" + launchd.PlistLabel,
		PlistPath: plistPath,
	}); err != nil {
		t.Fatalf("start returned error: %v", err)
	}

	data, err := os.ReadFile(trace)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	log := string(data)
	kickstartIdx := strings.Index(log, "kickstart -k ")
	bootoutIdx := strings.Index(log, "bootout ")
	bootstrapIdx := strings.Index(log, "bootstrap ")
	if kickstartIdx == -1 || bootoutIdx == -1 || bootstrapIdx == -1 {
		t.Fatalf("unexpected launchctl sequence:\n%s", log)
	}
	if !(kickstartIdx < bootoutIdx && bootoutIdx < bootstrapIdx) {
		t.Fatalf("wrong launchctl order:\n%s", log)
	}
}

func TestServiceStart_MissingPlistGetsHelpfulError(t *testing.T) {
	withLaunchctlStub(t, `#!/bin/sh
if [ "$1" = "kickstart" ]; then
  echo "not loaded" >&2
  exit 1
fi
exit 0
`)

	missing := filepath.Join(t.TempDir(), "missing.plist")
	err := launchd.StartWithContext(launchd.LaunchdContext{
		Domain:    "gui/501",
		Target:    "gui/501/" + launchd.PlistLabel,
		PlistPath: missing,
	})
	if err == nil {
		t.Fatal("expected start error for missing plist")
	}
	if !strings.Contains(err.Error(), "service plist not found") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "service install --type=sudo") {
		t.Fatalf("missing install hint in error: %v", err)
	}
}

func TestServiceStart_DoesNotRestartWhenAlreadyRunning(t *testing.T) {
	dir := t.TempDir()
	trace := filepath.Join(dir, "launchctl.trace")
	withLaunchctlStub(t, fmt.Sprintf(`#!/bin/sh
echo "$@" >> %q
if [ "$1" = "print" ]; then
  cat <<'EOF'
{
    state = running
    pid = 12345
}
EOF
  exit 0
fi
if [ "$1" = "kickstart" ] || [ "$1" = "bootstrap" ] || [ "$1" = "bootout" ]; then
  echo "unexpected $1" >&2
  exit 1
fi
exit 0
`, trace))

	var startErr error
	out := captureStdout(t, func() {
		startErr = ServiceStart(ServiceControlOptions{SudoFlag: true})
	})
	if startErr != nil {
		t.Fatalf("start returned error: %v", startErr)
	}
	if !strings.Contains(out, "already running") {
		t.Fatalf("expected already running message, got:\n%s", out)
	}

	data, err := os.ReadFile(trace)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	log := string(data)
	if !strings.Contains(log, "print ") {
		t.Fatalf("expected print call in trace, got:\n%s", log)
	}
	if strings.Contains(log, "kickstart ") || strings.Contains(log, "bootstrap ") || strings.Contains(log, "bootout ") {
		t.Fatalf("start should not restart running service, trace:\n%s", log)
	}
}

func TestServiceStatus_PrintsRunningState(t *testing.T) {
	withLaunchctlStub(t, `#!/bin/sh
if [ "$1" = "print" ]; then
  cat <<'EOF'
{
    state = spawn scheduled
    stdout path = /tmp/darwin-exporter.log
    stderr path = /tmp/darwin-exporter.err
    last exit code = 255
    pid = 4242
}
EOF
  exit 0
fi
exit 0
`)

	var statusErr error
	out := captureStdout(t, func() {
		statusErr = ServiceStatus(ServiceControlOptions{SudoFlag: true})
	})
	if statusErr != nil {
		t.Fatalf("status returned error: %v", statusErr)
	}
	if !strings.Contains(out, launchd.PlistLabel+".plist") {
		t.Fatalf("stdout missing plist label, got:\n%s", out)
	}
	if !strings.Contains(out, "state: Running") {
		t.Fatalf("stdout missing running state, got:\n%s", out)
	}
	if !strings.Contains(out, "pid: 4242") {
		t.Fatalf("stdout missing pid, got:\n%s", out)
	}
	if !strings.Contains(out, "user: ") {
		t.Fatalf("stdout missing user, got:\n%s", out)
	}
	if !strings.Contains(out, "stdout: /tmp/darwin-exporter.log") {
		t.Fatalf("stdout missing stdout path, got:\n%s", out)
	}
	if !strings.Contains(out, "stderr: /tmp/darwin-exporter.err") {
		t.Fatalf("stdout missing stderr path, got:\n%s", out)
	}
	if !strings.Contains(out, "exit code: 255") {
		t.Fatalf("stdout missing exit code, got:\n%s", out)
	}
	if !strings.Contains(out, "since:") {
		t.Fatalf("stdout missing since, got:\n%s", out)
	}
	if !strings.Contains(out, "process:") {
		t.Fatalf("stdout missing process, got:\n%s", out)
	}
}

func TestServiceStatus_MissingServiceIsNotLoaded(t *testing.T) {
	withLaunchctlStub(t, `#!/bin/sh
if [ "$1" = "print" ]; then
  echo "Could not find service" >&2
  exit 1
fi
exit 0
`)

	var statusErr error
	out := captureStdout(t, func() {
		statusErr = ServiceStatus(ServiceControlOptions{SudoFlag: true})
	})
	if statusErr != nil {
		t.Fatalf("status returned error: %v", statusErr)
	}
	if !strings.Contains(out, launchd.PlistLabel+".plist") {
		t.Fatalf("stdout missing plist label, got:\n%s", out)
	}
	if !strings.Contains(out, "state: Not loaded") {
		t.Fatalf("stdout missing not loaded summary, got:\n%s", out)
	}
	if !strings.Contains(out, "pid: -") {
		t.Fatalf("stdout missing pid placeholder, got:\n%s", out)
	}
	if !strings.Contains(out, "exit code: -") {
		t.Fatalf("stdout missing exit code placeholder, got:\n%s", out)
	}
}

func TestServiceStatus_MissingServiceUsesCachedLastExitCode(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)

	ctx, err := launchd.ResolveLaunchdContext(ServiceControlOptions{SudoFlag: true})
	if err != nil {
		t.Fatalf("resolve context: %v", err)
	}
	launchd.WriteStatusCache(ctx.CachePath, launchd.ServiceStatusCache{
		LastExitCode: "(never exited)",
		StdoutPath:   "/tmp/de.log",
		StderrPath:   "/tmp/de.err",
	})

	withLaunchctlStub(t, `#!/bin/sh
if [ "$1" = "print" ]; then
  echo "Could not find service" >&2
  exit 1
fi
exit 0
`)

	var statusErr error
	out := captureStdout(t, func() {
		statusErr = ServiceStatus(ServiceControlOptions{SudoFlag: true})
	})
	if statusErr != nil {
		t.Fatalf("status returned error: %v", statusErr)
	}
	if !strings.Contains(out, "exit code: (never exited)") {
		t.Fatalf("stdout missing cached last exit code, got:\n%s", out)
	}
	if !strings.Contains(out, "stdout: /tmp/de.log") {
		t.Fatalf("stdout missing cached stdout path, got:\n%s", out)
	}
	if !strings.Contains(out, "stderr: /tmp/de.err") {
		t.Fatalf("stdout missing cached stderr path, got:\n%s", out)
	}
}

func TestServiceLogs_PrintsTailFromStdoutAndStderr(t *testing.T) {
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "darwin-exporter.log")
	stderrPath := filepath.Join(dir, "darwin-exporter.err")

	if err := os.WriteFile(stdoutPath, []byte("o1\no2\no3\no4\n"), 0644); err != nil {
		t.Fatalf("write stdout log: %v", err)
	}
	if err := os.WriteFile(stderrPath, []byte("e1\ne2\ne3\n"), 0644); err != nil {
		t.Fatalf("write stderr log: %v", err)
	}

	withLaunchctlStub(t, fmt.Sprintf(`#!/bin/sh
if [ "$1" = "print" ]; then
  cat <<'EOF'
{
    state = running
    stdout path = %s
    stderr path = %s
}
EOF
  exit 0
fi
exit 0
`, stdoutPath, stderrPath))

	var logsErr error
	out := captureStdout(t, func() {
		logsErr = ServiceLogs(ServiceLogsOptions{
			ServiceControlOptions: ServiceControlOptions{SudoFlag: true},
			Lines:                 2,
		})
	})
	if logsErr != nil {
		t.Fatalf("logs returned error: %v", logsErr)
	}
	if !strings.Contains(out, "==> "+stdoutPath+" <==") {
		t.Fatalf("stdout section missing, got:\n%s", out)
	}
	if !strings.Contains(out, "[stdout]") {
		t.Fatalf("stdout section label missing, got:\n%s", out)
	}
	if !strings.Contains(out, "o3\no4") {
		t.Fatalf("stdout tail missing expected lines, got:\n%s", out)
	}
	if !strings.Contains(out, "==> "+stderrPath+" <==") {
		t.Fatalf("stderr section missing, got:\n%s", out)
	}
	if !strings.Contains(out, "[stderr]") {
		t.Fatalf("stderr section label missing, got:\n%s", out)
	}
	if !strings.Contains(out, "e2\ne3") {
		t.Fatalf("stderr tail missing expected lines, got:\n%s", out)
	}
}

func TestServiceLogs_ColorAlwaysAddsANSIToHeader(t *testing.T) {
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "darwin-exporter.log")
	if err := os.WriteFile(stdoutPath, []byte("o1\no2\n"), 0644); err != nil {
		t.Fatalf("write stdout log: %v", err)
	}

	withLaunchctlStub(t, fmt.Sprintf(`#!/bin/sh
if [ "$1" = "print" ]; then
  cat <<'EOF'
{
    state = running
    stdout path = %s
}
EOF
  exit 0
fi
exit 0
`, stdoutPath))

	SetColorMode("always")
	t.Cleanup(func() { SetColorMode("never") })

	var logsErr error
	out := captureStdout(t, func() {
		logsErr = ServiceLogs(ServiceLogsOptions{
			ServiceControlOptions: ServiceControlOptions{SudoFlag: true},
			Lines:                 1,
		})
	})
	if logsErr != nil {
		t.Fatalf("logs returned error: %v", logsErr)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected ANSI output with color=always, got:\n%s", out)
	}
}

func TestServiceLogs_ColorAlwaysColorsLogSeverityLines(t *testing.T) {
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "darwin-exporter.log")
	content := strings.Join([]string{
		`time=2026-02-21T12:00:00Z level=info msg="ready"`,
		`time=2026-02-21T12:00:01Z level=warn msg="slow scrape"`,
		`time=2026-02-21T12:00:02Z level=error msg="collector failed"`,
		`time=2026-02-21T12:00:03Z level=debug msg="trace point"`,
	}, "\n") + "\n"
	if err := os.WriteFile(stdoutPath, []byte(content), 0644); err != nil {
		t.Fatalf("write stdout log: %v", err)
	}

	withLaunchctlStub(t, fmt.Sprintf(`#!/bin/sh
if [ "$1" = "print" ]; then
  cat <<'EOF'
{
    state = running
    stdout path = %s
}
EOF
  exit 0
fi
exit 0
`, stdoutPath))

	SetColorMode("always")
	t.Cleanup(func() { SetColorMode("never") })

	var logsErr error
	out := captureStdout(t, func() {
		logsErr = ServiceLogs(ServiceLogsOptions{
			ServiceControlOptions: ServiceControlOptions{SudoFlag: true},
			Lines:                 20,
		})
	})
	if logsErr != nil {
		t.Fatalf("logs returned error: %v", logsErr)
	}

	if !strings.Contains(out, styleKey("time")+"=2026-02-21T12:00:00Z") {
		t.Fatalf("expected time key highlighting with default value color, got:\n%s", out)
	}
	if !strings.Contains(out, styleKey("level")+"="+styleInfo("info")) {
		t.Fatalf("expected info level highlighting, got:\n%s", out)
	}
	if !strings.Contains(out, styleKey("level")+"="+styleWarn("warn")) {
		t.Fatalf("expected warn level highlighting, got:\n%s", out)
	}
	if !strings.Contains(out, styleKey("level")+"="+styleError("error")) {
		t.Fatalf("expected error level highlighting, got:\n%s", out)
	}
	if !strings.Contains(out, styleKey("level")+"="+styleKey("debug")) {
		t.Fatalf("expected debug level highlighting, got:\n%s", out)
	}
	if strings.Contains(out, ansiCyan+`time=2026-02-21T12:00:00Z level=info msg="ready"`+ansiReset) {
		t.Fatalf("line should not be monochrome-highlighted, got:\n%s", out)
	}
}

func TestServiceLogs_ColorAlwaysHighlightsJSONLogsByTokenType(t *testing.T) {
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "darwin-exporter.log")
	jsonLine := `{"time":"2026-02-21T12:00:00Z","level":"error","msg":"boom","component":"wifi"}`
	if err := os.WriteFile(stdoutPath, []byte(jsonLine+"\n"), 0644); err != nil {
		t.Fatalf("write stdout log: %v", err)
	}

	withLaunchctlStub(t, fmt.Sprintf(`#!/bin/sh
if [ "$1" = "print" ]; then
  cat <<'EOF'
{
    state = running
    stdout path = %s
}
EOF
  exit 0
fi
exit 0
`, stdoutPath))

	SetColorMode("always")
	t.Cleanup(func() { SetColorMode("never") })

	var logsErr error
	out := captureStdout(t, func() {
		logsErr = ServiceLogs(ServiceLogsOptions{
			ServiceControlOptions: ServiceControlOptions{SudoFlag: true},
			Lines:                 10,
		})
	})
	if logsErr != nil {
		t.Fatalf("logs returned error: %v", logsErr)
	}
	if !strings.Contains(out, styleKey(`"time"`)+":"+`"2026-02-21T12:00:00Z"`) {
		t.Fatalf("expected json time key highlighting with default value color, got:\n%s", out)
	}
	if !strings.Contains(out, styleKey(`"level"`)+":"+styleError(`"error"`)) {
		t.Fatalf("expected json level highlighting, got:\n%s", out)
	}
	if !strings.Contains(out, styleKey(`"msg"`)+":"+`"boom"`) {
		t.Fatalf("expected json msg key highlighting with default value color, got:\n%s", out)
	}
}

func TestServiceLogs_ColorAlwaysColorsPlainStderrLineRed(t *testing.T) {
	dir := t.TempDir()
	stderrPath := filepath.Join(dir, "darwin-exporter.err")
	line := "plain stderr line without explicit level"
	if err := os.WriteFile(stderrPath, []byte(line+"\n"), 0644); err != nil {
		t.Fatalf("write stderr log: %v", err)
	}

	withLaunchctlStub(t, fmt.Sprintf(`#!/bin/sh
if [ "$1" = "print" ]; then
  cat <<'EOF'
{
    state = running
    stderr path = %s
}
EOF
  exit 0
fi
exit 0
`, stderrPath))

	SetColorMode("always")
	t.Cleanup(func() { SetColorMode("never") })

	var logsErr error
	out := captureStdout(t, func() {
		logsErr = ServiceLogs(ServiceLogsOptions{
			ServiceControlOptions: ServiceControlOptions{SudoFlag: true},
			Lines:                 10,
		})
	})
	if logsErr != nil {
		t.Fatalf("logs returned error: %v", logsErr)
	}
	if !strings.Contains(out, ansiRed+line+ansiReset) {
		t.Fatalf("expected plain stderr line to be red, got:\n%s", out)
	}
}
