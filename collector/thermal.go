//go:build darwin

package collector

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

// Thermal pressure states mapped from NSProcessInfo thermalState numeric values.
// 0=nominal, 1=fair (light throttle), 2=serious (heavy throttle), 3=critical.
const (
	thermalNominal  = "nominal"
	thermalFair     = "fair"
	thermalSerious  = "serious"
	thermalCritical = "critical"
)

// thermalStates is the ordered list of valid thermal states.
var thermalStates = []string{thermalNominal, thermalFair, thermalSerious, thermalCritical}

const (
	powermetricsBin = "/usr/bin/powermetrics"
)

var (
	powermetricsArgs = []string{"-n", "1", "-i", "500", "--samplers", "cpu_power"}
	numberTokenRe    = regexp.MustCompile(`[-+]?\d+(?:\.\d+)?`)
	tempWithUnitRe   = regexp.MustCompile(`(?i)([-+]?\d+(?:\.\d+)?)\s*°?\s*([CF])\b`)
	smcDiskKeyRe     = regexp.MustCompile(`(?i)^T[HN]([0-9])[A-Za-z]$`)
)

// ThermalStateProvider returns current thermal pressure state.
type ThermalStateProvider interface {
	CurrentState() (string, error)
}

// SMCProvider discovers and reads SMC keys.
type SMCProvider interface {
	DiscoverKeySets() ([]string, []string, []string, error)
	ReadValues(keys []string) (map[string]float64, error)
}

// PowermetricsProvider reads CPU temperature from powermetrics fallback path.
type PowermetricsProvider interface {
	Available() bool
	ReadCPUTemperature() (float64, error)
}

type defaultThermalStateProvider struct{}

func (defaultThermalStateProvider) CurrentState() (string, error) {
	return queryThermalState()
}

type defaultSMCProvider struct{}

func (defaultSMCProvider) DiscoverKeySets() ([]string, []string, []string, error) {
	return discoverSMCKeySets()
}

func (defaultSMCProvider) ReadValues(keys []string) (map[string]float64, error) {
	return readSMCKeyValues(keys)
}

type defaultPowermetricsProvider struct {
	cmdPrefix []string
}

func newDefaultPowermetricsProvider() *defaultPowermetricsProvider {
	p := &defaultPowermetricsProvider{}
	if os.Geteuid() == 0 {
		p.cmdPrefix = []string{powermetricsBin}
	} else if canSudoPowermetrics() {
		p.cmdPrefix = []string{"sudo", "-n", powermetricsBin}
	}
	return p
}

func (p *defaultPowermetricsProvider) Available() bool {
	return p != nil && len(p.cmdPrefix) > 0
}

func (p *defaultPowermetricsProvider) ReadCPUTemperature() (float64, error) {
	if !p.Available() {
		return 0, fmt.Errorf("powermetrics is not accessible (need root or sudoers)")
	}
	args := append([]string{}, p.cmdPrefix[1:]...)
	args = append(args, powermetricsArgs...)
	out, err := runCommand(p.cmdPrefix[0], args...)
	if err != nil {
		return 0, err
	}
	temp, err := parsePowermetricsCPUTemperature(out)
	if err != nil {
		return 0, fmt.Errorf("parsing powermetrics cpu temperature: %w", err)
	}
	return temp, nil
}

// ThermalCollector collects macOS thermal pressure metrics.
type ThermalCollector struct {
	log      *logrus.Logger
	pressure *prometheus.Desc
	cpuTemp  *prometheus.Desc
	gpuTemp  *prometheus.Desc
	diskTemp *prometheus.Desc

	stateProvider ThermalStateProvider
	smcProvider   SMCProvider
	powermetrics  PowermetricsProvider
	smcCPUKeys    []string
	smcGPUKeys    []string
	smcDiskKeys   []string

	cpuTempUnavailableLogged  atomic.Bool
	gpuTempUnavailableLogged  atomic.Bool
	diskTempUnavailableLogged atomic.Bool
}

// NewThermalCollector creates a ThermalCollector.
func NewThermalCollector(log *logrus.Logger) *ThermalCollector {
	return NewThermalCollectorWithProviders(
		log,
		defaultSMCProvider{},
		newDefaultPowermetricsProvider(),
		defaultThermalStateProvider{},
	)
}

// NewThermalCollectorWithProviders creates a ThermalCollector with injected
// providers. Intended for tests and staged refactoring.
func NewThermalCollectorWithProviders(
	log *logrus.Logger,
	smcProvider SMCProvider,
	powermetricsProvider PowermetricsProvider,
	stateProvider ThermalStateProvider,
) *ThermalCollector {
	if smcProvider == nil {
		smcProvider = defaultSMCProvider{}
	}
	if powermetricsProvider == nil {
		powermetricsProvider = newDefaultPowermetricsProvider()
	}
	if stateProvider == nil {
		stateProvider = defaultThermalStateProvider{}
	}

	c := &ThermalCollector{
		log:           log,
		stateProvider: stateProvider,
		smcProvider:   smcProvider,
		powermetrics:  powermetricsProvider,
		pressure: prometheus.NewDesc(
			"darwin_thermal_pressure",
			"macOS thermal pressure state (value=1 for current state).",
			[]string{"state"},
			nil,
		),
		cpuTemp: prometheus.NewDesc(
			"darwin_cpu_temperature_celsius",
			"CPU package/die temperature in degrees Celsius (best effort via built-in SMC, fallback powermetrics).",
			nil,
			nil,
		),
		gpuTemp: prometheus.NewDesc(
			"darwin_gpu_temperature_celsius",
			"GPU package/die temperature in degrees Celsius (best effort via built-in SMC).",
			nil,
			nil,
		),
		diskTemp: prometheus.NewDesc(
			"darwin_disk_temperature_celsius",
			"Disk/NAND temperature in degrees Celsius (best effort via built-in SMC).",
			[]string{"device"},
			nil,
		),
	}

	cpuKeys, gpuKeys, diskKeys, err := c.smcProvider.DiscoverKeySets()
	if err != nil {
		log.WithError(err).Debug("SMC temperature keys unavailable")
	} else {
		c.smcCPUKeys = cpuKeys
		c.smcGPUKeys = gpuKeys
		c.smcDiskKeys = diskKeys
	}

	return c
}

// Update collects thermal pressure metrics.
func (c *ThermalCollector) Update(ch chan<- prometheus.Metric) error {
	state, err := c.stateProvider.CurrentState()
	if err != nil {
		c.log.WithError(err).Warn("failed to query thermal state, thermal metrics unavailable")
		return err
	}

	// Emit 1 for the current state, 0 for all others.
	for _, s := range thermalStates {
		val := 0.0
		if s == state {
			val = 1.0
		}
		ch <- prometheus.MustNewConstMetric(c.pressure, prometheus.GaugeValue, val, s)
	}

	if cpuTemp, err := c.queryCPUTemperature(); err == nil {
		ch <- prometheus.MustNewConstMetric(c.cpuTemp, prometheus.GaugeValue, cpuTemp)
		c.cpuTempUnavailableLogged.Store(false)
	} else if c.cpuTempUnavailableLogged.CompareAndSwap(false, true) {
		c.log.WithError(err).Debug("cpu temperature unavailable")
	}

	if gpuTemp, err := c.queryGPUTemperature(); err == nil {
		ch <- prometheus.MustNewConstMetric(c.gpuTemp, prometheus.GaugeValue, gpuTemp)
		c.gpuTempUnavailableLogged.Store(false)
	} else if c.gpuTempUnavailableLogged.CompareAndSwap(false, true) {
		c.log.WithError(err).Debug("gpu temperature unavailable")
	}

	if diskTemps, err := c.queryDiskTemperatures(); err == nil {
		devs := make([]string, 0, len(diskTemps))
		for dev := range diskTemps {
			devs = append(devs, dev)
		}
		sort.Strings(devs)
		for _, dev := range devs {
			ch <- prometheus.MustNewConstMetric(c.diskTemp, prometheus.GaugeValue, diskTemps[dev], dev)
		}
		c.diskTempUnavailableLogged.Store(false)
	} else if c.diskTempUnavailableLogged.CompareAndSwap(false, true) {
		c.log.WithError(err).Debug("disk temperature unavailable")
	}

	return nil
}

func canSudoPowermetrics() bool {
	args := append([]string{"-n", "-l", powermetricsBin}, powermetricsArgs...)
	return exec.Command("sudo", args...).Run() == nil
}

func (c *ThermalCollector) queryCPUTemperature() (float64, error) {
	var smcErr error
	if len(c.smcCPUKeys) > 0 {
		values, err := c.smcProvider.ReadValues(c.smcCPUKeys)
		if err != nil {
			smcErr = fmt.Errorf("reading SMC CPU keys: %w", err)
		} else if avg, ok := averageSMCTemperature(values, 10, 120); ok {
			return avg, nil
		} else {
			smcErr = fmt.Errorf("no valid SMC CPU temperatures")
		}
	} else {
		smcErr = fmt.Errorf("SMC CPU keys are unavailable")
	}

	if c.powermetrics == nil || !c.powermetrics.Available() {
		return 0, fmt.Errorf("%w; powermetrics is not accessible (need root or sudoers)", smcErr)
	}

	temp, err := c.powermetrics.ReadCPUTemperature()
	if err != nil {
		return 0, fmt.Errorf("%w; powermetrics fallback failed: %w", smcErr, err)
	}
	return temp, nil
}

func (c *ThermalCollector) queryDiskTemperatures() (map[string]float64, error) {
	if len(c.smcDiskKeys) == 0 {
		return nil, fmt.Errorf("SMC disk keys are unavailable")
	}

	values, err := c.smcProvider.ReadValues(c.smcDiskKeys)
	if err != nil {
		return nil, fmt.Errorf("reading SMC disk keys: %w", err)
	}
	return collectDiskTemperaturesFromSMC(values)
}

func (c *ThermalCollector) queryGPUTemperature() (float64, error) {
	if len(c.smcGPUKeys) == 0 {
		return 0, fmt.Errorf("SMC GPU keys are unavailable")
	}

	values, err := c.smcProvider.ReadValues(c.smcGPUKeys)
	if err != nil {
		return 0, fmt.Errorf("reading SMC GPU keys: %w", err)
	}
	avg, ok := averageSMCTemperature(values, 10, 130)
	if !ok {
		return 0, fmt.Errorf("no valid SMC GPU temperatures")
	}
	return avg, nil
}

func parsePowermetricsCPUTemperature(data []byte) (float64, error) {
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "cpu") || !strings.Contains(lower, "temp") {
			continue
		}

		unitMatches := tempWithUnitRe.FindAllStringSubmatch(line, -1)
		if len(unitMatches) > 0 {
			last := unitMatches[len(unitMatches)-1]
			v, err := strconv.ParseFloat(last[1], 64)
			if err != nil {
				continue
			}
			if strings.EqualFold(last[2], "F") {
				v = (v - 32.0) * (5.0 / 9.0)
			}
			return v, nil
		}

		num := numberTokenRe.FindString(line)
		if num != "" {
			v, err := strconv.ParseFloat(num, 64)
			if err == nil {
				return v, nil
			}
		}
	}
	return 0, fmt.Errorf("no cpu temperature line found")
}

func averageSMCTemperature(values map[string]float64, min, max float64) (float64, bool) {
	sum := 0.0
	count := 0.0
	for _, temp := range values {
		if temp < min || temp > max {
			continue
		}
		sum += temp
		count++
	}
	if count == 0 {
		return 0, false
	}
	return sum / count, true
}

func collectDiskTemperaturesFromSMC(values map[string]float64) (map[string]float64, error) {
	sums := make(map[string]float64)
	counts := make(map[string]float64)

	for key, temp := range values {
		if temp <= 0 || temp >= 150 {
			continue
		}

		device, ok := diskDeviceFromSMCKey(key)
		if !ok {
			continue
		}
		sums[device] += temp
		counts[device]++
	}

	if len(sums) == 0 {
		return nil, fmt.Errorf("no valid disk temperatures found in SMC values")
	}

	out := make(map[string]float64, len(sums))
	for device, sum := range sums {
		out[device] = sum / counts[device]
	}
	return out, nil
}

func diskDeviceFromSMCKey(key string) (string, bool) {
	key = strings.TrimSpace(key)
	m := smcDiskKeyRe.FindStringSubmatch(key)
	if len(m) != 2 {
		return "", false
	}
	return "disk" + m[1], true
}

// queryThermalState returns the current macOS thermal pressure state.
// It uses notifyutil to read the com.apple.system.thermalstate notification,
// which reflects NSProcessInfo.thermalState (public API, no root required).
//
// Reference:
// - https://developer.apple.com/documentation/foundation/processinfo/thermalstate
// - https://developer.apple.com/documentation/foundation/processinfo/1412637-thermalstate
// Fallback: sysctl kern.thermal_level and machdep.xcpm.thermal_level for
// older macOS versions (pre-Tahoe) where the sysctl keys still exist.
func queryThermalState() (string, error) {
	// Primary: notifyutil reads NSProcessInfo thermal state notification.
	// Works on all macOS versions including 26.x (Tahoe) without root.
	if state, err := notifyutilThermal(); err == nil {
		return state, nil
	}

	// Fallback for older macOS (Intel, pre-Tahoe): machdep.xcpm.thermal_level.
	if state, err := sysctlThermal("machdep.xcpm.thermal_level"); err == nil {
		return state, nil
	}

	// Fallback for older Apple Silicon macOS: kern.thermal_level.
	if state, err := sysctlThermal("kern.thermal_level"); err == nil {
		return state, nil
	}

	return "", fmt.Errorf("no supported thermal query method found")
}

// notifyutilThermal reads the thermal state via notifyutil.
// Output format: "com.apple.system.thermalstate <level>" where level is 0-3.
func notifyutilThermal() (string, error) {
	out, err := runCommand("notifyutil", "-g", "com.apple.system.thermalstate")
	if err != nil {
		return "", fmt.Errorf("notifyutil -g com.apple.system.thermalstate: %w", err)
	}

	// Output: "com.apple.system.thermalstate 0\n"
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) < 2 {
		return "", fmt.Errorf("unexpected notifyutil output: %q", string(out))
	}

	level, err := strconv.Atoi(fields[len(fields)-1])
	if err != nil {
		return "", fmt.Errorf("parsing notifyutil output %q: %w", string(out), err)
	}

	return thermalLevelToState(level), nil
}

// sysctlThermal reads a thermal level sysctl key and maps it to a state string.
func sysctlThermal(key string) (string, error) {
	out, err := runCommand("sysctl", "-n", key)
	if err != nil {
		return "", fmt.Errorf("sysctl %s: %w", key, err)
	}

	valStr := strings.TrimSpace(string(out))
	level, err := strconv.Atoi(valStr)
	if err != nil {
		return "", fmt.Errorf("parsing sysctl %s output %q: %w", key, valStr, err)
	}

	return thermalLevelToState(level), nil
}

// thermalLevelToState converts a numeric thermal level to a state label.
// Levels above 3 map to "critical" as a safe default.
func thermalLevelToState(level int) string {
	switch {
	case level <= 0:
		return thermalNominal
	case level == 1:
		return thermalFair
	case level == 2:
		return thermalSerious
	default:
		return thermalCritical
	}
}
