//go:build darwin

package collector

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

// Reference:
// Battery metrics intentionally omit fields that are already exported by
// node_exporter power-supply metrics (node_power_supply_*):
// - https://github.com/prometheus/node_exporter
// - https://github.com/prometheus/node_exporter/blob/master/collector/powersupplyclass_linux.go
//
// BatteryCollector collects battery metrics via pmset and ioreg.
// Metrics already covered by node_exporter node_power_supply_* are omitted:
//   - charge level          → node_power_supply_current_capacity
//   - charging status       → node_power_supply_charging
//   - current amperes       → node_power_supply_current_ampere
//   - time remaining        → node_power_supply_time_to_empty/full_seconds
//   - power source state    → node_power_supply_power_source_state
type BatteryCollector struct {
	log *logrus.Logger

	cycleCount    *prometheus.Desc
	healthPercent *prometheus.Desc
	voltageVolts  *prometheus.Desc
	tempCelsius   *prometheus.Desc
}

// NewBatteryCollector creates a BatteryCollector.
func NewBatteryCollector(log *logrus.Logger) *BatteryCollector {
	const ns = "darwin"
	const sub = "battery"
	return &BatteryCollector{
		log:           log,
		cycleCount:    newDesc(ns, sub, "cycle_count", "Battery charge cycle count."),
		healthPercent: newDesc(ns, sub, "health_percent", "Battery health as percentage of design capacity."),
		voltageVolts:  newDesc(ns, sub, "voltage_volts", "Battery voltage in volts."),
		tempCelsius:   newDesc(ns, sub, "temperature_celsius", "Battery temperature in degrees Celsius."),
	}
}

// BatteryInfo holds parsed battery data.
type BatteryInfo struct {
	CycleCount    float64
	HealthPercent float64
	VoltageVolts  float64
	TempCelsius   float64
}

// Update collects battery metrics.
func (c *BatteryCollector) Update(ch chan<- prometheus.Metric) error {
	info, err := c.queryBattery()
	if err != nil {
		c.log.WithError(err).Warn("failed to query battery, metrics unavailable")
		return err
	}

	// Extended metrics from ioreg (may not be available on all machines).
	if info.CycleCount > 0 {
		ch <- prometheus.MustNewConstMetric(c.cycleCount, prometheus.GaugeValue, info.CycleCount)
	}
	if info.HealthPercent > 0 {
		ch <- prometheus.MustNewConstMetric(c.healthPercent, prometheus.GaugeValue, info.HealthPercent)
	}
	if info.VoltageVolts != 0 {
		ch <- prometheus.MustNewConstMetric(c.voltageVolts, prometheus.GaugeValue, info.VoltageVolts)
	}
	if info.TempCelsius > 0 {
		ch <- prometheus.MustNewConstMetric(c.tempCelsius, prometheus.GaugeValue, info.TempCelsius)
	}

	return nil
}

// queryBattery fetches battery data from ioreg.
func (c *BatteryCollector) queryBattery() (*BatteryInfo, error) {
	info := &BatteryInfo{}

	// Parse ioreg for extended battery data.
	ioreOut, err := runCommand("ioreg", "-rc", "AppleSmartBattery", "-w0")
	if err != nil {
		// Not fatal — ioreg may not be available (e.g., desktop Mac without battery).
		c.log.WithError(err).Debug("ioreg AppleSmartBattery unavailable, skipping extended metrics")
		return info, nil
	}
	parseIoregBattery(ioreOut, info)

	return info, nil
}

// parseIoregBattery extracts battery data from ioreg -rc AppleSmartBattery output.
// ioreg outputs key = value pairs in a property list format.
func parseIoregBattery(data []byte, info *BatteryInfo) {
	// ioreg -rc AppleSmartBattery -w0 outputs key = value pairs.
	// We look for specific keys.
	keyRe := regexp.MustCompile(`"(\w+)" = (\S+)`)
	matches := keyRe.FindAllSubmatch(data, -1)

	var maxCapacity, designCapacity float64

	for _, m := range matches {
		key := string(m[1])
		val := strings.TrimRight(string(m[2]), ",")

		switch key {
		case "CycleCount":
			if v, err := strconv.ParseFloat(val, 64); err == nil {
				info.CycleCount = v
			}
		case "MaxCapacity":
			if v, err := strconv.ParseFloat(val, 64); err == nil {
				maxCapacity = v
			}
		case "DesignCapacity":
			if v, err := strconv.ParseFloat(val, 64); err == nil {
				designCapacity = v
			}
		case "Voltage":
			// Voltage is in millivolts.
			if v, err := strconv.ParseFloat(val, 64); err == nil {
				info.VoltageVolts = v / 1000.0
			}
		case "Temperature":
			// Temperature is in units of 0.01 degrees Celsius.
			if v, err := strconv.ParseFloat(val, 64); err == nil {
				info.TempCelsius = v / 100.0
			}
		}
	}

	if designCapacity > 0 && maxCapacity > 0 {
		info.HealthPercent = (maxCapacity / designCapacity) * 100.0
	}
}
