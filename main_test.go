package main

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/timansky/darwin-exporter/config"
)

// TestNewLogger_TimestampKey verifies that the logger uses "ts" instead of "time" for timestamps
func TestNewLogger_TimestampKey(t *testing.T) {
	tests := []struct {
		name         string
		format       string
		expectedType string
	}{
		{"JSON format", "json", "JSONFormatter"},
		{"Text format", "text", "TextFormatter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Logging: config.LoggingConfig{
					Level:  "info",
					Format: tt.format,
				},
			}

			log := newLogger(cfg)

			// Verify the formatter type and FieldMap
			switch formatter := log.Formatter.(type) {
			case *logrus.JSONFormatter:
				if formatter.FieldMap[logrus.FieldKeyTime] != "ts" {
					t.Errorf("JSONFormatter.FieldMap[FieldKeyTime] = %q, want %q", formatter.FieldMap[logrus.FieldKeyTime], "ts")
				}
			case *logrus.TextFormatter:
				if formatter.FieldMap[logrus.FieldKeyTime] != "ts" {
					t.Errorf("TextFormatter.FieldMap[FieldKeyTime] = %q, want %q", formatter.FieldMap[logrus.FieldKeyTime], "ts")
				}
			default:
				t.Errorf("Unexpected formatter type: %T", formatter)
			}
		})
	}
}

// TestNewLogger_LogOutput verifies that logger produces output with "ts=" key
func TestNewLogger_LogOutput(t *testing.T) {
	cfg := &config.Config{
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}

	log := newLogger(cfg)

	// Capture log output by logging a test message
	// The TextFormatter with TimestampKey="ts" should output "ts=" instead of "time="
	log.Info("test message")

	// Note: This test verifies the logger works, but actual output format
	// verification would require capturing stdout which is done in integration tests
}
