//go:build darwin

package collector

import (
	"testing"
)

// sampleSystemProfilerConnected is a minimal JSON response from system_profiler
// SPAirPortDataType -json when WiFi is connected on 5GHz with all new fields.
const sampleSystemProfilerConnected = `{
  "SPAirPortDataType": [{
    "spairport_airport_interfaces": [{
      "_name": "en0",
      "spairport_status_information": "spairport_status_connected",
      "spairport_current_network_information": {
        "_name": "MyNetwork",
        "spairport_signal_noise": "-55 dBm / -90 dBm",
        "spairport_network_rate": 300,
        "spairport_network_channel": "36 (5GHz, 80MHz)",
        "spairport_security_mode": "wpa2-psk",
        "spairport_network_mcs": 9,
        "spairport_network_phymode": "802.11ac",
        "spairport_network_country_code": "US"
      }
    }]
  }]
}`

// sampleSystemProfilerConnected24 is a response for a 2.4GHz connection.
const sampleSystemProfilerConnected24 = `{
  "SPAirPortDataType": [{
    "spairport_airport_interfaces": [{
      "_name": "en0",
      "spairport_status_information": "spairport_status_connected",
      "spairport_current_network_information": {
        "_name": "HomeWifi",
        "spairport_signal_noise": "-60 dBm / -95 dBm",
        "spairport_network_rate": 144,
        "spairport_network_channel": "6 (2GHz, 20MHz)",
        "spairport_security_mode": "wpa2-psk"
      }
    }]
  }]
}`

// sampleSystemProfilerConnectedRedacted is a response where macOS redacts
// sensitive WiFi fields.
const sampleSystemProfilerConnectedRedacted = `{
  "SPAirPortDataType": [{
    "spairport_airport_interfaces": [{
      "_name": "en0",
      "spairport_status_information": "spairport_status_connected",
      "spairport_current_network_information": {
        "_name": "<redacted>",
        "spairport_signal_noise": "-60 dBm / -95 dBm",
        "spairport_network_rate": 144,
        "spairport_network_channel": "6 (2GHz, 20MHz)",
        "spairport_security_mode": "<private>"
      }
    }]
  }]
}`

// sampleSystemProfilerDisconnected is a response when the interface is not connected.
const sampleSystemProfilerDisconnected = `{
  "SPAirPortDataType": [{
    "spairport_airport_interfaces": [{
      "_name": "en0",
      "spairport_status_information": "spairport_status_not_connected"
    }]
  }]
}`

// sampleSystemProfilerEmpty is a response with no interfaces.
const sampleSystemProfilerEmpty = `{
  "SPAirPortDataType": []
}`

func TestParseSystemProfilerOutput_Connected(t *testing.T) {
	info, err := parseSystemProfilerOutput([]byte(sampleSystemProfilerConnected))
	if err != nil {
		t.Fatalf("parseSystemProfilerOutput error: %v", err)
	}

	if !info.Connected {
		t.Error("expected Connected=true")
	}
	if info.Interface != "en0" {
		t.Errorf("expected Interface=en0, got %q", info.Interface)
	}
	if info.RSSI != -55 {
		t.Errorf("expected RSSI=-55, got %v", info.RSSI)
	}
	if info.Noise != -90 {
		t.Errorf("expected Noise=-90, got %v", info.Noise)
	}
	if info.TxRate != 300 {
		t.Errorf("expected TxRate=300, got %v", info.TxRate)
	}
	if info.Channel != 36 {
		t.Errorf("expected Channel=36, got %v", info.Channel)
	}
	if info.SSID != "MyNetwork" {
		t.Errorf("expected SSID=MyNetwork, got %q", info.SSID)
	}
	if info.Security != "WPA2" {
		t.Errorf("expected Security=WPA2, got %q", info.Security)
	}
	if info.Band != "5GHz" {
		t.Errorf("expected Band=5GHz, got %q", info.Band)
	}
}

func TestParseSystemProfilerOutput_SelectsConnectedInterface(t *testing.T) {
	input := `{
  "SPAirPortDataType": [{
    "spairport_airport_interfaces": [{
      "_name": "awdl0",
      "spairport_status_information": "spairport_status_not_connected"
    },{
      "_name": "en0",
      "spairport_status_information": "spairport_status_connected",
      "spairport_current_network_information": {
        "_name": "MyNetwork",
        "spairport_signal_noise": "-55 dBm / -90 dBm",
        "spairport_network_rate": 300,
        "spairport_network_channel": "36 (5GHz, 80MHz)",
        "spairport_security_mode": "wpa2-psk",
        "spairport_network_mcs": 9,
        "spairport_network_phymode": "802.11ac",
        "spairport_network_country_code": "US"
      }
    }]
  }]
}`
	info, err := parseSystemProfilerOutput([]byte(input))
	if err != nil {
		t.Fatalf("parseSystemProfilerOutput error: %v", err)
	}
	if info.Interface != "en0" {
		t.Fatalf("expected connected interface en0, got %q", info.Interface)
	}
	if !info.Connected {
		t.Fatal("expected connected=true")
	}
}

func TestParseSystemProfilerOutput_NewFields(t *testing.T) {
	info, err := parseSystemProfilerOutput([]byte(sampleSystemProfilerConnected))
	if err != nil {
		t.Fatalf("parseSystemProfilerOutput error: %v", err)
	}

	if info.MCSIndex != 9 {
		t.Errorf("expected MCSIndex=9, got %v", info.MCSIndex)
	}
	if info.PHYMode != "802.11ac" {
		t.Errorf("expected PHYMode=802.11ac, got %q", info.PHYMode)
	}
	if info.CountryCode != "US" {
		t.Errorf("expected CountryCode=US, got %q", info.CountryCode)
	}
	if info.ChannelWidth != 80 {
		t.Errorf("expected ChannelWidth=80, got %v", info.ChannelWidth)
	}
}

func TestParseSystemProfilerOutput_2GHz(t *testing.T) {
	info, err := parseSystemProfilerOutput([]byte(sampleSystemProfilerConnected24))
	if err != nil {
		t.Fatalf("parseSystemProfilerOutput error: %v", err)
	}
	if info.Band != "2.4GHz" {
		t.Errorf("expected Band=2.4GHz, got %q", info.Band)
	}
	if info.Channel != 6 {
		t.Errorf("expected Channel=6, got %v", info.Channel)
	}
	if info.ChannelWidth != 20 {
		t.Errorf("expected ChannelWidth=20, got %v", info.ChannelWidth)
	}
}

func TestParseSystemProfilerOutput_RedactedSSID(t *testing.T) {
	info, err := parseSystemProfilerOutput([]byte(sampleSystemProfilerConnectedRedacted))
	if err != nil {
		t.Fatalf("parseSystemProfilerOutput error: %v", err)
	}
	if info.SSID != "" {
		t.Errorf("expected redacted SSID to normalize to empty, got %q", info.SSID)
	}
	if info.Security != "" {
		t.Errorf("expected redacted Security to normalize to empty, got %q", info.Security)
	}
}

func TestNormalizeSecurityMode(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"wpa2-psk", "WPA2"},
		{"pairport_security_mode_wpa3_transition", "WPA3"},
		{"WPA3 Personal", "WPA3"},
		{"none", "none"},
		{"", ""},
		{"custom-security", "custom-security"},
	}
	for _, tc := range cases {
		if got := normalizeSecurityMode(tc.in); got != tc.want {
			t.Errorf("normalizeSecurityMode(%q)=%q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseSystemProfilerOutput_Disconnected(t *testing.T) {
	info, err := parseSystemProfilerOutput([]byte(sampleSystemProfilerDisconnected))
	if err != nil {
		t.Fatalf("parseSystemProfilerOutput error: %v", err)
	}
	if info.Connected {
		t.Error("expected Connected=false for disconnected interface")
	}
	if info.Interface != "en0" {
		t.Errorf("expected Interface=en0, got %q", info.Interface)
	}
}

func TestParseSystemProfilerOutput_Empty(t *testing.T) {
	info, err := parseSystemProfilerOutput([]byte(sampleSystemProfilerEmpty))
	if err != nil {
		t.Fatalf("parseSystemProfilerOutput error: %v", err)
	}
	if info.Connected {
		t.Error("expected Connected=false when no interfaces present")
	}
}

func TestParseSystemProfilerOutput_SNR(t *testing.T) {
	info, err := parseSystemProfilerOutput([]byte(sampleSystemProfilerConnected))
	if err != nil {
		t.Fatalf("parseSystemProfilerOutput error: %v", err)
	}
	// SNR = RSSI - Noise = -55 - (-90) = 35
	snr := info.RSSI - info.Noise
	if snr != 35 {
		t.Errorf("expected SNR=35, got %v", snr)
	}
}

func TestParseChannelBand(t *testing.T) {
	cases := []struct {
		input    string
		channel  float64
		band     string
		widthMHz float64
	}{
		{"44 (5GHz, 80MHz)", 44, "5GHz", 80},
		{"6 (2GHz, 20MHz)", 6, "2.4GHz", 20},
		{"36 (5GHz, 40MHz)", 36, "5GHz", 40},
		{"149 (5GHz, 80MHz)", 149, "5GHz", 80},
		{"100 (5GHz, 160MHz)", 100, "5GHz", 160},
		{"", 0, "", 0},
	}
	for _, tc := range cases {
		ch, band, width := parseChannelBand(tc.input)
		if ch != tc.channel {
			t.Errorf("parseChannelBand(%q) channel = %v, want %v", tc.input, ch, tc.channel)
		}
		if band != tc.band {
			t.Errorf("parseChannelBand(%q) band = %q, want %q", tc.input, band, tc.band)
		}
		if width != tc.widthMHz {
			t.Errorf("parseChannelBand(%q) widthMHz = %v, want %v", tc.input, width, tc.widthMHz)
		}
	}
}

func TestParseSignalNoise(t *testing.T) {
	cases := []struct {
		input string
		rssi  float64
		noise float64
	}{
		{"-70 dBm / -95 dBm", -70, -95},
		{"-55 dBm / -90 dBm", -55, -90},
		{"", 0, 0},
	}
	for _, tc := range cases {
		rssi, noise := parseSignalNoise(tc.input)
		if rssi != tc.rssi {
			t.Errorf("parseSignalNoise(%q) rssi = %v, want %v", tc.input, rssi, tc.rssi)
		}
		if noise != tc.noise {
			t.Errorf("parseSignalNoise(%q) noise = %v, want %v", tc.input, noise, tc.noise)
		}
	}
}
