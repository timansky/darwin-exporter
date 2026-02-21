//go:build darwin

package collector

import (
	"errors"
	"io"
	"math"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

func TestThermalLevelToState(t *testing.T) {
	cases := []struct {
		level    int
		expected string
	}{
		{0, thermalNominal},
		{-1, thermalNominal},
		{1, thermalFair},
		{2, thermalSerious},
		{3, thermalCritical},
		{99, thermalCritical},
	}
	for _, tc := range cases {
		got := thermalLevelToState(tc.level)
		if got != tc.expected {
			t.Errorf("thermalLevelToState(%d) = %q, want %q", tc.level, got, tc.expected)
		}
	}
}

func TestThermalStates_Count(t *testing.T) {
	// Ensure all states are defined.
	if len(thermalStates) != 4 {
		t.Errorf("expected 4 thermal states, got %d", len(thermalStates))
	}
}

func TestThermalStates_ContainsAll(t *testing.T) {
	expected := []string{thermalNominal, thermalFair, thermalSerious, thermalCritical}
	for _, e := range expected {
		found := false
		for _, s := range thermalStates {
			if s == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("thermalStates does not contain %q", e)
		}
	}
}

// TestNotifyutilThermal_Integration verifies notifyutil can be invoked and returns
// a valid thermal state. This is a regression test for BUG-2: the sysctl keys
// machdep.xcpm.thermal_level and kern.thermal_level were removed in macOS 26.x (Tahoe).
func TestNotifyutilThermal_Integration(t *testing.T) {
	state, err := notifyutilThermal()
	if err != nil {
		t.Fatalf("notifyutilThermal() error: %v", err)
	}
	valid := map[string]bool{
		thermalNominal:  true,
		thermalFair:     true,
		thermalSerious:  true,
		thermalCritical: true,
	}
	if !valid[state] {
		t.Errorf("notifyutilThermal() returned unexpected state %q", state)
	}
}

func TestParsePowermetricsCPUTemperature_Celsius(t *testing.T) {
	in := []byte(`
*** Sampled system activity ***
CPU die temperature: 59.38 C
`)
	got, err := parsePowermetricsCPUTemperature(in)
	if err != nil {
		t.Fatalf("parsePowermetricsCPUTemperature() error: %v", err)
	}
	if math.Abs(got-59.38) > 0.001 {
		t.Fatalf("expected 59.38, got %v", got)
	}
}

func TestParsePowermetricsCPUTemperature_Fahrenheit(t *testing.T) {
	in := []byte(`CPU temperature: 140 F`)
	got, err := parsePowermetricsCPUTemperature(in)
	if err != nil {
		t.Fatalf("parsePowermetricsCPUTemperature() error: %v", err)
	}
	if math.Abs(got-60.0) > 0.001 {
		t.Fatalf("expected 60.0C, got %v", got)
	}
}

func TestParsePowermetricsCPUTemperature_NoData(t *testing.T) {
	if _, err := parsePowermetricsCPUTemperature([]byte("cpu_power sampler output")); err == nil {
		t.Fatal("expected error when cpu temperature line is missing")
	}
}

func TestAverageSMCTemperature(t *testing.T) {
	got, ok := averageSMCTemperature(map[string]float64{
		"Tp01": 40.0,
		"Tp05": 42.0,
		"Tp09": 0.0,  // ignored
		"Tp0D": 200., // ignored
	}, 10, 120)
	if !ok {
		t.Fatal("expected valid average for SMC CPU temperatures")
	}
	if math.Abs(got-41.0) > 0.001 {
		t.Fatalf("expected average 41.0, got %v", got)
	}
}

func TestAverageSMCTemperature_NoValidData(t *testing.T) {
	if _, ok := averageSMCTemperature(map[string]float64{
		"Tp01": 5.0,
		"Tp05": 500.0,
	}, 10, 120); ok {
		t.Fatal("expected no valid SMC temperature values")
	}
}

func TestCollectDiskTemperaturesFromSMC(t *testing.T) {
	got, err := collectDiskTemperaturesFromSMC(map[string]float64{
		"TH0a": 32.0,
		"TH0b": 34.0,
		"TH0x": 36.0,
		"TH1A": 30.0,
		"Tp01": 55.0, // ignored (not disk key)
	})
	if err != nil {
		t.Fatalf("collectDiskTemperaturesFromSMC() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 disk metrics, got %d", len(got))
	}
	if math.Abs(got["disk0"]-34.0) > 0.001 {
		t.Fatalf("expected disk0 average 34.0, got %v", got["disk0"])
	}
	if math.Abs(got["disk1"]-30.0) > 0.001 {
		t.Fatalf("expected disk1 30.0, got %v", got["disk1"])
	}
}

func TestCollectDiskTemperaturesFromSMC_NoData(t *testing.T) {
	if _, err := collectDiskTemperaturesFromSMC(map[string]float64{"Tp01": 60.0}); err == nil {
		t.Fatal("expected error when no disk sensors are present")
	}
}

func TestDiskDeviceFromSMCKey(t *testing.T) {
	cases := []struct {
		key    string
		device string
		ok     bool
	}{
		{key: "TH0x", device: "disk0", ok: true},
		{key: "th1a", device: "disk1", ok: true},
		{key: "TN2n", device: "disk2", ok: true},
		{key: "TP01", ok: false},
	}

	for _, tc := range cases {
		device, ok := diskDeviceFromSMCKey(tc.key)
		if ok != tc.ok {
			t.Fatalf("diskDeviceFromSMCKey(%q) ok=%v, want %v", tc.key, ok, tc.ok)
		}
		if ok && device != tc.device {
			t.Fatalf("diskDeviceFromSMCKey(%q) device=%q, want %q", tc.key, device, tc.device)
		}
	}
}

func TestSMCGPUDiscoverKeyRegex(t *testing.T) {
	cases := []struct {
		key   string
		match bool
	}{
		{key: "TG0D", match: true},
		{key: "Tg0p", match: true},
		{key: "Tp01", match: false},
		{key: "TH0a", match: false},
	}

	for _, tc := range cases {
		got := smcGPUDiscoverKeyRe.MatchString(tc.key)
		if got != tc.match {
			t.Fatalf("smcGPUDiscoverKeyRe.MatchString(%q)=%v, want %v", tc.key, got, tc.match)
		}
	}
}

type stubSMCProvider struct {
	discoverCPU  []string
	discoverGPU  []string
	discoverDisk []string
	discoverErr  error

	readValues map[string]float64
	readErr    error
	readCalls  int
}

func (s *stubSMCProvider) DiscoverKeySets() ([]string, []string, []string, error) {
	return append([]string{}, s.discoverCPU...),
		append([]string{}, s.discoverGPU...),
		append([]string{}, s.discoverDisk...),
		s.discoverErr
}

func (s *stubSMCProvider) ReadValues(keys []string) (map[string]float64, error) {
	s.readCalls++
	if s.readErr != nil {
		return nil, s.readErr
	}
	out := make(map[string]float64, len(s.readValues))
	for k, v := range s.readValues {
		out[k] = v
	}
	return out, nil
}

type stubPowermetricsProvider struct {
	available bool
	temp      float64
	err       error
	calls     int
}

func (p *stubPowermetricsProvider) Available() bool {
	return p.available
}

func (p *stubPowermetricsProvider) ReadCPUTemperature() (float64, error) {
	p.calls++
	if p.err != nil {
		return 0, p.err
	}
	return p.temp, nil
}

type stubThermalStateProvider struct {
	state string
	err   error
	calls int
}

func (s *stubThermalStateProvider) CurrentState() (string, error) {
	s.calls++
	if s.err != nil {
		return "", s.err
	}
	return s.state, nil
}

func newThermalTestLogger() *logrus.Logger {
	log := logrus.New()
	log.SetOutput(io.Discard)
	log.SetLevel(logrus.DebugLevel)
	return log
}

func TestThermalCollectorQueryCPUTemperature_UsesSMCBeforePowermetrics(t *testing.T) {
	smc := &stubSMCProvider{
		discoverCPU: []string{"Tp01", "Tp05"},
		readValues: map[string]float64{
			"Tp01": 40.0,
			"Tp05": 44.0,
		},
	}
	pm := &stubPowermetricsProvider{available: true, temp: 90.0}
	state := &stubThermalStateProvider{state: thermalNominal}

	c := NewThermalCollectorWithProviders(newThermalTestLogger(), smc, pm, state)
	got, err := c.queryCPUTemperature()
	if err != nil {
		t.Fatalf("queryCPUTemperature() error: %v", err)
	}
	if math.Abs(got-42.0) > 0.001 {
		t.Fatalf("expected 42.0 from SMC average, got %v", got)
	}
	if pm.calls != 0 {
		t.Fatalf("powermetrics calls=%d, want 0 when SMC succeeds", pm.calls)
	}
	if smc.readCalls != 1 {
		t.Fatalf("SMC read calls=%d, want 1", smc.readCalls)
	}
}

func TestThermalCollectorQueryCPUTemperature_FallsBackToPowermetrics(t *testing.T) {
	smc := &stubSMCProvider{
		discoverCPU: []string{"Tp01"},
		readValues: map[string]float64{
			"Tp01": 5.0, // filtered out by min threshold
		},
	}
	pm := &stubPowermetricsProvider{available: true, temp: 63.5}
	state := &stubThermalStateProvider{state: thermalNominal}

	c := NewThermalCollectorWithProviders(newThermalTestLogger(), smc, pm, state)
	got, err := c.queryCPUTemperature()
	if err != nil {
		t.Fatalf("queryCPUTemperature() fallback error: %v", err)
	}
	if math.Abs(got-63.5) > 0.001 {
		t.Fatalf("expected 63.5 from powermetrics fallback, got %v", got)
	}
	if pm.calls != 1 {
		t.Fatalf("powermetrics calls=%d, want 1", pm.calls)
	}
}

func TestThermalCollectorQueryCPUTemperature_ErrorContainsBothPaths(t *testing.T) {
	smc := &stubSMCProvider{
		discoverCPU: []string{"Tp01"},
		readErr:     errors.New("smc read failed"),
	}
	pm := &stubPowermetricsProvider{available: false}
	state := &stubThermalStateProvider{state: thermalNominal}

	c := NewThermalCollectorWithProviders(newThermalTestLogger(), smc, pm, state)
	_, err := c.queryCPUTemperature()
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "smc read failed") {
		t.Fatalf("error %q must contain SMC failure context", msg)
	}
	if !strings.Contains(msg, "powermetrics is not accessible") {
		t.Fatalf("error %q must contain powermetrics availability context", msg)
	}
}

func TestThermalCollectorUpdate_UsesInjectedStateProvider(t *testing.T) {
	smc := &stubSMCProvider{}
	pm := &stubPowermetricsProvider{available: false}
	state := &stubThermalStateProvider{state: thermalFair}

	c := NewThermalCollectorWithProviders(newThermalTestLogger(), smc, pm, state)
	ch := make(chan prometheus.Metric, 16)
	if err := c.Update(ch); err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if state.calls != 1 {
		t.Fatalf("state provider calls=%d, want 1", state.calls)
	}
	if got := len(ch); got != len(thermalStates) {
		t.Fatalf("expected %d thermal pressure metrics, got %d", len(thermalStates), got)
	}
}
