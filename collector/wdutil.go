//go:build darwin

package collector

import (
	"os"
	"os/exec"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

// WdutilCollector collects WiFi metrics from `wdutil info` that are not
// available through system_profiler: CCA, NSS, Guard Interval, and fault
// counters. Requires root or sudoers NOPASSWD for /usr/bin/wdutil.
type WdutilCollector struct {
	log *logrus.Logger

	// wdutilCmd is the command used to invoke wdutil info.
	// Set once during NewWdutilCollector based on privilege level.
	wdutilCmd []string

	available          *prometheus.Desc
	cca                *prometheus.Desc
	nss                *prometheus.Desc
	guardInterval      *prometheus.Desc
	faultsLastHour     *prometheus.Desc
	recoveriesLastHour *prometheus.Desc
	linkTestsLastHour  *prometheus.Desc
	info               *prometheus.Desc

	bssidUnavailableLogged atomic.Bool
	bssidFallbackLogged    atomic.Bool
}

// NewWdutilCollector creates a WdutilCollector and determines the wdutil
// invocation strategy (direct or via sudo) based on current privileges.
func NewWdutilCollector(log *logrus.Logger) *WdutilCollector {
	const ns = "darwin"
	const sub = "wdutil"

	c := &WdutilCollector{
		log:                log,
		available:          newDesc(ns, sub, "available", "1 if wdutil is accessible (root or sudoers), 0 otherwise."),
		cca:                newDesc(ns, sub, "wifi_cca_percent", "WiFi Clear Channel Assessment percentage (0-100)."),
		nss:                newDesc(ns, sub, "wifi_nss", "WiFi Number of Spatial Streams."),
		guardInterval:      newDesc(ns, sub, "wifi_guard_interval_ns", "WiFi Guard Interval in nanoseconds."),
		faultsLastHour:     newDesc(ns, sub, "wifi_faults_last_hour", "WiFi fault count in the last hour (sliding window)."),
		recoveriesLastHour: newDesc(ns, sub, "wifi_recoveries_last_hour", "WiFi recovery count in the last hour (sliding window)."),
		linkTestsLastHour:  newDesc(ns, sub, "wifi_link_tests_last_hour", "WiFi link test count in the last hour (sliding window)."),
		info:               newDesc(ns, sub, "wifi_info", "WiFi connection info from wdutil (value=1).", "interface", "bssid", "ipv4_address", "dns_server"),
	}

	if os.Geteuid() == 0 {
		c.wdutilCmd = []string{"/usr/bin/wdutil", "info"}
	} else if canSudo() {
		c.wdutilCmd = []string{"sudo", "-n", "/usr/bin/wdutil", "info"}
	} else {
		log.Warn("wdutil collector enabled but no root/sudo access; " +
			"run: darwin-exporter service install --type=sudo OR darwin-exporter service install --type=root")
		c.wdutilCmd = nil
	}

	return c
}

// canSudo checks whether sudo -n /usr/bin/wdutil info is permitted without a
// password (i.e. sudoers NOPASSWD is configured for the current user).
func canSudo() bool {
	err := exec.Command("sudo", "-n", "-l", "/usr/bin/wdutil", "info").Run()
	return err == nil
}

// Update collects wdutil metrics and emits them to ch.
func (c *WdutilCollector) Update(ch chan<- prometheus.Metric) error {
	if c.wdutilCmd == nil {
		// No access: emit available=0 and return.
		ch <- prometheus.MustNewConstMetric(c.available, prometheus.GaugeValue, 0)
		return nil
	}

	out, err := runCommand(c.wdutilCmd[0], c.wdutilCmd[1:]...)
	if err != nil {
		c.log.WithError(err).Warn("failed to run wdutil info")
		ch <- prometheus.MustNewConstMetric(c.available, prometheus.GaugeValue, 0)
		return nil
	}

	wi, err := parseWdutilOutput(out)
	if err != nil {
		c.log.WithError(err).Warn("failed to parse wdutil output")
		ch <- prometheus.MustNewConstMetric(c.available, prometheus.GaugeValue, 0)
		return nil
	}

	if !wi.Available {
		ch <- prometheus.MustNewConstMetric(c.available, prometheus.GaugeValue, 0)
		return nil
	}

	ch <- prometheus.MustNewConstMetric(c.available, prometheus.GaugeValue, 1)
	ch <- prometheus.MustNewConstMetric(c.cca, prometheus.GaugeValue, wi.CCA)
	ch <- prometheus.MustNewConstMetric(c.nss, prometheus.GaugeValue, wi.NSS)
	ch <- prometheus.MustNewConstMetric(c.guardInterval, prometheus.GaugeValue, wi.GuardInterval)
	ch <- prometheus.MustNewConstMetric(c.faultsLastHour, prometheus.GaugeValue, wi.FaultsLastHour)
	ch <- prometheus.MustNewConstMetric(c.recoveriesLastHour, prometheus.GaugeValue, wi.RecoveriesLastHour)
	ch <- prometheus.MustNewConstMetric(c.linkTestsLastHour, prometheus.GaugeValue, wi.LinkTestsLastHour)
	iface, ipconfigBSSID := "", ""
	if wi.BSSID == "" || wi.Interface == "" {
		iface, ipconfigBSSID = fetchIPConfigWiFiBSSID(c.log)
		if wi.Interface == "" && iface != "" {
			wi.Interface = iface
		}
	}
	if wi.BSSID == "" {
		if ipconfigBSSID != "" {
			wi.BSSID = ipconfigBSSID
			c.bssidUnavailableLogged.Store(false)
			if c.bssidFallbackLogged.CompareAndSwap(false, true) {
				c.log.WithFields(logrus.Fields{
					"interface":       iface,
					"fallback_source": "ipconfig",
					"primary_source":  "wdutil",
				}).Info("applied WiFi BSSID fallback")
			}
		} else {
			c.bssidFallbackLogged.Store(false)
			if c.bssidUnavailableLogged.CompareAndSwap(false, true) {
				if iface != "" {
					c.log.WithField("interface", iface).Warn("WiFi BSSID is unavailable (redacted by macOS privacy controls)")
				} else {
					c.log.Warn("WiFi BSSID is unavailable (redacted by macOS privacy controls)")
				}
			}
		}
	} else {
		c.bssidFallbackLogged.Store(false)
		c.bssidUnavailableLogged.Store(false)
	}
	if wi.Interface == "" {
		if summary := fetchIPConfigWiFiSummary(c.log); summary != nil {
			wi.Interface = summary.Interface
		}
	}
	ch <- prometheus.MustNewConstMetric(c.info, prometheus.GaugeValue, 1,
		wi.Interface, wi.BSSID, wi.IPv4Address, wi.DNSServer)

	return nil
}
