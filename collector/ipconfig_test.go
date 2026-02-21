//go:build darwin

package collector

import (
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"
)

func withMockIPConfigCommand(t *testing.T, fn func(name string, args ...string) ([]byte, error)) {
	t.Helper()
	prev := runIPConfigCommand
	runIPConfigCommand = fn
	t.Cleanup(func() {
		runIPConfigCommand = prev
		ipconfigSSIDUnavailableLogged.Store(false)
	})
}

func TestParseIPConfigGetIfListOutput(t *testing.T) {
	out := []byte("en4 en5 en0 en0 bridge0\n")
	got := parseIPConfigGetIfListOutput(out)
	want := []string{"en4", "en5", "en0", "bridge0"}
	if len(got) != len(want) {
		t.Fatalf("len(got)=%d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestPrioritizeInterfaces(t *testing.T) {
	got := prioritizeInterfaces([]string{"en4", "bridge0", "en5"})
	if len(got) == 0 || got[0] != "en0" {
		t.Fatalf("expected en0 first, got %#v", got)
	}
}

func TestParseIPConfigSummaryOutput(t *testing.T) {
	out := []byte(`<dictionary> {
  BSSID : 48:a9:8a:0d:d0:a5
  InterfaceType : WiFi
  SSID : MyNetwork
  Security : WPA3_SAE
}`)
	got := parseIPConfigSummaryOutput(out)
	if got.InterfaceType != "WiFi" {
		t.Fatalf("InterfaceType=%q, want WiFi", got.InterfaceType)
	}
	if got.SSID != "MyNetwork" {
		t.Fatalf("SSID=%q, want MyNetwork", got.SSID)
	}
	if got.BSSID != "48:a9:8a:0d:d0:a5" {
		t.Fatalf("BSSID=%q, want 48:a9:8a:0d:d0:a5", got.BSSID)
	}
	if got.Security != "WPA3_SAE" {
		t.Fatalf("Security=%q, want WPA3_SAE", got.Security)
	}
}

func TestParseIPConfigSummaryOutput_RedactedSSID(t *testing.T) {
	out := []byte(`<dictionary> {
  InterfaceType : WiFi
  SSID : <redacted>
  Security : WPA3_SAE
}`)
	got := parseIPConfigSummaryOutput(out)
	if got.SSID != "" {
		t.Fatalf("SSID=%q, want empty for redacted value", got.SSID)
	}
	if got.Security != "WPA3_SAE" {
		t.Fatalf("Security=%q, want WPA3_SAE", got.Security)
	}
}

func TestApplyIPConfigFallback(t *testing.T) {
	info := &WiFiInfo{SSID: "", Security: ""}
	applyIPConfigFallback(info, &ipconfigSummary{
		SSID:     "FallbackSSID",
		Security: "WPA2",
	})
	if info.SSID != "FallbackSSID" {
		t.Fatalf("SSID=%q, want FallbackSSID", info.SSID)
	}
	if info.Security != "WPA2" {
		t.Fatalf("Security=%q, want WPA2", info.Security)
	}
}

func TestFetchIPConfigWiFiSummary_UsesSetVerboseRetry(t *testing.T) {
	var setVerboseOnCalls int
	var setVerboseOffCalls int
	var getSummaryCalls int

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
			getSummaryCalls++
			if getSummaryCalls == 1 {
				return []byte("<dictionary> {\n  InterfaceType : WiFi\n  Security : WPA3_SAE\n}\n"), nil
			}
			return []byte("<dictionary> {\n  InterfaceType : WiFi\n  SSID : RetrySSID\n  Security : WPA3_SAE\n}\n"), nil
		case "setverbose":
			if len(args) != 2 {
				return nil, fmt.Errorf("unexpected setverbose args: %v", args)
			}
			switch args[1] {
			case "1":
				setVerboseOnCalls++
			case "0":
				setVerboseOffCalls++
			default:
				return nil, fmt.Errorf("unexpected setverbose value: %q", args[1])
			}
			return []byte{}, nil
		default:
			return nil, fmt.Errorf("unexpected ipconfig args: %v", args)
		}
	})

	got := fetchIPConfigWiFiSummary(logrus.New())
	if got == nil {
		t.Fatal("expected summary, got nil")
	}
	if got.SSID != "RetrySSID" {
		t.Fatalf("SSID=%q, want RetrySSID", got.SSID)
	}
	if setVerboseOnCalls != 1 {
		t.Fatalf("setverbose 1 calls=%d, want 1", setVerboseOnCalls)
	}
	if setVerboseOffCalls != 1 {
		t.Fatalf("setverbose 0 calls=%d, want 1", setVerboseOffCalls)
	}
}

func TestFetchIPConfigWiFiSummary_NoSetVerboseWhenSSIDPresent(t *testing.T) {
	var setVerboseCalls int

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
			return []byte("<dictionary> {\n  InterfaceType : WiFi\n  SSID : DirectSSID\n}\n"), nil
		case "setverbose":
			setVerboseCalls++
			return []byte{}, nil
		default:
			return nil, fmt.Errorf("unexpected ipconfig args: %v", args)
		}
	})

	got := fetchIPConfigWiFiSummary(logrus.New())
	if got == nil {
		t.Fatal("expected summary, got nil")
	}
	if got.SSID != "DirectSSID" {
		t.Fatalf("SSID=%q, want DirectSSID", got.SSID)
	}
	if setVerboseCalls != 0 {
		t.Fatalf("setverbose calls=%d, want 0", setVerboseCalls)
	}
}

func TestFetchIPConfigWiFiBSSID_PicksFirstNonEmptyBSSID(t *testing.T) {
	withMockIPConfigCommand(t, func(name string, args ...string) ([]byte, error) {
		if name != ipconfigBin {
			return nil, fmt.Errorf("unexpected command: %s %v", name, args)
		}
		if len(args) == 0 {
			return nil, fmt.Errorf("missing subcommand")
		}
		switch args[0] {
		case "getiflist":
			return []byte("en0 en4\n"), nil
		case "getsummary":
			if len(args) != 2 {
				return nil, fmt.Errorf("unexpected getsummary args: %v", args)
			}
			switch args[1] {
			case "en0":
				return []byte("<dictionary> {\n  InterfaceType : WiFi\n  SSID : Home\n  BSSID : <redacted>\n}\n"), nil
			case "en4":
				return []byte("<dictionary> {\n  InterfaceType : WiFi\n  SSID : Other\n  BSSID : 11:22:33:44:55:66\n}\n"), nil
			default:
				return nil, fmt.Errorf("unexpected iface: %s", args[1])
			}
		case "setverbose":
			return []byte{}, nil
		default:
			return nil, fmt.Errorf("unexpected ipconfig args: %v", args)
		}
	})

	iface, bssid := fetchIPConfigWiFiBSSID(logrus.New())
	if iface != "en4" {
		t.Fatalf("iface=%q, want en4", iface)
	}
	if bssid != "11:22:33:44:55:66" {
		t.Fatalf("bssid=%q, want 11:22:33:44:55:66", bssid)
	}
}

func TestFetchIPConfigWiFiBSSID_UsesSetVerboseRetryAndRestore(t *testing.T) {
	var setVerboseOnCalls int
	var setVerboseOffCalls int
	var getSummaryCalls int

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
			getSummaryCalls++
			if getSummaryCalls == 1 {
				return []byte("<dictionary> {\n  InterfaceType : WiFi\n  BSSID : <redacted>\n}\n"), nil
			}
			return []byte("<dictionary> {\n  InterfaceType : WiFi\n  BSSID : aa:bb:cc:dd:ee:ff\n}\n"), nil
		case "setverbose":
			if len(args) != 2 {
				return nil, fmt.Errorf("unexpected setverbose args: %v", args)
			}
			switch args[1] {
			case "1":
				setVerboseOnCalls++
			case "0":
				setVerboseOffCalls++
			default:
				return nil, fmt.Errorf("unexpected setverbose value: %q", args[1])
			}
			return []byte{}, nil
		default:
			return nil, fmt.Errorf("unexpected ipconfig args: %v", args)
		}
	})

	iface, bssid := fetchIPConfigWiFiBSSID(logrus.New())
	if iface != "en0" {
		t.Fatalf("iface=%q, want en0", iface)
	}
	if bssid != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("bssid=%q, want aa:bb:cc:dd:ee:ff", bssid)
	}
	if setVerboseOnCalls != 1 {
		t.Fatalf("setverbose 1 calls=%d, want 1", setVerboseOnCalls)
	}
	if setVerboseOffCalls != 1 {
		t.Fatalf("setverbose 0 calls=%d, want 1", setVerboseOffCalls)
	}
}
