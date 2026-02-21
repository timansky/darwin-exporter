//go:build darwin

package collector

import (
	"testing"
)

// wdutilFullOutput is a fixture representing a full `wdutil info` output
// with all WIFI fields populated.
var wdutilFullOutput = []byte(`
SYSTEM

  System version  : 14.3
  Uptime          : 5 days

WIFI

  Interface Name  : en0
  CCA             : 5 %
  NSS             : 1
  Guard Interval  : 800
  SSID            : <redacted>
  BSSID           : aa:bb:cc:dd:ee:ff
  IPv4 Address    : 172.20.20.20
  DNS             : 172.20.20.53

WIFI FAULTS LAST HOUR

  Total           : None

WIFI RECOVERIES LAST HOUR

  Total           : None

WIFI LINK TESTS LAST HOUR

  Total           : None

BLUETOOTH

  Status          : On
`)

// wdutilFaultsOutput is a fixture where faults/recoveries/link tests have values.
var wdutilFaultsOutput = []byte(`
WIFI

  Interface Name  : en1
  CCA             : 12 %
  NSS             : 2
  Guard Interval  : 400
  BSSID           : 11:22:33:44:55:66
  IPv4 Address    : 10.0.0.1
  DNS             : 8.8.8.8

WIFI FAULTS LAST HOUR

  Total           : 3

WIFI RECOVERIES LAST HOUR

  Total           : 2

WIFI LINK TESTS LAST HOUR

  Total           : 5
`)

// wdutilNoWifiOutput has no WIFI section.
var wdutilNoWifiOutput = []byte(`
SYSTEM

  System version  : 14.3

BLUETOOTH

  Status          : Off
`)

// wdutilEmptyOutput is completely empty.
var wdutilEmptyOutput = []byte(``)

// wdutilOnlyHeaderOutput has a WIFI section header but no fields.
var wdutilOnlyHeaderOutput = []byte(`
WIFI
`)

// wdutilRealMacOS15Output is actual output from macOS 15.4 with <redacted> fields.
var wdutilRealMacOS15Output = []byte(`
————————————————————————————————————————————————————————————————————
NETWORK
————————————————————————————————————————————————————————————————————
    Primary IPv4         : en0 (Wi-Fi / 0808D86F-1896-4B19-9C3E-62E32A42561C)
                         : 172.20.20.20
    Primary IPv6         : None
    DNS Addresses        : 172.20.20.53
    Apple                : Reachable
————————————————————————————————————————————————————————————————————
WIFI
————————————————————————————————————————————————————————————————————
    MAC Address          : <redacted> (hw=<redacted>)
    Interface Name       : en0
    Power                : On [On]
    Op Mode              : STA
    SSID                 : <redacted>
    BSSID                : <redacted>
    RSSI                 : -71 dBm
    CCA                  : 3 %
    Noise                : -95 dBm
    Tx Rate              : 288.0 Mbps
    Security             : WPA3 Personal
    PHY Mode             : 11ax
    MCS Index            : 3
    Guard Interval       : 1600
    NSS                  : 2
    Channel              : 5g44/80
    Country Code         : KZ
    IPv4 Address         : 172.20.20.20
    DNS                  : 172.20.20.53

WIFI FAULTS LAST HOUR
————————————————————————————————————————————————————————————————————
    None
————————————————————————————————————————————————————————————————————
WIFI RECOVERIES LAST HOUR
————————————————————————————————————————————————————————————————————
    None
`)

func TestParseWdutilOutput_RealMacOS15(t *testing.T) {
	info, err := parseWdutilOutput(wdutilRealMacOS15Output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.Available {
		t.Error("expected Available=true for real macOS 15 output")
	}
	if info.CCA != 3 {
		t.Errorf("CCA: got %v, want 3", info.CCA)
	}
	if info.Interface != "en0" {
		t.Errorf("Interface: got %q, want en0", info.Interface)
	}
	if info.NSS != 2 {
		t.Errorf("NSS: got %v, want 2", info.NSS)
	}
	if info.GuardInterval != 1600 {
		t.Errorf("GuardInterval: got %v, want 1600", info.GuardInterval)
	}
	if info.IPv4Address != "172.20.20.20" {
		t.Errorf("IPv4Address: got %q, want 172.20.20.20", info.IPv4Address)
	}
	if info.DNSServer != "172.20.20.53" {
		t.Errorf("DNSServer: got %q, want 172.20.20.53", info.DNSServer)
	}
	if info.BSSID != "" {
		t.Errorf("BSSID: got %q, want empty for redacted value", info.BSSID)
	}
}

func TestParseWdutilOutput_Full(t *testing.T) {
	info, err := parseWdutilOutput(wdutilFullOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.Available {
		t.Error("expected Available=true for full output")
	}
	if info.CCA != 5 {
		t.Errorf("CCA: got %v, want 5", info.CCA)
	}
	if info.Interface != "en0" {
		t.Errorf("Interface: got %q, want en0", info.Interface)
	}
	if info.NSS != 1 {
		t.Errorf("NSS: got %v, want 1", info.NSS)
	}
	if info.GuardInterval != 800 {
		t.Errorf("GuardInterval: got %v, want 800", info.GuardInterval)
	}
	if info.BSSID != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("BSSID: got %q, want aa:bb:cc:dd:ee:ff", info.BSSID)
	}
	if info.IPv4Address != "172.20.20.20" {
		t.Errorf("IPv4Address: got %q, want 172.20.20.20", info.IPv4Address)
	}
	if info.DNSServer != "172.20.20.53" {
		t.Errorf("DNSServer: got %q, want 172.20.20.53", info.DNSServer)
	}
	if info.FaultsLastHour != 0 {
		t.Errorf("FaultsLastHour: got %v, want 0 (None)", info.FaultsLastHour)
	}
	if info.RecoveriesLastHour != 0 {
		t.Errorf("RecoveriesLastHour: got %v, want 0 (None)", info.RecoveriesLastHour)
	}
	if info.LinkTestsLastHour != 0 {
		t.Errorf("LinkTestsLastHour: got %v, want 0 (None)", info.LinkTestsLastHour)
	}
}

func TestParseWdutilOutput_FaultsNonZero(t *testing.T) {
	info, err := parseWdutilOutput(wdutilFaultsOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.Available {
		t.Error("expected Available=true")
	}
	if info.CCA != 12 {
		t.Errorf("CCA: got %v, want 12", info.CCA)
	}
	if info.Interface != "en1" {
		t.Errorf("Interface: got %q, want en1", info.Interface)
	}
	if info.NSS != 2 {
		t.Errorf("NSS: got %v, want 2", info.NSS)
	}
	if info.GuardInterval != 400 {
		t.Errorf("GuardInterval: got %v, want 400", info.GuardInterval)
	}
	if info.FaultsLastHour != 3 {
		t.Errorf("FaultsLastHour: got %v, want 3", info.FaultsLastHour)
	}
	if info.RecoveriesLastHour != 2 {
		t.Errorf("RecoveriesLastHour: got %v, want 2", info.RecoveriesLastHour)
	}
	if info.LinkTestsLastHour != 5 {
		t.Errorf("LinkTestsLastHour: got %v, want 5", info.LinkTestsLastHour)
	}
}

func TestParseWdutilOutput_NoWifi(t *testing.T) {
	info, err := parseWdutilOutput(wdutilNoWifiOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Available {
		t.Error("expected Available=false when no WIFI section")
	}
}

func TestParseWdutilOutput_Empty(t *testing.T) {
	info, err := parseWdutilOutput(wdutilEmptyOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Available {
		t.Error("expected Available=false for empty output")
	}
}

func TestParseWdutilOutput_OnlyHeader(t *testing.T) {
	info, err := parseWdutilOutput(wdutilOnlyHeaderOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Available {
		t.Error("expected Available=false for header-only output")
	}
}

func TestParseNoneOrFloat(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"None", 0},
		{"none", 0},
		{"NONE", 0},
		{"3", 3},
		{"  5  ", 5},
		{"", 0},
		{"abc", 0},
	}
	for _, tt := range tests {
		got := parseNoneOrFloat(tt.input)
		if got != tt.want {
			t.Errorf("parseNoneOrFloat(%q): got %v, want %v", tt.input, got, tt.want)
		}
	}
}
