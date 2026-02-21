//go:build darwin

package collector

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/sirupsen/logrus"
)

// newWdutilTestLogger returns a logrus logger suitable for tests (quiet).
func newWdutilTestLogger() *logrus.Logger {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	return log
}

// collectWdutilMetrics runs the given collector's Update and returns emitted metrics.
func collectWdutilMetrics(t *testing.T, c *WdutilCollector) []prometheus.Metric {
	t.Helper()
	ch := make(chan prometheus.Metric, 32)
	if err := c.Update(ch); err != nil {
		t.Errorf("unexpected Update error: %v", err)
	}
	close(ch)
	var out []prometheus.Metric
	for m := range ch {
		out = append(out, m)
	}
	return out
}

// TestWdutilCollector_NoAccess verifies that a collector without privilege
// emits exactly one metric: available=0.
func TestWdutilCollector_NoAccess(t *testing.T) {
	// Build collector skeleton with nil wdutilCmd (simulates no access).
	base := NewWdutilCollector(newWdutilTestLogger())
	base.wdutilCmd = nil

	metrics := collectWdutilMetrics(t, base)
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric (available=0), got %d", len(metrics))
	}
}

// TestWdutilCollector_WithFullOutput verifies Update() emits all metrics when
// wdutil returns valid output.
func TestWdutilCollector_WithFullOutput(t *testing.T) {
	// Use a fake command that echoes fixture output.
	// We override wdutilCmd to use /bin/echo with the fixture text piped via sh.
	c := NewWdutilCollector(newWdutilTestLogger())

	// Directly test Update path by injecting a fake runCommand via monkey-patching
	// the wdutilCmd to a sh command that prints fixture output.
	c.wdutilCmd = []string{"/bin/sh", "-c",
		`printf 'WIFI\n  CCA             : 10 %%\n  NSS             : 2\n  Guard Interval  : 800\n  BSSID           : aa:bb:cc:dd:ee:ff\n  IPv4 Address    : 10.0.0.1\n  DNS             : 10.0.0.1\nWIFI FAULTS LAST HOUR\n  Total           : None\nWIFI RECOVERIES LAST HOUR\n  Total           : None\nWIFI LINK TESTS LAST HOUR\n  Total           : None\n'`}

	metrics := collectWdutilMetrics(t, c)
	// Expect: available + cca + nss + guardInterval + faultsLastHour + recoveriesLastHour + linkTestsLastHour + info = 8
	if len(metrics) != 8 {
		t.Errorf("expected 8 metrics, got %d", len(metrics))
		for _, m := range metrics {
			t.Logf("  metric: %v", m.Desc())
		}
	}
}

func TestCanSudo_CommandExists(t *testing.T) {
	// Verify that sudo binary exists; if it doesn't, skip.
	if _, err := exec.LookPath("sudo"); err != nil {
		t.Skip("sudo not found")
	}
	// Just ensure canSudo doesn't panic.
	result := canSudo()
	t.Logf("canSudo() = %v", result)
}

func findWdutilInfoMetric(t *testing.T, metrics []prometheus.Metric) *dto.Metric {
	t.Helper()
	for _, m := range metrics {
		if !strings.Contains(m.Desc().String(), `fqName: "darwin_wdutil_wifi_info"`) {
			continue
		}
		var dm dto.Metric
		if err := m.Write(&dm); err != nil {
			t.Fatalf("metric.Write() error: %v", err)
		}
		return &dm
	}
	t.Fatal("darwin_wdutil_wifi_info metric not found")
	return nil
}

func wdutilLabelValue(m *dto.Metric, name string) string {
	for _, lp := range m.GetLabel() {
		if lp.GetName() == name {
			return lp.GetValue()
		}
	}
	return ""
}

func TestWdutilCollector_BSSIDFallbackFromIPConfig(t *testing.T) {
	withMockIPConfigCommand(t, func(name string, args ...string) ([]byte, error) {
		if name != ipconfigBin {
			return nil, fmt.Errorf("unexpected command: %s %v", name, args)
		}
		if len(args) == 0 {
			return nil, fmt.Errorf("missing subcommand")
		}
		switch args[0] {
		case "getiflist":
			return []byte("en0\n"), nil
		case "getsummary":
			return []byte(`<dictionary> {
  InterfaceType : WiFi
  SSID : FallbackSSID
  BSSID : 48:a9:8a:0d:d0:a5
}`), nil
		case "setverbose":
			return []byte{}, nil
		default:
			return nil, fmt.Errorf("unexpected ipconfig args: %v", args)
		}
	})

	c := NewWdutilCollector(newWdutilTestLogger())
	c.wdutilCmd = []string{"/bin/sh", "-c",
		`printf 'WIFI\n  CCA             : 10 %%\n  NSS             : 2\n  Guard Interval  : 800\n  BSSID           : <redacted>\n  IPv4 Address    : 10.0.0.1\n  DNS             : 10.0.0.1\nWIFI FAULTS LAST HOUR\n  Total           : None\nWIFI RECOVERIES LAST HOUR\n  Total           : None\nWIFI LINK TESTS LAST HOUR\n  Total           : None\n'`}

	metrics := collectWdutilMetrics(t, c)
	dm := findWdutilInfoMetric(t, metrics)
	if got := wdutilLabelValue(dm, "interface"); got != "en0" {
		t.Fatalf("interface label=%q, want en0", got)
	}
	if got := wdutilLabelValue(dm, "bssid"); got != "48:a9:8a:0d:d0:a5" {
		t.Fatalf("bssid label=%q, want 48:a9:8a:0d:d0:a5", got)
	}
}
