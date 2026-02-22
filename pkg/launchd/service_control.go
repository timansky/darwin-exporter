//go:build darwin

package launchd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/go-logfmt/logfmt"
	"github.com/nxadm/tail"
	plist "howett.net/plist"
)

var launchctlCommand = exec.Command

// SetLaunchctlCommand overrides the command constructor used for launchctl calls.
// Passing nil resets it to exec.Command.
func SetLaunchctlCommand(fn func(name string, arg ...string) *exec.Cmd) {
	if fn == nil {
		launchctlCommand = exec.Command
		return
	}
	launchctlCommand = fn
}

// ServiceControlOptions holds mode flags for service lifecycle commands.
type ServiceControlOptions struct {
	SudoFlag bool
	RootFlag bool
}

// ServiceLogsOptions controls `service logs`.
type ServiceLogsOptions struct {
	ServiceControlOptions
	Lines int
}

type LaunchdContext struct {
	Domain    string
	Target    string
	PlistPath string
	CachePath string
}

type serviceRuntime struct {
	Loaded bool
	State  string
	PID    string
}

type logSource int

const (
	logSourceUnknown logSource = iota
	logSourceStdout
	logSourceStderr
	logSourceCombined
)

var activeHooks Hooks

func withHooks(hooks Hooks, fn func() error) error {
	prev := activeHooks
	activeHooks = hooks
	defer func() {
		activeHooks = prev
	}()
	return fn()
}

func styleInfo(s string) string    { return activeHooks.styleInfo(s) }
func styleWarn(s string) string    { return activeHooks.styleWarn(s) }
func styleError(s string) string   { return activeHooks.styleError(s) }
func styleKey(s string) string     { return activeHooks.styleKey(s) }
func styleSuccess(s string) string { return activeHooks.styleSuccess(s) }

// ServiceStart starts an installed launchd service.
func ServiceStart(opts ServiceControlOptions, hooks Hooks) error {
	return withHooks(hooks, func() error {
		ctx, err := ResolveLaunchdContext(opts)
		if err != nil {
			return err
		}

		rt, err := queryServiceRuntime(ctx)
		if err != nil {
			return fmt.Errorf("checking service status before start: %w", err)
		}
		if rt.Loaded && isRunningState(rt.State, rt.PID) {
			if rt.PID == "" {
				hooks.warnf("Service %s already running", PlistLabel)
			} else {
				hooks.warnf("Service %s already running (pid=%s)", PlistLabel, rt.PID)
			}
			return nil
		}

		if err := StartWithContext(ctx); err != nil {
			return err
		}
		cache := readStatusCache(ctx.CachePath)
		cache.StartedAt = time.Now().Format(time.RFC3339)
		cache.StoppedAt = ""
		WriteStatusCache(ctx.CachePath, cache)
		hooks.successf("Service %s started", PlistLabel)
		return nil
	})
}

func StartWithContext(ctx LaunchdContext) error {

	// Try kickstart first for already-bootstrapped services.
	if err := runLaunchctl("kickstart", "-k", ctx.Target); err == nil {
		return nil
	}

	if _, err := os.Stat(ctx.PlistPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf(
				"service plist not found: %s (run: darwin-exporter service install --type=%s)",
				ctx.PlistPath,
				installTypeForContext(ctx),
			)
		}
		return fmt.Errorf("checking service plist %s: %w", ctx.PlistPath, err)
	}

	// Ensure stale/broken jobs are unloaded before bootstrap.
	if err := bootoutIgnoreMissing(ctx.Target); err != nil {
		return fmt.Errorf("starting service: %w", err)
	}

	// Fall back to bootstrap when the service is not loaded yet.
	if err := runLaunchctl("bootstrap", ctx.Domain, ctx.PlistPath); err != nil {
		return fmt.Errorf("starting service: %w", err)
	}
	return nil
}

// ServiceStop stops a launchd service if it is currently running/loaded.
func ServiceStop(opts ServiceControlOptions, hooks Hooks) error {
	return withHooks(hooks, func() error {
		ctx, err := ResolveLaunchdContext(opts)
		if err != nil {
			return err
		}
		cacheLiveServiceStatus(ctx)
		if err := bootoutIgnoreMissing(ctx.Target); err != nil {
			return fmt.Errorf("stopping service: %w", err)
		}
		cache := readStatusCache(ctx.CachePath)
		cache.StoppedAt = time.Now().Format(time.RFC3339)
		WriteStatusCache(ctx.CachePath, cache)
		hooks.successf("Service %s stopped", PlistLabel)
		return nil
	})
}

// ServiceRestart restarts a launchd service.
func ServiceRestart(opts ServiceControlOptions, hooks Hooks) error {
	return withHooks(hooks, func() error {
		ctx, err := ResolveLaunchdContext(opts)
		if err != nil {
			return err
		}

		if err := bootoutIgnoreMissing(ctx.Target); err != nil {
			return fmt.Errorf("restarting service: %w", err)
		}
		if err := StartWithContext(ctx); err != nil {
			return err
		}
		cache := readStatusCache(ctx.CachePath)
		cache.StartedAt = time.Now().Format(time.RFC3339)
		cache.StoppedAt = ""
		WriteStatusCache(ctx.CachePath, cache)
		hooks.successf("Service %s restarted", PlistLabel)
		return nil
	})
}

// ServiceEnable enables launchd autostart for a service.
func ServiceEnable(opts ServiceControlOptions, hooks Hooks) error {
	return withHooks(hooks, func() error {
		ctx, err := ResolveLaunchdContext(opts)
		if err != nil {
			return err
		}
		if err := runLaunchctl("enable", ctx.Target); err != nil {
			return fmt.Errorf("enabling service: %w", err)
		}
		hooks.successf("Service %s enabled", PlistLabel)
		return nil
	})
}

// ServiceDisable disables launchd autostart for a service.
func ServiceDisable(opts ServiceControlOptions, hooks Hooks) error {
	return withHooks(hooks, func() error {
		ctx, err := ResolveLaunchdContext(opts)
		if err != nil {
			return err
		}
		if err := runLaunchctl("disable", ctx.Target); err != nil {
			return fmt.Errorf("disabling service: %w", err)
		}
		hooks.successf("Service %s disabled", PlistLabel)
		return nil
	})
}

// ServiceStatus prints launchd service status.
func ServiceStatus(opts ServiceControlOptions, hooks Hooks) error {
	return withHooks(hooks, func() error {
		ctx, err := ResolveLaunchdContext(opts)
		if err != nil {
			return err
		}

		out, err := runLaunchctlOutput("print", ctx.Target)
		if err != nil {
			cached := readStatusCache(ctx.CachePath)
			lastExitCode := cached.LastExitCode
			if lastExitCode == "" {
				lastExitCode = "-"
			}
			stdoutPath := cached.StdoutPath
			if stdoutPath == "" {
				stdoutPath = "-"
			}
			stderrPath := cached.StderrPath
			if stderrPath == "" {
				stderrPath = "-"
			}
			if stdoutPath == "-" || stderrPath == "-" {
				if plistStdout, plistStderr, perr := plistLogPaths(ctx.PlistPath); perr == nil {
					if stdoutPath == "-" && plistStdout != "" {
						stdoutPath = plistStdout
					}
					if stderrPath == "-" && plistStderr != "" {
						stderrPath = plistStderr
					}
				}
			}
			if isLaunchctlMissingError(err.Error()) {
				printServiceStatus(serviceStatusView{
					Label:    PlistLabel + ".plist",
					Summary:  "Not loaded",
					State:    "Not loaded",
					PID:      "-",
					User:     userFromTarget(ctx.Target),
					Process:  processFromPlist(ctx.PlistPath),
					Since:    stoppedSince(cached.StoppedAt),
					Stdout:   stdoutPath,
					Stderr:   stderrPath,
					ExitCode: lastExitCode,
				})
				return nil
			}
			return fmt.Errorf("checking service status: %w", err)
		}

		state := launchctlField(out, "state")
		if state == "" {
			state = "unknown"
		}

		stdoutPath := launchctlField(out, "stdout path")
		if stdoutPath == "" {
			stdoutPath = "-"
		}
		stderrPath := launchctlField(out, "stderr path")
		if stderrPath == "" {
			stderrPath = "-"
		}
		lastExitCode := launchctlField(out, "last exit code")
		if lastExitCode == "" {
			lastExitCode = "-"
		}
		pid := launchctlField(out, "pid")
		if pid == "" {
			pid = "-"
		}
		cached := readStatusCache(ctx.CachePath)
		WriteStatusCache(ctx.CachePath, ServiceStatusCache{
			LastExitCode: lastExitCode,
			StdoutPath:   stdoutPath,
			StderrPath:   stderrPath,
			StartedAt:    chooseStartTimestamp(cached.StartedAt),
			StoppedAt:    "",
		})
		summary := serviceSummary(state, pid)
		since := sinceForPID(pid)
		if since == "-" && strings.EqualFold(summary, "running") {
			since = startedSince(cached.StartedAt)
		}
		printServiceStatus(serviceStatusView{
			Label:    PlistLabel + ".plist",
			Summary:  summary,
			State:    summary,
			PID:      pid,
			User:     userFromTarget(ctx.Target),
			Process:  processFromLaunchctl(out, ctx.PlistPath),
			Since:    since,
			Stdout:   stdoutPath,
			Stderr:   stderrPath,
			ExitCode: lastExitCode,
		})
		return nil
	})
}

// ServiceLogs prints the last N lines from service log files.
func ServiceLogs(opts ServiceLogsOptions, hooks Hooks) error {
	return withHooks(hooks, func() error {
		ctx, err := ResolveLaunchdContext(opts.ServiceControlOptions)
		if err != nil {
			return err
		}

		lines := opts.Lines
		if lines <= 0 {
			lines = 100
		}

		paths, err := resolveServiceLogPaths(ctx)
		if err != nil {
			return fmt.Errorf("resolving service log paths: %w", err)
		}

		printed := make(map[string]bool)
		order := []string{paths.stdoutPath, paths.stderrPath}
		for _, p := range order {
			if p == "" || printed[p] {
				continue
			}
			printed[p] = true

			fmt.Printf("%s %s\n", styleInfo(fmt.Sprintf("==> %s <==", p)), styleLogSourceLabel(p, paths))
			content, readErr := tailFileLines(p, lines)
			if readErr != nil {
				if os.IsNotExist(readErr) {
					fmt.Println(styleWarn("(file not found)"))
					fmt.Println()
					continue
				}
				return fmt.Errorf("reading %s: %w", p, readErr)
			}
			if strings.TrimSpace(content) == "" {
				fmt.Println(styleWarn("(empty)"))
				fmt.Println()
				continue
			}
			source := detectLogSource(p, paths)
			content = styleLogContent(content, source)
			fmt.Print(content)
			if !strings.HasSuffix(content, "\n") {
				fmt.Println()
			}
			fmt.Println()
		}

		return nil
	})
}

func ResolveLaunchdContext(opts ServiceControlOptions) (LaunchdContext, error) {
	mode, err := DetectInstallMode(opts.SudoFlag, opts.RootFlag)
	if err != nil {
		return LaunchdContext{}, err
	}

	switch mode {
	case ModeSudo:
		invoker, err := ResolveInvokerUser()
		if err != nil {
			return LaunchdContext{}, fmt.Errorf("resolving invoker user: %w", err)
		}
		if invoker.HomeDir == "" {
			return LaunchdContext{}, fmt.Errorf("could not determine home directory for %s", invoker.Username)
		}
		domain := "gui/" + invoker.Uid
		stateHome := os.Getenv("XDG_STATE_HOME")
		if stateHome == "" {
			stateHome = filepath.Join(invoker.HomeDir, ".local", "state")
		}
		return LaunchdContext{
			Domain:    domain,
			Target:    domain + "/" + PlistLabel,
			PlistPath: filepath.Join(invoker.HomeDir, "Library", "LaunchAgents", PlistLabel+".plist"),
			CachePath: filepath.Join(stateHome, "darwin-exporter", "launchd-status.json"),
		}, nil
	case ModeRoot:
		if os.Geteuid() != 0 {
			return LaunchdContext{}, fmt.Errorf("--root mode requires root privileges (run with sudo)")
		}
		return LaunchdContext{
			Domain:    "system",
			Target:    "system/" + PlistLabel,
			PlistPath: "/Library/LaunchDaemons/" + PlistLabel + ".plist",
			CachePath: "/var/tmp/darwin-exporter/launchd-status.json",
		}, nil
	default:
		return LaunchdContext{}, fmt.Errorf("unknown install mode")
	}
}

func runLaunchctl(args ...string) error {
	_, err := runLaunchctlOutput(args...)
	return err
}

func runLaunchctlOutput(args ...string) (string, error) {
	out, err := launchctlCommand("launchctl", args...).CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if err != nil {
		return msg, fmt.Errorf("launchctl %s: %s: %w", strings.Join(args, " "), msg, err)
	}
	return msg, nil
}

func bootoutIgnoreMissing(target string) error {
	out, err := launchctlCommand("launchctl", "bootout", target).CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if isLaunchctlMissingError(msg) {
		return nil
	}
	return fmt.Errorf("launchctl bootout %s: %s: %w", target, msg, err)
}

func isLaunchctlMissingError(msg string) bool {
	s := strings.ToLower(strings.TrimSpace(msg))
	return strings.Contains(s, "no such process") || strings.Contains(s, "could not find service") || strings.Contains(s, "not found")
}

func launchctlField(out, field string) string {
	prefix := field + " = "
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func queryServiceRuntime(ctx LaunchdContext) (serviceRuntime, error) {
	out, err := runLaunchctlOutput("print", ctx.Target)
	if err != nil {
		if isLaunchctlMissingError(err.Error()) {
			return serviceRuntime{}, nil
		}
		return serviceRuntime{}, err
	}
	return serviceRuntime{
		Loaded: true,
		State:  launchctlField(out, "state"),
		PID:    launchctlField(out, "pid"),
	}, nil
}

func isRunningState(state, pid string) bool {
	if strings.TrimSpace(pid) != "" {
		return true
	}
	s := strings.ToLower(strings.TrimSpace(state))
	return strings.Contains(s, "running") || strings.Contains(s, "active") || strings.Contains(s, "spawn")
}

func serviceSummary(state, pid string) string {
	if pid != "-" && isRunningState(state, pid) {
		return "Running"
	}
	return "Loaded"
}

type serviceStatusView struct {
	Label    string
	Summary  string
	State    string
	PID      string
	User     string
	Process  string
	Since    string
	Stdout   string
	Stderr   string
	ExitCode string
}

func printServiceStatus(v serviceStatusView) {
	if v.Label == "" {
		v.Label = PlistLabel + ".plist"
	}
	if v.State == "" {
		v.State = "-"
	}
	if v.PID == "" {
		v.PID = "-"
	}
	if v.User == "" {
		v.User = "-"
	}
	if v.Process == "" {
		v.Process = "-"
	}
	if v.Since == "" {
		v.Since = "-"
	}
	if v.Stdout == "" {
		v.Stdout = "-"
	}
	if v.Stderr == "" {
		v.Stderr = "-"
	}
	if v.ExitCode == "" {
		v.ExitCode = "-"
	}

	fmt.Printf("%s %s\n", statusBullet(v.Summary), v.Label)
	printServiceStatusLine("state", styleSummary(v.State))
	printServiceStatusLine("pid", v.PID)
	printServiceStatusLine("user", v.User)
	printServiceStatusLine("process", v.Process)
	printServiceStatusLine("since", v.Since)
	printServiceStatusLine("stdout", v.Stdout)
	printServiceStatusLine("stderr", v.Stderr)
	printServiceStatusLine("exit code", v.ExitCode)
}

func printServiceStatusLine(key, value string) {
	const width = 10
	keyLen := len(key)
	if keyLen > width {
		keyLen = width
	}
	padding := strings.Repeat(" ", width-keyLen)
	fmt.Printf(" %s%s: %s\n", padding, styleKey(key), value)
}

func statusBullet(summary string) string {
	switch strings.ToLower(strings.TrimSpace(summary)) {
	case "running":
		return styleSuccess("●")
	case "not loaded":
		return styleWarn("○")
	default:
		return styleInfo("◐")
	}
}

func userFromTarget(target string) string {
	parts := strings.Split(target, "/")
	if len(parts) >= 2 && parts[0] == "gui" {
		uid := parts[1]
		u, err := user.LookupId(uid)
		if err == nil && strings.TrimSpace(u.Username) != "" {
			return fmt.Sprintf("%s (%s)", u.Username, uid)
		}
		return uid
	}
	if len(parts) >= 1 && parts[0] == "system" {
		return "root (0)"
	}
	return "-"
}

func processFromLaunchctl(out, plistPath string) string {
	if v := launchctlField(out, "path"); v != "" {
		return v
	}
	if v := launchctlField(out, "program"); v != "" {
		return v
	}
	return processFromPlist(plistPath)
}

func processFromPlist(plistPath string) string {
	args, err := plistProgramArguments(plistPath)
	if err != nil || len(args) == 0 {
		return "-"
	}
	return strings.Join(args, " ")
}

func sinceForPID(pid string) string {
	if strings.TrimSpace(pid) == "" || pid == "-" {
		return "-"
	}
	if _, err := strconv.Atoi(pid); err != nil {
		return "-"
	}
	out, err := exec.Command("ps", "-p", pid, "-o", "lstart=").CombinedOutput()
	if err != nil {
		return "-"
	}
	startRaw := strings.TrimSpace(string(out))
	if startRaw == "" {
		return "-"
	}
	start, err := time.Parse("Mon Jan _2 15:04:05 2006", startRaw)
	if err != nil {
		return "-"
	}
	now := time.Now()
	if start.After(now) {
		return "-"
	}
	return fmt.Sprintf("%s; %s", start.Format("Mon 2006-01-02 15:04:05 -07"), humanize.RelTime(start, now, "ago", "from now"))
}

func stoppedSince(ts string) string {
	if strings.TrimSpace(ts) == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	now := time.Now()
	if t.After(now) {
		return "stopped " + t.Format("Mon 2006-01-02 15:04:05 -07")
	}
	return fmt.Sprintf("stopped %s; %s", t.Format("Mon 2006-01-02 15:04:05 -07"), humanize.RelTime(t, now, "ago", "from now"))
}

func startedSince(ts string) string {
	if strings.TrimSpace(ts) == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	now := time.Now()
	if t.After(now) {
		return t.Format("Mon 2006-01-02 15:04:05 -07")
	}
	return fmt.Sprintf("%s; %s", t.Format("Mon 2006-01-02 15:04:05 -07"), humanize.RelTime(t, now, "ago", "from now"))
}

func chooseStartTimestamp(existing string) string {
	if strings.TrimSpace(existing) != "" {
		return existing
	}
	return time.Now().Format(time.RFC3339)
}

type serviceLogPaths struct {
	stdoutPath string
	stderrPath string
}

type ServiceStatusCache struct {
	LastExitCode string `json:"last_exit_code"`
	StdoutPath   string `json:"stdout_path"`
	StderrPath   string `json:"stderr_path"`
	StartedAt    string `json:"started_at"`
	StoppedAt    string `json:"stopped_at"`
}

func resolveServiceLogPaths(ctx LaunchdContext) (serviceLogPaths, error) {
	var paths serviceLogPaths

	out, err := runLaunchctlOutput("print", ctx.Target)
	if err == nil {
		paths.stdoutPath = launchctlField(out, "stdout path")
		paths.stderrPath = launchctlField(out, "stderr path")
	}

	if paths.stdoutPath == "" || paths.stderrPath == "" {
		plistStdout, plistStderr, plistErr := plistLogPaths(ctx.PlistPath)
		if plistErr != nil && paths.stdoutPath == "" && paths.stderrPath == "" {
			return serviceLogPaths{}, fmt.Errorf("service not loaded and plist %q not readable: %w", ctx.PlistPath, plistErr)
		}
		if paths.stdoutPath == "" {
			paths.stdoutPath = plistStdout
		}
		if paths.stderrPath == "" {
			paths.stderrPath = plistStderr
		}
	}

	if paths.stdoutPath == "" && paths.stderrPath == "" {
		return serviceLogPaths{}, fmt.Errorf("could not determine log file paths")
	}
	return paths, nil
}

func plistLogPaths(plistPath string) (string, string, error) {
	doc, err := readPlistMap(plistPath)
	if err != nil {
		return "", "", err
	}
	return plistString(doc["StandardOutPath"]), plistString(doc["StandardErrorPath"]), nil
}

func readPlistMap(plistPath string) (map[string]any, error) {
	data, err := os.ReadFile(plistPath)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if _, err := plist.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing plist %q: %w", plistPath, err)
	}
	return doc, nil
}

func plistString(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func plistProgramArguments(plistPath string) ([]string, error) {
	doc, err := readPlistMap(plistPath)
	if err != nil {
		return nil, err
	}
	raw, ok := doc["ProgramArguments"]
	if !ok {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, nil
	}
	args := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if ok {
			s = strings.TrimSpace(s)
			if s != "" {
				args = append(args, s)
			}
		}
	}
	return args, nil
}

func tailFileLines(path string, lines int) (string, error) {
	t, err := tail.TailFile(path, tail.Config{
		Follow:    false,
		ReOpen:    false,
		MustExist: true,
		Poll:      true,
		Logger:    tail.DiscardingLogger,
		Location:  &tail.SeekInfo{Offset: 0, Whence: 0},
	})
	if err != nil {
		return "", err
	}
	defer func() {
		_ = t.Stop()
		t.Cleanup()
	}()

	if lines <= 0 {
		lines = 100
	}
	buf := make([]string, 0, lines)
	for line := range t.Lines {
		if line == nil {
			continue
		}
		if line.Err != nil {
			return "", line.Err
		}
		if len(buf) == lines {
			copy(buf, buf[1:])
			buf[len(buf)-1] = line.Text
			continue
		}
		buf = append(buf, line.Text)
	}
	if len(buf) == 0 {
		return "", nil
	}
	return strings.Join(buf, "\n"), nil
}

func cacheLiveServiceStatus(ctx LaunchdContext) {
	out, err := runLaunchctlOutput("print", ctx.Target)
	if err != nil {
		return
	}
	WriteStatusCache(ctx.CachePath, ServiceStatusCache{
		LastExitCode: fallbackValue(launchctlField(out, "last exit code"), "-"),
		StdoutPath:   launchctlField(out, "stdout path"),
		StderrPath:   launchctlField(out, "stderr path"),
	})
}

func WriteStatusCache(path string, data ServiceStatusCache) {
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	buf, err := json.Marshal(data)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, buf, 0644)
}

func readStatusCache(path string) ServiceStatusCache {
	var data ServiceStatusCache
	if path == "" {
		return data
	}
	buf, err := os.ReadFile(path)
	if err != nil {
		return data
	}
	_ = json.Unmarshal(buf, &data)
	return data
}

func fallbackValue(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func styleSummary(summary string) string {
	switch strings.ToLower(strings.TrimSpace(summary)) {
	case "running":
		return styleSuccess(summary)
	case "not loaded":
		return styleWarn(summary)
	case "loaded":
		return styleInfo(summary)
	default:
		return summary
	}
}

func styleLogSourceLabel(path string, paths serviceLogPaths) string {
	switch {
	case path == paths.stdoutPath && path == paths.stderrPath:
		return styleKey("[stdout+stderr]")
	case path == paths.stderrPath:
		return styleError("[stderr]")
	case path == paths.stdoutPath:
		return styleInfo("[stdout]")
	default:
		return styleKey("[log]")
	}
}

func detectLogSource(path string, paths serviceLogPaths) logSource {
	switch {
	case path == paths.stdoutPath && path == paths.stderrPath:
		return logSourceCombined
	case path == paths.stderrPath:
		return logSourceStderr
	case path == paths.stdoutPath:
		return logSourceStdout
	default:
		return logSourceUnknown
	}
}

func styleLogContent(content string, source logSource) string {
	if content == "" {
		return content
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = styleLogLine(line, source)
	}
	return strings.Join(lines, "\n")
}

func styleLogLine(line string, source logSource) string {
	if strings.TrimSpace(line) == "" {
		return line
	}

	if looksLikeJSONLog(line) {
		return styleJSONLogLine(line, source)
	}
	if strings.Contains(line, "=") {
		return styleLogfmtLine(line, source)
	}

	l := strings.ToLower(line)
	switch {
	case containsAny(l, " level=panic", " level=fatal", "\"level\":\"panic\"", "\"level\":\"fatal\"", "panic:"):
		return styleError(line)
	case containsAny(l, " level=error", "\"level\":\"error\"", " error ", " err="):
		return styleError(line)
	case containsAny(l, " level=warn", " level=warning", "\"level\":\"warn\"", "\"level\":\"warning\"", " warn ", " warning "):
		return styleWarn(line)
	case containsAny(l, " level=info", "\"level\":\"info\"", " info "):
		return styleInfo(line)
	case containsAny(l, " level=debug", " level=trace", "\"level\":\"debug\"", "\"level\":\"trace\"", " debug ", " trace "):
		return styleKey(line)
	}

	if source == logSourceStderr {
		return styleError(line)
	}
	return line
}

func looksLikeJSONLog(line string) bool {
	s := strings.TrimSpace(line)
	return strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") && strings.Contains(s, ":")
}

func styleLogfmtLine(line string, source logSource) string {
	dec := logfmt.NewDecoder(bytes.NewReader([]byte(line)))
	styled := make([]string, 0, 8)
	for dec.ScanRecord() {
		for dec.ScanKeyval() {
			key := string(dec.Key())
			val := string(dec.Value())
			if isLevelKey(key) {
				styled = append(styled, styleKey(key)+"="+styleLevelValue(val))
				continue
			}
			styled = append(styled, styleKey(key)+"="+styleLogValueByKey(key, val, source))
		}
	}
	if dec.Err() == nil && len(styled) > 0 {
		return strings.Join(styled, " ")
	}
	return line
}

func styleJSONLogLine(line string, source logSource) string {
	var b strings.Builder
	lastKey := ""

	for i := 0; i < len(line); {
		if line[i] != '"' {
			b.WriteByte(line[i])
			i++
			continue
		}

		j := i + 1
		escaped := false
		for j < len(line) {
			ch := line[j]
			if escaped {
				escaped = false
				j++
				continue
			}
			if ch == '\\' {
				escaped = true
				j++
				continue
			}
			if ch == '"' {
				break
			}
			j++
		}
		if j >= len(line) {
			b.WriteString(line[i:])
			break
		}

		token := line[i : j+1]
		k := j + 1
		for k < len(line) && (line[k] == ' ' || line[k] == '\t') {
			k++
		}
		if k < len(line) && line[k] == ':' {
			lastKey = strings.Trim(strings.ToLower(token), `"`)
			b.WriteString(styleKey(token))
		} else {
			b.WriteString(styleLogValueByKey(lastKey, token, source))
			lastKey = ""
		}
		i = j + 1
	}

	return b.String()
}

func styleLogValueByKey(key, val string, source logSource) string {
	k := strings.ToLower(strings.TrimSpace(key))
	switch k {
	case "level", "lvl", "severity":
		return styleLevelValue(val)
	case "time", "ts", "timestamp":
		return val
	case "msg", "message":
		return val
	case "err", "error":
		return styleError(val)
	}
	if source == logSourceStderr {
		return styleError(val)
	}
	return val
}

func isLevelKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "level", "lvl", "severity":
		return true
	default:
		return false
	}
}

func styleLevelValue(val string) string {
	level := strings.TrimSpace(strings.Trim(strings.ToLower(val), `"'`))
	switch level {
	case "panic", "fatal", "error":
		return styleError(val)
	case "warn", "warning":
		return styleWarn(val)
	case "info":
		return styleInfo(val)
	case "debug", "trace":
		return styleKey(val)
	default:
		return val
	}
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

func installTypeForContext(ctx LaunchdContext) string {
	if ctx.Domain == "system" || strings.HasPrefix(ctx.Domain, "system/") {
		return "root"
	}
	return "sudo"
}
