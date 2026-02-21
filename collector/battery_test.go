//go:build darwin

package collector

import (
	"testing"
)

func TestParseIoregBattery_CycleCount(t *testing.T) {
	input := `    "CycleCount" = 42
    "MaxCapacity" = 8000
    "DesignCapacity" = 10000
    "Voltage" = 12000
    "InstantAmperage" = 18446744073709550616
    "Temperature" = 2935`

	info := &BatteryInfo{}
	parseIoregBattery([]byte(input), info)

	if info.CycleCount != 42 {
		t.Errorf("expected CycleCount=42, got %v", info.CycleCount)
	}
	// HealthPercent = 8000/10000 * 100 = 80
	if info.HealthPercent != 80 {
		t.Errorf("expected HealthPercent=80, got %v", info.HealthPercent)
	}
	// VoltageVolts = 12000/1000 = 12
	if info.VoltageVolts != 12 {
		t.Errorf("expected VoltageVolts=12, got %v", info.VoltageVolts)
	}
	// Temperature = 2935/100 = 29.35
	if info.TempCelsius != 29.35 {
		t.Errorf("expected TempCelsius=29.35, got %v", info.TempCelsius)
	}
}

func TestParseIoregBattery_NoKeys(t *testing.T) {
	input := `This output has no matching keys at all`
	info := &BatteryInfo{}
	parseIoregBattery([]byte(input), info)
	if info.CycleCount != 0 || info.HealthPercent != 0 || info.VoltageVolts != 0 || info.TempCelsius != 0 {
		t.Errorf("expected all zero fields for input with no keys, got %+v", info)
	}
}

func TestParseIoregBattery_NonNumericValues(t *testing.T) {
	input := `    "CycleCount" = not-a-number
    "Voltage" = also-bad`
	info := &BatteryInfo{}
	// Must not panic.
	parseIoregBattery([]byte(input), info)
	if info.CycleCount != 0 {
		t.Errorf("expected CycleCount=0 for non-numeric value, got %v", info.CycleCount)
	}
	if info.VoltageVolts != 0 {
		t.Errorf("expected VoltageVolts=0 for non-numeric value, got %v", info.VoltageVolts)
	}
}
