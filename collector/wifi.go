//go:build darwin

package collector

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// WiFiInfo holds parsed WiFi data.
type WiFiInfo struct {
	// Fields populated by both system_profiler and CoreWLAN paths.
	Interface    string
	RSSI         float64
	Noise        float64
	TxRate       float64
	Channel      float64
	SSID         string
	Security     string
	Band         string
	Connected    bool
	ChannelWidth float64 // MHz (20/40/80/160)

	// Fields populated by system_profiler path.
	MCSIndex    float64 // spairport_network_mcs
	PHYMode     string  // spairport_network_phymode ("802.11ac")
	CountryCode string  // spairport_network_country_code

	// Fields populated by CoreWLAN (CGo) path only.
	TxPower       float64 // transmitPower in mW
	PHYModeNum    float64 // CWPHYMode enum: 0-6
	PowerOn       bool    // CWInterface.powerOn
	ServiceActive bool    // CWInterface.serviceActive
}

// WiFiSnapshot is an atomic cache entry for the CoreWLAN event-driven path.
type WiFiSnapshot struct {
	Info      WiFiInfo
	Timestamp time.Time
	Source    string // "corewlan" or "system_profiler"
}

// spAirPortData is the top-level JSON structure from system_profiler SPAirPortDataType -json.
type spAirPortData struct {
	SPAirPortDataType []spAirPortDataType `json:"SPAirPortDataType"`
}

type spAirPortDataType struct {
	Interfaces []spAirPortInterface `json:"spairport_airport_interfaces"`
}

type spAirPortInterface struct {
	Name           string            `json:"_name"`
	StatusInfo     string            `json:"spairport_status_information"`
	CurrentNetwork *spAirPortNetwork `json:"spairport_current_network_information"`
}

type spAirPortNetwork struct {
	Name        string  `json:"_name"`
	SignalNoise string  `json:"spairport_signal_noise"`
	TxRate      float64 `json:"spairport_network_rate"`
	Channel     string  `json:"spairport_network_channel"`
	Security    string  `json:"spairport_security_mode"`
	MCSIndex    int     `json:"spairport_network_mcs"`
	PHYMode     string  `json:"spairport_network_phymode"`
	CountryCode string  `json:"spairport_network_country_code"`
}

// parseSystemProfilerOutput parses the JSON output of system_profiler SPAirPortDataType -json.
func parseSystemProfilerOutput(data []byte) (*WiFiInfo, error) {
	var sp spAirPortData
	if err := json.Unmarshal(data, &sp); err != nil {
		return nil, fmt.Errorf("parsing system_profiler JSON: %w", err)
	}

	if len(sp.SPAirPortDataType) == 0 || len(sp.SPAirPortDataType[0].Interfaces) == 0 {
		return &WiFiInfo{}, nil
	}

	iface := selectSystemProfilerInterface(sp.SPAirPortDataType[0].Interfaces)
	info := &WiFiInfo{
		Interface: strings.TrimSpace(iface.Name),
	}

	if iface.StatusInfo == "spairport_status_connected" && iface.CurrentNetwork != nil {
		info.Connected = true
		net := iface.CurrentNetwork
		info.SSID = normalizeSensitiveValue(net.Name)
		info.Security = normalizeSecurityMode(normalizeSensitiveValue(net.Security))
		info.TxRate = net.TxRate
		info.Channel, info.Band, info.ChannelWidth = parseChannelBand(net.Channel)
		info.RSSI, info.Noise = parseSignalNoise(net.SignalNoise)
		info.MCSIndex = float64(net.MCSIndex)
		info.PHYMode = net.PHYMode
		info.CountryCode = net.CountryCode
	}

	return info, nil
}

func selectSystemProfilerInterface(ifaces []spAirPortInterface) spAirPortInterface {
	if len(ifaces) == 0 {
		return spAirPortInterface{}
	}
	for _, iface := range ifaces {
		if iface.StatusInfo == "spairport_status_connected" && iface.CurrentNetwork != nil {
			return iface
		}
	}
	return ifaces[0]
}

func normalizeSecurityMode(v string) string {
	s := strings.ToLower(strings.TrimSpace(v))
	if s == "" {
		return ""
	}
	switch {
	case strings.Contains(s, "wpa3"):
		return "WPA3"
	case strings.Contains(s, "wpa2"):
		return "WPA2"
	case strings.Contains(s, "wpa"):
		return "WPA"
	case strings.Contains(s, "wep"):
		return "WEP"
	case strings.Contains(s, "open"), strings.Contains(s, "none"):
		return "none"
	default:
		return strings.TrimSpace(v)
	}
}

// parseChannelBand extracts channel number, band, and channel width from a
// string like "44 (5GHz, 80MHz)".
func parseChannelBand(s string) (channel float64, band string, widthMHz float64) {
	if s == "" {
		return 0, "", 0
	}
	// Channel number is the first token before a space.
	parts := strings.SplitN(s, " ", 2)
	if v, err := strconv.ParseFloat(parts[0], 64); err == nil {
		channel = v
	}
	// Band is in the parenthetical suffix.
	switch {
	case strings.Contains(s, "6GHz"):
		band = "6GHz"
	case strings.Contains(s, "5GHz"):
		band = "5GHz"
	default:
		band = "2.4GHz"
	}
	// Channel width from suffix like "80MHz".
	switch {
	case strings.Contains(s, "160MHz"):
		widthMHz = 160
	case strings.Contains(s, "80MHz"):
		widthMHz = 80
	case strings.Contains(s, "40MHz"):
		widthMHz = 40
	case strings.Contains(s, "20MHz"):
		widthMHz = 20
	}
	return channel, band, widthMHz
}

// parseSignalNoise extracts RSSI and noise from a string like "-70 dBm / -95 dBm".
func parseSignalNoise(s string) (rssi, noise float64) {
	if s == "" {
		return 0, 0
	}
	// Format: "<rssi> dBm / <noise> dBm"
	parts := strings.Split(s, " / ")
	if len(parts) != 2 {
		return 0, 0
	}
	rssi, _ = strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(parts[0]), " dBm"), 64)
	noise, _ = strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(parts[1]), " dBm"), 64)
	return rssi, noise
}
