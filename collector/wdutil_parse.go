//go:build darwin

package collector

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
)

// WdutilInfo holds parsed data from `wdutil info` output.
type WdutilInfo struct {
	Available          bool
	CCA                float64 // Clear Channel Assessment (0-100%)
	NSS                float64 // Number of Spatial Streams
	GuardInterval      float64 // Guard interval in nanoseconds (400/800/1600/3200)
	FaultsLastHour     float64
	RecoveriesLastHour float64
	LinkTestsLastHour  float64
	Interface          string
	BSSID              string
	IPv4Address        string
	DNSServer          string
}

// section tracks which wdutil output section we are currently parsing.
type section int

const (
	sectionOther      section = iota
	sectionWifi               // WIFI
	sectionFaults             // WIFI FAULTS LAST HOUR
	sectionRecoveries         // WIFI RECOVERIES LAST HOUR
	sectionLinkTests          // WIFI LINK TESTS LAST HOUR
)

// parseWdutilOutput parses the plain-text output of `wdutil info`.
// It extracts the WIFI section fields and the WIFI FAULTS/RECOVERIES/LINK TESTS
// sub-sections. Returns a non-nil struct even on parse errors;
// Available is set to true when at least one numeric field was successfully parsed.
func parseWdutilOutput(data []byte) (*WdutilInfo, error) {
	info := &WdutilInfo{}
	current := sectionOther
	anyNumeric := false

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			continue
		}

		// Section headers have no ":" and consist of uppercase words.
		// They also have no leading whitespace (or minimal indentation).
		if !strings.Contains(trimmed, ":") {
			upper := strings.ToUpper(trimmed)
			switch {
			case strings.Contains(upper, "WIFI FAULTS"):
				current = sectionFaults
			case strings.Contains(upper, "WIFI RECOVERIES"):
				current = sectionRecoveries
			case strings.Contains(upper, "WIFI LINK TESTS"):
				current = sectionLinkTests
			case upper == "WIFI":
				current = sectionWifi
			case strings.HasPrefix(upper, "—"):
				// Separator line (em-dashes), keep current section.
				continue
			default:
				// Any other all-caps section header resets to other.
				// Detect by checking it is fully uppercase (allow spaces/digits).
				allUpper := true
				for _, r := range upper {
					if r >= 'a' && r <= 'z' {
						allUpper = false
						break
					}
				}
				if allUpper {
					current = sectionOther
				}
			}
			continue
		}

		// Only parse content in known sections.
		if current == sectionOther {
			continue
		}

		// Parse key : value line.
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}

		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		keyUpper := strings.ToUpper(key)

		switch current {
		case sectionWifi:
			switch keyUpper {
			case "CCA":
				// Value like "5 %" — strip the % suffix.
				numStr := strings.TrimSuffix(strings.TrimSpace(val), "%")
				numStr = strings.TrimSpace(numStr)
				if f := firstFloat(numStr); f != 0 || numStr == "0" {
					info.CCA = f
					anyNumeric = true
				}

			case "NSS":
				if f := firstFloat(val); f > 0 {
					info.NSS = f
					anyNumeric = true
				}

			case "GUARD INTERVAL":
				if f := firstFloat(val); f > 0 {
					info.GuardInterval = f
					anyNumeric = true
				}

			case "INTERFACE NAME":
				info.Interface = strings.TrimSpace(val)

			case "BSSID":
				info.BSSID = normalizeSensitiveValue(val)

			case "IPV4 ADDRESS", "IPV4 ADDRESSES":
				info.IPv4Address = normalizeSensitiveValue(val)

			case "DNS", "DNS ADDRESSES", "DNS ADDRESS":
				info.DNSServer = normalizeSensitiveValue(val)
			}

		case sectionFaults:
			if keyUpper == "TOTAL" {
				info.FaultsLastHour = parseNoneOrFloat(val)
				anyNumeric = true
			}

		case sectionRecoveries:
			if keyUpper == "TOTAL" {
				info.RecoveriesLastHour = parseNoneOrFloat(val)
				anyNumeric = true
			}

		case sectionLinkTests:
			if keyUpper == "TOTAL" {
				info.LinkTestsLastHour = parseNoneOrFloat(val)
				anyNumeric = true
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return info, err
	}

	if anyNumeric {
		info.Available = true
	}

	return info, nil
}

// firstFloat returns the first whitespace-separated float64 token in s,
// or 0 if none is found.
func firstFloat(s string) float64 {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(fields[0], 64)
	return v
}

// parseNoneOrFloat returns 0 for "None", otherwise attempts to parse a float.
func parseNoneOrFloat(s string) float64 {
	if strings.EqualFold(strings.TrimSpace(s), "none") {
		return 0
	}
	return firstFloat(s)
}
