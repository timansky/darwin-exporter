//go:build darwin

package collector

import (
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/sirupsen/logrus"
)

const ipconfigBin = "/usr/sbin/ipconfig"

var (
	// runIPConfigCommand allows tests to mock command execution.
	runIPConfigCommand = runCommand

	// ipconfigVerboseMu serializes temporary verbose-mode retries.
	ipconfigVerboseMu sync.Mutex

	// ipconfigSSIDUnavailableLogged suppresses repeated warnings when macOS
	// keeps redacting SSID values.
	ipconfigSSIDUnavailableLogged atomic.Bool
)

// ipconfigSummary contains the WiFi-related fields extracted from
// `ipconfig getsummary <iface>`.
type ipconfigSummary struct {
	Interface     string
	InterfaceType string
	SSID          string
	BSSID         string
	Security      string
}

// parseIPConfigGetIfListOutput parses `ipconfig getiflist` output into a
// de-duplicated list of interfaces preserving order.
func parseIPConfigGetIfListOutput(out []byte) []string {
	fields := strings.Fields(string(out))
	ifaces := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, iface := range fields {
		iface = strings.TrimSpace(iface)
		if iface == "" {
			continue
		}
		if _, ok := seen[iface]; ok {
			continue
		}
		seen[iface] = struct{}{}
		ifaces = append(ifaces, iface)
	}
	return ifaces
}

// prioritizeInterfaces orders interfaces so likely WiFi interfaces are queried first.
func prioritizeInterfaces(ifaces []string) []string {
	out := make([]string, 0, len(ifaces)+1)
	seen := make(map[string]struct{}, len(ifaces)+1)
	add := func(iface string) {
		if iface == "" {
			return
		}
		if _, ok := seen[iface]; ok {
			return
		}
		seen[iface] = struct{}{}
		out = append(out, iface)
	}

	// Most Macs use en0 for the primary WiFi adapter.
	add("en0")
	for _, iface := range ifaces {
		if strings.HasPrefix(iface, "en") {
			add(iface)
		}
	}
	for _, iface := range ifaces {
		add(iface)
	}
	return out
}

// parseIPConfigSummaryOutput extracts top-level key/value fields from
// `ipconfig getsummary <iface>` text output.
func parseIPConfigSummaryOutput(out []byte) ipconfigSummary {
	var s ipconfigSummary
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "}" || strings.HasSuffix(line, "{") {
			continue
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = normalizeIPConfigValue(val)
		switch key {
		case "InterfaceType":
			s.InterfaceType = val
		case "SSID":
			s.SSID = normalizeSensitiveValue(val)
		case "BSSID":
			s.BSSID = normalizeSensitiveValue(val)
		case "Security":
			s.Security = normalizeSensitiveValue(val)
		}
	}
	return s
}

func normalizeIPConfigValue(v string) string {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "{") && strings.HasSuffix(v, "}") {
		inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(v, "{"), "}"))
		if !strings.Contains(inner, ",") {
			return inner
		}
	}
	return v
}

func readIPConfigSummary(iface string) (*ipconfigSummary, error) {
	out, err := runIPConfigCommand(ipconfigBin, "getsummary", iface)
	if err != nil {
		return nil, err
	}
	s := parseIPConfigSummaryOutput(out)
	s.Interface = iface
	return &s, nil
}

// fetchIPConfigWiFiSummary finds a WiFi interface from ipconfig and returns its
// summary, preferring a summary with non-empty SSID.
func fetchIPConfigWiFiSummary(log *logrus.Logger) *ipconfigSummary {
	out, err := runIPConfigCommand(ipconfigBin, "getiflist")
	if err != nil {
		if log != nil {
			log.WithError(err).Error("ipconfig getiflist failed")
		}
		return nil
	}

	ifaces := prioritizeInterfaces(parseIPConfigGetIfListOutput(out))
	firstWiFi, withSSID := scanIPConfigWiFiSummaries(ifaces)
	if withSSID != nil {
		ipconfigSSIDUnavailableLogged.Store(false)
		return withSSID
	}
	if firstWiFi == nil || firstWiFi.SSID != "" {
		if firstWiFi != nil && firstWiFi.SSID != "" {
			ipconfigSSIDUnavailableLogged.Store(false)
		}
		return firstWiFi
	}

	// Some systems expose SSID in getsummary only when IPConfiguration verbose
	// mode is enabled. Enable it only for this retry phase, then restore.
	retryFirst, retryWithSSID := retryIPConfigScanWithVerbose(log, ifaces, scanIPConfigWiFiSummaries)
	if retryWithSSID != nil {
		if log != nil {
			log.WithFields(logrus.Fields{
				"interface": retryWithSSID.Interface,
				"ssid":      retryWithSSID.SSID,
			}).Info("ipconfig fallback recovered SSID after enabling verbose mode")
		}
		ipconfigSSIDUnavailableLogged.Store(false)
		return retryWithSSID
	}
	if retryFirst != nil {
		logRedactedSSIDWarning(log, retryFirst.Interface)
		return retryFirst
	}
	logRedactedSSIDWarning(log, firstWiFi.Interface)
	return firstWiFi
}

// fetchIPConfigWiFiBSSID returns the first non-empty WiFi BSSID exposed by
// `ipconfig getsummary <iface>`, preferring en0 and retrying once after
// enabling verbose mode.
func fetchIPConfigWiFiBSSID(log *logrus.Logger) (iface, bssid string) {
	out, err := runIPConfigCommand(ipconfigBin, "getiflist")
	if err != nil {
		if log != nil {
			log.WithError(err).Error("ipconfig getiflist failed")
		}
		return "", ""
	}

	ifaces := prioritizeInterfaces(parseIPConfigGetIfListOutput(out))
	firstWiFi, withBSSID := scanIPConfigWiFiBSSID(ifaces)
	if withBSSID != nil {
		return withBSSID.Interface, withBSSID.BSSID
	}

	// On some systems BSSID appears only after enabling verbose mode.
	retryFirst, retryWithBSSID := retryIPConfigScanWithVerbose(log, ifaces, scanIPConfigWiFiBSSID)
	if retryWithBSSID != nil {
		return retryWithBSSID.Interface, retryWithBSSID.BSSID
	}
	if retryFirst != nil {
		return retryFirst.Interface, ""
	}
	if firstWiFi != nil {
		return firstWiFi.Interface, ""
	}
	return "", ""
}

func scanIPConfigWiFiSummaries(ifaces []string) (firstWiFi, withSSID *ipconfigSummary) {
	for _, iface := range ifaces {
		s, err := readIPConfigSummary(iface)
		if err != nil {
			continue
		}
		if s.InterfaceType != "WiFi" {
			continue
		}
		if firstWiFi == nil {
			first := *s
			firstWiFi = &first
		}
		if s.SSID != "" {
			return firstWiFi, s
		}
	}
	return firstWiFi, nil
}

func scanIPConfigWiFiBSSID(ifaces []string) (firstWiFi, withBSSID *ipconfigSummary) {
	for _, iface := range ifaces {
		s, err := readIPConfigSummary(iface)
		if err != nil {
			continue
		}
		if s.InterfaceType != "WiFi" {
			continue
		}
		if firstWiFi == nil {
			first := *s
			firstWiFi = &first
		}
		if s.BSSID != "" {
			return firstWiFi, s
		}
	}
	return firstWiFi, nil
}

func retryIPConfigScanWithVerbose(
	log *logrus.Logger,
	ifaces []string,
	scan func([]string) (firstWiFi, withValue *ipconfigSummary),
) (firstWiFi, withValue *ipconfigSummary) {
	ipconfigVerboseMu.Lock()
	defer ipconfigVerboseMu.Unlock()

	cmdPrefix := enableIPConfigVerbose(log)
	if len(cmdPrefix) == 0 {
		return scan(ifaces)
	}
	defer disableIPConfigVerbose(log, cmdPrefix)

	return scan(ifaces)
}

func enableIPConfigVerbose(log *logrus.Logger) []string {
	directCmd := []string{ipconfigBin}
	if err := runIPConfigSetVerbose(directCmd, "1"); err == nil {
		if log != nil {
			log.Debug("enabled ipconfig verbose mode")
		}
		return directCmd
	} else if log != nil {
		log.WithError(err).Error("ipconfig setverbose 1 failed")
	}

	if os.Geteuid() != 0 {
		sudoCmd := []string{"sudo", "-n", ipconfigBin}
		if err := runIPConfigSetVerbose(sudoCmd, "1"); err == nil {
			if log != nil {
				log.Debug("enabled ipconfig verbose mode via sudo")
			}
			return sudoCmd
		} else if log != nil {
			log.WithError(err).Error("sudo -n ipconfig setverbose 1 failed")
		}
	}

	if log != nil {
		log.Error("failed to enable ipconfig verbose mode")
	}
	return nil
}

func disableIPConfigVerbose(log *logrus.Logger, cmdPrefix []string) {
	if len(cmdPrefix) == 0 {
		return
	}
	err := runIPConfigSetVerbose(cmdPrefix, "0")
	if err == nil {
		if log != nil {
			log.Debug("restored ipconfig verbose mode to default")
		}
		return
	}
	if log != nil {
		log.WithError(err).Warn("failed to restore ipconfig verbose mode to default")
	}
}

func runIPConfigSetVerbose(cmdPrefix []string, value string) error {
	args := make([]string, 0, len(cmdPrefix)+1)
	if len(cmdPrefix) > 1 {
		args = append(args, cmdPrefix[1:]...)
	}
	args = append(args, "setverbose", value)
	_, err := runIPConfigCommand(cmdPrefix[0], args...)
	return err
}

func logRedactedSSIDWarning(log *logrus.Logger, iface string) {
	if log == nil {
		return
	}
	if !ipconfigSSIDUnavailableLogged.CompareAndSwap(false, true) {
		return
	}
	log.WithField("interface", iface).Warn(
		"WiFi SSID is redacted by macOS privacy controls (Location Services/TCC); exporter cannot read plain SSID",
	)
}

// applyIPConfigFallback fills missing WiFi fields from ipconfig summary.
func applyIPConfigFallback(info *WiFiInfo, s *ipconfigSummary) (ssidApplied, securityApplied bool) {
	if info == nil || s == nil {
		return false, false
	}
	if info.SSID == "" {
		info.SSID = s.SSID
		ssidApplied = s.SSID != ""
	}
	if info.Security == "" {
		info.Security = s.Security
		securityApplied = s.Security != ""
	}
	return ssidApplied, securityApplied
}
