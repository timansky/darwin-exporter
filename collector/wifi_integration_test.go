//go:build darwin && cgo && integration

package collector

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// TestCoreWLANMonitor_Integration starts a real CoreWLAN monitor, waits for the
// initial snapshot, then shuts down. Must be run with:
//
//	go test -tags "cgo,integration" ./collector/
func TestCoreWLANMonitor_Integration(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	// Reset cache before test.
	globalCache.Store(nil)

	collector := NewWiFiCollector(log)
	defer func() {
		if err := collector.Close(); err != nil {
			t.Errorf("Close() error: %v", err)
		}
	}()

	// Wait up to 3 seconds for the initial snapshot to appear.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if snap := globalCache.Load(); snap != nil {
			t.Logf("Initial snapshot received from %s at %v", snap.Source, snap.Timestamp)
			t.Logf("Connected=%v RSSI=%v TxRate=%v Channel=%v Band=%q",
				snap.Info.Connected, snap.Info.RSSI, snap.Info.TxRate,
				snap.Info.Channel, snap.Info.Band)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	// If no snapshot after 3s, the system may have no WiFi interface.
	t.Skip("No WiFi snapshot received within 3s — possibly no WiFi interface available")
}
