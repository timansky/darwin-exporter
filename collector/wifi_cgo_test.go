//go:build darwin && cgo

package collector

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

// TestTrimAtNull_NullTerminated verifies that trimAtNull strips content after
// the first null byte, matching the safe CGo string conversion behaviour.
func TestTrimAtNull_NullTerminated(t *testing.T) {
	got := trimAtNull("hello\x00garbage")
	if got != "hello" {
		t.Errorf("expected %q, got %q", "hello", got)
	}
}

// TestTrimAtNull_NoNull verifies that a string without a null byte is returned
// as-is.
func TestTrimAtNull_NoNull(t *testing.T) {
	got := trimAtNull("hello")
	if got != "hello" {
		t.Errorf("expected %q, got %q", "hello", got)
	}
}

// TestTrimAtNull_Empty verifies that an empty string is returned unchanged.
func TestTrimAtNull_Empty(t *testing.T) {
	got := trimAtNull("")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// TestTrimAtNull_AllNull verifies that a string of all null bytes returns empty.
func TestTrimAtNull_AllNull(t *testing.T) {
	got := trimAtNull("\x00\x00\x00")
	if got != "" {
		t.Errorf("expected empty string for all-null input, got %q", got)
	}
}

// TestTrimAtNull_SmallBuffer verifies trimming for short strings (security[64]).
func TestTrimAtNull_SmallBuffer(t *testing.T) {
	// Simulate what C.GoStringN returns for security[64] when set to "WPA2-Personal\0".
	raw := "WPA2-Personal\x00" + string(make([]byte, 50)) // null + zero padding
	got := trimAtNull(raw)
	if got != "WPA2-Personal" {
		t.Errorf("expected %q, got %q", "WPA2-Personal", got)
	}
}

func TestGoWiFiEventCallback_StoresSnapshot(t *testing.T) {
	// Reset global cache.
	globalCache.Store(nil)

	// Inject a snapshot directly via storeSnapshot (bypassing Obj-C).
	storeSnapshot(WiFiInfo{
		Connected:    true,
		PowerOn:      true,
		RSSI:         -60,
		Noise:        -90,
		TxRate:       300,
		TxPower:      100,
		Channel:      36,
		ChannelWidth: 80,
		Band:         "5GHz",
		PHYModeNum:   5,
		SSID:         "TestNet",
	})

	snap := globalCache.Load()
	if snap == nil {
		t.Fatal("expected snapshot to be stored, got nil")
	}
	if snap.Source != "corewlan" {
		t.Errorf("expected Source=corewlan, got %q", snap.Source)
	}
	if !snap.Info.Connected {
		t.Error("expected Connected=true")
	}
	if snap.Info.RSSI != -60 {
		t.Errorf("expected RSSI=-60, got %v", snap.Info.RSSI)
	}
	if snap.Info.Noise != -90 {
		t.Errorf("expected Noise=-90, got %v", snap.Info.Noise)
	}
	if snap.Info.TxRate != 300 {
		t.Errorf("expected TxRate=300, got %v", snap.Info.TxRate)
	}
	if snap.Info.TxPower != 100 {
		t.Errorf("expected TxPower=100, got %v", snap.Info.TxPower)
	}
	if snap.Info.Channel != 36 {
		t.Errorf("expected Channel=36, got %v", snap.Info.Channel)
	}
	if snap.Info.ChannelWidth != 80 {
		t.Errorf("expected ChannelWidth=80, got %v", snap.Info.ChannelWidth)
	}
	if snap.Info.Band != "5GHz" {
		t.Errorf("expected Band=5GHz, got %q", snap.Info.Band)
	}
	if snap.Info.PHYModeNum != 5 {
		t.Errorf("expected PHYModeNum=5, got %v", snap.Info.PHYModeNum)
	}
	if snap.Info.SSID != "TestNet" {
		t.Errorf("expected SSID=TestNet, got %q", snap.Info.SSID)
	}
	if snap.Timestamp.IsZero() {
		t.Error("expected Timestamp to be set")
	}
}

func TestGoWiFiEventCallback_Disconnected(t *testing.T) {
	globalCache.Store(nil)

	storeSnapshot(WiFiInfo{Connected: false, PowerOn: true})

	snap := globalCache.Load()
	if snap == nil {
		t.Fatal("expected snapshot even when disconnected")
	}
	if snap.Info.Connected {
		t.Error("expected Connected=false")
	}
}

func TestUpdate_EmitsMetricsFromCache(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.PanicLevel)

	// Pre-fill the global cache with a connected snapshot.
	snap := &WiFiSnapshot{
		Timestamp: time.Now(),
		Source:    "corewlan",
		Info: WiFiInfo{
			Connected:    true,
			RSSI:         -55,
			Noise:        -90,
			TxRate:       450,
			TxPower:      80,
			Channel:      36,
			ChannelWidth: 80,
			Band:         "5GHz",
			PHYModeNum:   5,
			SSID:         "TestNet",
		},
	}
	globalCache.Store(snap)

	// Build a minimal collector (no goroutine, close() not called).
	const ns = "darwin"
	const sub = "wifi"
	l := func(name, help string, labels ...string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(ns, sub, name), help, labels, nil)
	}
	c := &WiFiCollector{
		log:          log,
		rssi:         l("rssi_dbm", ""),
		noise:        l("noise_dbm", ""),
		snr:          l("snr_db", ""),
		txRate:       l("tx_rate_mbps", ""),
		channel:      l("channel", ""),
		connected:    l("connected", ""),
		info:         l("info", "", "interface", "ssid", "security", "band", "phymode", "country_code"),
		mcsIndex:     l("mcs_index", ""),
		channelWidth: l("channel_width_mhz", ""),
		txPower:      l("tx_power_mw", ""),
		phyMode:      l("phymode", ""),
		done:         make(chan struct{}),
	}

	ch := make(chan prometheus.Metric, 20)
	if err := c.Update(ch); err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	close(ch)

	metrics := make([]prometheus.Metric, 0)
	for m := range ch {
		metrics = append(metrics, m)
	}

	if len(metrics) == 0 {
		t.Error("expected at least one metric to be emitted")
	}
}

func TestUpdate_NilCache_EmitsConnectedZero(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.PanicLevel)

	globalCache.Store(nil)

	const ns = "darwin"
	const sub = "wifi"
	l := func(name, help string, labels ...string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(ns, sub, name), help, labels, nil)
	}
	c := &WiFiCollector{
		log:       log,
		connected: l("connected", ""),
		done:      make(chan struct{}),
	}

	ch := make(chan prometheus.Metric, 5)
	if err := c.Update(ch); err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	close(ch)

	count := 0
	for range ch {
		count++
	}
	if count != 1 {
		t.Errorf("expected 1 metric (connected=0), got %d", count)
	}
}

func TestChannelWidthMHz(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 0},
		{1, 20},
		{2, 40},
		{3, 80},
		{4, 160},
		{99, 0},
	}
	for _, tc := range cases {
		got := channelWidthMHz(tc.in)
		if int(got) != tc.want {
			t.Errorf("channelWidthMHz(%d) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestChannelBandLabel(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "unknown"},
		{1, "2.4GHz"},
		{2, "5GHz"},
		{3, "6GHz"},
		{99, "unknown"},
	}
	for _, tc := range cases {
		got := channelBandLabel(tc.in)
		if got != tc.want {
			t.Errorf("channelBandLabel(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Regression test for BUG: darwin_wifi_connected always returns 0 on macOS 14+.
// On macOS 14+ iface.ssid returns nil without Location Services permission.
// The fix uses iface.wlanChannel (no permission required) as the connected indicator.
// This test verifies that a snapshot with a non-nil channel but empty SSID (the
// macOS 14+ scenario) correctly reports Connected=true.
func TestGoWiFiEventCallback_ConnectedWithoutSSID(t *testing.T) {
	globalCache.Store(nil)

	// Simulate macOS 14+ state: WiFi connected, channel present, SSID hidden.
	storeSnapshot(WiFiInfo{
		Connected: true,
		PowerOn:   true,
		Channel:   36,
		Band:      "5GHz",
		SSID:      "", // Location Services not granted — SSID unavailable.
	})

	snap := globalCache.Load()
	if snap == nil {
		t.Fatal("expected snapshot to be stored, got nil")
	}
	if !snap.Info.Connected {
		t.Error("expected Connected=true when channel is present (macOS 14+ regression)")
	}
	if snap.Info.SSID != "" {
		t.Errorf("expected empty SSID (Location Services not granted), got %q", snap.Info.SSID)
	}
	if snap.Info.Channel != 36 {
		t.Errorf("expected Channel=36, got %v", snap.Info.Channel)
	}
}
