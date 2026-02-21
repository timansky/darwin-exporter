//go:build darwin && cgo

package collector

/*
#cgo LDFLAGS: -framework CoreWLAN -framework Foundation -framework CoreFoundation
#include "wifi_darwin.h"
#include <stdlib.h>
*/
import "C"

import (
	"runtime"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

// trimAtNull returns s truncated at the first null byte.
// C.GoStringN may include bytes after the null terminator if the C string is
// shorter than maxLen; trimAtNull removes that trailing garbage.
func trimAtNull(s string) string {
	if idx := strings.IndexByte(s, 0); idx >= 0 {
		return s[:idx]
	}
	return s
}

// cCharArrayToString safely converts a C fixed-size char array to a Go string.
// maxLen must be the total size of the array (e.g. 256 for ssid[256]).
// The function limits reading to maxLen-1 bytes and trims at the first null byte,
// preventing buffer over-read when C code does not null-terminate the array.
func cCharArrayToString(arr *C.char, maxLen int) string {
	// C.GoStringN reads exactly maxLen bytes; we pass maxLen-1 to never
	// read the sentinel position, matching the CWiFiData contract.
	return trimAtNull(C.GoStringN(arr, C.int(maxLen-1)))
}

// globalCache holds the latest WiFi snapshot written by the CoreWLAN event
// goroutine and read lock-free by Prometheus scrape goroutines.
var globalCache atomic.Pointer[WiFiSnapshot]

// sprofilerEnrichment holds only the fields fetched from system_profiler
// that CoreWLAN cannot provide without Location Services (ssid, country_code).
type sprofilerEnrichment struct {
	SSID        string
	Security    string
	CountryCode string
	MCSIndex    float64
}

// globalSprofilerEnrichment holds the latest system_profiler enrichment data.
var globalSprofilerEnrichment atomic.Pointer[sprofilerEnrichment]

// sprofilerTTL is the interval between system_profiler polls.
const sprofilerTTL = 60 * time.Second

// CWChannelWidth enum values (CoreWLAN CWChannelWidth).
// See: https://developer.apple.com/documentation/corewlan/cwchannelwidth
const (
	cwChannelWidthUnknown = 0 // CWChannelWidthUnknown
	cwChannelWidth20      = 1 // CWChannelWidth20MHz
	cwChannelWidth40      = 2 // CWChannelWidth40MHz
	cwChannelWidth80      = 3 // CWChannelWidth80MHz
	cwChannelWidth160     = 4 // CWChannelWidth160MHz
)

// CWPHYMode enum values (CoreWLAN CWPHYMode).
// See: https://developer.apple.com/documentation/corewlan/cwphymode
const (
	cwPHYModeNone = 0 // CWPHYModeNone
	cwPHYMode11a  = 1 // CWPHYMode11a
	cwPHYMode11b  = 2 // CWPHYMode11b
	cwPHYMode11g  = 3 // CWPHYMode11g
	cwPHYMode11n  = 4 // CWPHYMode11n
	cwPHYMode11ac = 5 // CWPHYMode11ac
	cwPHYMode11ax = 6 // CWPHYMode11ax
)

// CWChannelBand enum values (CoreWLAN CWChannelBand).
// See: https://developer.apple.com/documentation/corewlan/cwchannelband
const (
	cwChannelBandUnknown = 0 // CWChannelBandUnknown
	cwChannelBand2GHz    = 1 // CWChannelBand2GHz
	cwChannelBand5GHz    = 2 // CWChannelBand5GHz
	cwChannelBand6GHz    = 3 // CWChannelBand6GHz
)

// goWiFiEventCallback is called from Objective-C delegate methods via C shim.
// It converts the C struct into a Go WiFiSnapshot and atomically updates the cache.
//
//export goWiFiEventCallback
func goWiFiEventCallback(data C.CWiFiData) {
	snap := &WiFiSnapshot{
		Timestamp: time.Now(),
		Source:    "corewlan",
		Info: WiFiInfo{
			Interface:  strings.TrimSpace(cCharArrayToString((*C.char)(unsafe.Pointer(&data.interface_name[0])), 32)),
			Connected:  data.connected != 0,
			PowerOn:    data.power_on != 0,
			RSSI:       float64(data.rssi),
			Noise:      float64(data.noise),
			TxRate:     float64(data.tx_rate),
			TxPower:    float64(data.tx_power),
			Channel:    float64(data.channel),
			PHYModeNum: float64(data.phy_mode),
			PHYMode:    phymodeLabel(int(data.phy_mode)),
			// cCharArrayToString limits reads to array size-1, preventing
			// buffer over-read when C code does not null-terminate.
			Security:     normalizeSecurityMode(normalizeSensitiveValue(cCharArrayToString((*C.char)(unsafe.Pointer(&data.security[0])), 64))),
			ChannelWidth: channelWidthMHz(int(data.channel_width)),
			Band:         channelBandLabel(int(data.channel_band)),
			SSID:         normalizeSensitiveValue(cCharArrayToString((*C.char)(unsafe.Pointer(&data.ssid[0])), 256)),
		},
	}
	globalCache.Store(snap)
}

// storeSnapshot constructs a WiFiSnapshot from a WiFiInfo and stores it atomically.
// Used by tests to inject state without going through the Objective-C callback.
func storeSnapshot(info WiFiInfo) {
	snap := &WiFiSnapshot{
		Timestamp: time.Now(),
		Source:    "corewlan",
		Info:      info,
	}
	globalCache.Store(snap)
}

// channelWidthMHz maps CWChannelWidth enum to MHz.
func channelWidthMHz(w int) float64 {
	switch w {
	case cwChannelWidthUnknown:
		return 0
	case cwChannelWidth20:
		return 20
	case cwChannelWidth40:
		return 40
	case cwChannelWidth80:
		return 80
	case cwChannelWidth160:
		return 160
	default:
		return 0
	}
}

// phymodeLabel maps CWPHYMode enum to a human-readable 802.11 standard string.
func phymodeLabel(m int) string {
	switch m {
	case cwPHYMode11a:
		return "802.11a"
	case cwPHYMode11b:
		return "802.11b"
	case cwPHYMode11g:
		return "802.11g"
	case cwPHYMode11n:
		return "802.11n"
	case cwPHYMode11ac:
		return "802.11ac"
	case cwPHYMode11ax:
		return "802.11ax"
	default:
		return "none"
	}
}

// channelBandLabel maps CWChannelBand enum to a human-readable label.
func channelBandLabel(b int) string {
	switch b {
	case cwChannelBand2GHz:
		return "2.4GHz"
	case cwChannelBand5GHz:
		return "5GHz"
	case cwChannelBand6GHz:
		return "6GHz"
	default:
		return "unknown"
	}
}

// fetchSprofilerEnrichment runs system_profiler and returns ssid+country_code.
func fetchSprofilerEnrichment(log *logrus.Logger) {
	info := &WiFiInfo{}
	haveParsedSource := false

	out, err := runCommand("system_profiler", "SPAirPortDataType", "-json")
	if err == nil {
		info, err = parseSystemProfilerOutput(out)
		if err != nil {
			log.WithError(err).Debug("system_profiler enrichment parse failed")
		} else {
			haveParsedSource = true
		}
	} else {
		log.WithError(err).Debug("system_profiler enrichment fetch failed")
	}

	// Fallback: ipconfig can still expose SSID/security even when CoreWLAN or
	// system_profiler redacts fields without Location Services permissions.
	ipSummary := fetchIPConfigWiFiSummary(log)
	if ipSummary != nil {
		haveParsedSource = true
		applyIPConfigFallback(info, ipSummary)
	}
	if !haveParsedSource {
		return
	}

	enr := &sprofilerEnrichment{
		SSID:        info.SSID,
		Security:    info.Security,
		CountryCode: info.CountryCode,
		MCSIndex:    info.MCSIndex,
	}
	globalSprofilerEnrichment.Store(enr)
	log.WithFields(logrus.Fields{
		"ssid":         enr.SSID,
		"security":     enr.Security,
		"country_code": enr.CountryCode,
		"mcs_index":    enr.MCSIndex,
	}).Debug("system_profiler enrichment updated")
}

// startSprofilerPoller runs system_profiler at startup and then every sprofilerTTL.
// It stops when done is closed.
func startSprofilerPoller(log *logrus.Logger, done <-chan struct{}) {
	fetchSprofilerEnrichment(log)
	ticker := time.NewTicker(sprofilerTTL)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			fetchSprofilerEnrichment(log)
		case <-done:
			return
		}
	}
}

// enrichedSSID returns the SSID from CoreWLAN if available, otherwise falls back
// to the system_profiler enrichment cache. Same for country_code.
func enrichedFields(coreSSID, coreSecurity string, coreMCSIndex float64) (ssid, security, countryCode string, mcsIndex float64) {
	ssid = coreSSID
	security = coreSecurity
	mcsIndex = coreMCSIndex
	enr := globalSprofilerEnrichment.Load()
	if enr == nil {
		return ssid, security, "", mcsIndex
	}
	if ssid == "" {
		ssid = enr.SSID
	}
	if security == "" {
		security = enr.Security
	}
	countryCode = enr.CountryCode
	// Prefer enrichment MCS to keep parity with !cgo builds that source MCS
	// from system_profiler.
	if enr.MCSIndex > 0 {
		mcsIndex = enr.MCSIndex
	}
	return ssid, security, countryCode, mcsIndex
}

// WiFiCollector collects WiFi metrics via CoreWLAN (CGo path).
type WiFiCollector struct {
	log *logrus.Logger

	rssi         *prometheus.Desc
	noise        *prometheus.Desc
	snr          *prometheus.Desc
	txRate       *prometheus.Desc
	channel      *prometheus.Desc
	connected    *prometheus.Desc
	info         *prometheus.Desc
	mcsIndex     *prometheus.Desc
	channelWidth *prometheus.Desc
	txPower      *prometheus.Desc
	phyMode      *prometheus.Desc

	done chan struct{} // closed by Close() to signal the monitor goroutine

	enrichmentFallbackLogged atomic.Bool
	ssidUnavailableLogged    atomic.Bool
}

// NewWiFiCollector creates a WiFiCollector and starts the CoreWLAN event loop.
func NewWiFiCollector(log *logrus.Logger) *WiFiCollector {
	const ns = "darwin"
	const sub = "wifi"

	c := &WiFiCollector{
		log:          log,
		rssi:         newDesc(ns, sub, "rssi_dbm", "WiFi RSSI signal strength in dBm."),
		noise:        newDesc(ns, sub, "noise_dbm", "WiFi noise level in dBm."),
		snr:          newDesc(ns, sub, "snr_db", "WiFi signal-to-noise ratio in dB (RSSI - Noise)."),
		txRate:       newDesc(ns, sub, "tx_rate_mbps", "WiFi transmit rate in Mbps."),
		channel:      newDesc(ns, sub, "channel", "WiFi channel number."),
		connected:    newDesc(ns, sub, "connected", "WiFi connection status (1 = connected, 0 = disconnected)."),
		info:         newDesc(ns, sub, "info", "WiFi connection information (value=1).", "interface", "ssid", "security", "band", "phymode", "country_code"),
		mcsIndex:     newDesc(ns, sub, "mcs_index", "WiFi MCS index."),
		channelWidth: newDesc(ns, sub, "channel_width_mhz", "WiFi channel width in MHz."),
		txPower:      newDesc(ns, sub, "tx_power_mw", "WiFi transmit power in mW."),
		phyMode:      newDesc(ns, sub, "phymode", "WiFi PHY mode (0=none,1=11a,2=11b,3=11g,4=11n,5=11ac,6=11ax)."),
		done:         make(chan struct{}),
	}

	// Start system_profiler poller for ssid/country_code enrichment (TTL-based).
	go startSprofilerPoller(log, c.done)

	// Start CoreWLAN monitor in a dedicated OS thread.
	go func() {
		runtime.LockOSThread()
		// startCoreWLANMonitor blocks until stopCoreWLANMonitor() is called.
		C.startCoreWLANMonitor()
	}()

	log.Debug("CoreWLAN WiFi monitor started")
	return c
}

// Close stops the CoreWLAN RunLoop and releases resources.
func (c *WiFiCollector) Close() error {
	select {
	case <-c.done:
		// Already closed.
	default:
		close(c.done)
		C.stopCoreWLANMonitor()
		c.log.Debug("CoreWLAN WiFi monitor stopped")
	}
	return nil
}

// Update reads the latest WiFiSnapshot from the atomic cache and emits metrics.
func (c *WiFiCollector) Update(ch chan<- prometheus.Metric) error {
	snap := globalCache.Load()
	if snap == nil {
		// Cache not yet populated (monitor just started).
		ch <- prometheus.MustNewConstMetric(c.connected, prometheus.GaugeValue, 0)
		return nil
	}

	info := snap.Info

	connected := 0.0
	if info.Connected {
		connected = 1.0
	}
	ch <- prometheus.MustNewConstMetric(c.connected, prometheus.GaugeValue, connected)

	if !info.Connected {
		return nil
	}

	ch <- prometheus.MustNewConstMetric(c.rssi, prometheus.GaugeValue, info.RSSI)
	ch <- prometheus.MustNewConstMetric(c.noise, prometheus.GaugeValue, info.Noise)
	ch <- prometheus.MustNewConstMetric(c.snr, prometheus.GaugeValue, info.RSSI-info.Noise)
	ch <- prometheus.MustNewConstMetric(c.txRate, prometheus.GaugeValue, info.TxRate)
	ch <- prometheus.MustNewConstMetric(c.channel, prometheus.GaugeValue, info.Channel)

	// Ensure enrichment cache is initialized before first scrape.
	if globalSprofilerEnrichment.Load() == nil {
		fetchSprofilerEnrichment(c.log)
	}

	// Enrich ssid and country_code from system_profiler cache when CoreWLAN
	// cannot provide them (macOS 14+ without Location Services permission).
	ssid, security, countryCode, mcsIndex := enrichedFields(info.SSID, info.Security, info.MCSIndex)
	if info.CountryCode != "" {
		countryCode = info.CountryCode
	}
	ssidFallback := info.SSID == "" && ssid != ""
	securityFallback := info.Security == "" && security != ""
	countryFallback := info.CountryCode == "" && countryCode != ""
	if ssidFallback || securityFallback || countryFallback {
		if c.enrichmentFallbackLogged.CompareAndSwap(false, true) {
			c.log.WithFields(logrus.Fields{
				"ssid_fallback":     ssidFallback,
				"security_fallback": securityFallback,
				"country_fallback":  countryFallback,
				"fallback_source":   "enrichment_cache",
				"primary_source":    "corewlan",
			}).Info("applied WiFi fallback fields")
		}
	} else {
		c.enrichmentFallbackLogged.Store(false)
	}
	if ssid == "" {
		if c.ssidUnavailableLogged.CompareAndSwap(false, true) {
			c.log.Warn("WiFi SSID is unavailable (redacted by macOS privacy controls)")
		}
	} else {
		c.ssidUnavailableLogged.Store(false)
	}
	ch <- prometheus.MustNewConstMetric(c.info, prometheus.GaugeValue, 1,
		info.Interface, ssid, security, info.Band, info.PHYMode, countryCode)

	if info.ChannelWidth > 0 {
		ch <- prometheus.MustNewConstMetric(c.channelWidth, prometheus.GaugeValue, info.ChannelWidth)
	}
	if mcsIndex > 0 {
		ch <- prometheus.MustNewConstMetric(c.mcsIndex, prometheus.GaugeValue, mcsIndex)
	}
	if info.TxPower > 0 {
		ch <- prometheus.MustNewConstMetric(c.txPower, prometheus.GaugeValue, info.TxPower)
	}
	ch <- prometheus.MustNewConstMetric(c.phyMode, prometheus.GaugeValue, info.PHYModeNum)

	return nil
}
