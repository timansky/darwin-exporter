package collector

import (
	"fmt"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/sirupsen/logrus"
)

// mockCollector is a test double for the Collector interface.
type mockCollector struct {
	metrics []prometheus.Metric
	err     error
}

func (m *mockCollector) Update(ch chan<- prometheus.Metric) error {
	for _, metric := range m.metrics {
		ch <- metric
	}
	return m.err
}

// panicCollector is a test double that panics in Update.
type panicCollector struct {
	panicVal interface{}
}

func (p *panicCollector) Update(_ chan<- prometheus.Metric) error {
	panic(p.panicVal)
}

// collectMetrics runs Collect and returns all collected metrics.
func collectMetrics(r *Registry) []prometheus.Metric {
	ch := make(chan prometheus.Metric, 50)
	r.Collect(ch)
	close(ch)
	var out []prometheus.Metric
	for m := range ch {
		out = append(out, m)
	}
	return out
}

// descContains checks whether the metric's Desc string includes substr.
func descContains(m prometheus.Metric, substr string) bool {
	return strings.Contains(m.Desc().String(), substr)
}

func metricGaugeValue(m prometheus.Metric) (float64, bool) {
	var pm dto.Metric
	if err := m.Write(&pm); err != nil || pm.GetGauge() == nil {
		return 0, false
	}
	return pm.GetGauge().GetValue(), true
}

func metricLabelValue(m prometheus.Metric, label string) (string, bool) {
	var pm dto.Metric
	if err := m.Write(&pm); err != nil {
		return "", false
	}
	for _, lp := range pm.GetLabel() {
		if lp.GetName() == label {
			return lp.GetValue(), true
		}
	}
	return "", false
}

func newTestRegistry() *Registry {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	return NewRegistry(log)
}

func TestRegistry_Register(t *testing.T) {
	r := newTestRegistry()
	r.Register("test", &mockCollector{})
	names := r.Names()
	if len(names) != 1 || names[0] != "test" {
		t.Errorf("expected [test], got %v", names)
	}
}

func TestRegistry_Collect_NoError(t *testing.T) {
	r := newTestRegistry()
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "darwin_test_metric",
		Help: "Test metric.",
	})
	gauge.Set(42)
	r.Register("test", &mockCollector{metrics: []prometheus.Metric{gauge}})

	ch := make(chan prometheus.Metric, 10)
	r.Collect(ch)
	close(ch)

	var collected []prometheus.Metric
	for m := range ch {
		collected = append(collected, m)
	}
	if len(collected) != 2 {
		t.Errorf("expected 2 metrics (collector + collector_up), got %d", len(collected))
	}
}

func TestRegistry_Collect_WithError(t *testing.T) {
	r := newTestRegistry()
	r.Register("failing", &mockCollector{err: fmt.Errorf("test error")})

	ch := make(chan prometheus.Metric, 10)
	r.Collect(ch)
	close(ch)

	var collected []prometheus.Metric
	for m := range ch {
		collected = append(collected, m)
	}
	// Should receive error metric + collector_up=0.
	if len(collected) != 2 {
		t.Errorf("expected 2 metrics (error + collector_up), got %d", len(collected))
	}
}

func TestRegistry_Describe(t *testing.T) {
	r := newTestRegistry()
	ch := make(chan *prometheus.Desc, 10)
	// Should not block or panic for unchecked collector.
	r.Describe(ch)
	close(ch)
}

func TestRegistry_MultipleCollectors(t *testing.T) {
	r := newTestRegistry()
	g1 := prometheus.NewGauge(prometheus.GaugeOpts{Name: "darwin_a", Help: "A"})
	g2 := prometheus.NewGauge(prometheus.GaugeOpts{Name: "darwin_b", Help: "B"})
	g1.Set(1)
	g2.Set(2)

	r.Register("coll_a", &mockCollector{metrics: []prometheus.Metric{g1}})
	r.Register("coll_b", &mockCollector{metrics: []prometheus.Metric{g2}})

	ch := make(chan prometheus.Metric, 10)
	r.Collect(ch)
	close(ch)

	count := 0
	for range ch {
		count++
	}
	if count != 4 {
		t.Errorf("expected 4 metrics (2 collector + 2 collector_up), got %d", count)
	}
}

// TestRegistry_Collect_PanicRecovery verifies that a panicking collector does
// not crash the process and produces an error metric.
func TestRegistry_Collect_PanicRecovery(t *testing.T) {
	r := newTestRegistry()
	r.Register("panicky", &panicCollector{panicVal: "something went wrong"})

	// Must not panic.
	metrics := collectMetrics(r)

	if len(metrics) == 0 {
		t.Fatal("expected at least one error metric from panicking collector, got none")
	}
	// The error metric should reference darwin_collector_error.
	found := false
	for _, m := range metrics {
		if descContains(m, "darwin_collector_error") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected darwin_collector_error metric from panicking collector")
	}
}

// TestRegistry_Collect_PanicRecovery_NilPointer verifies that a nil pointer
// dereference panic is caught and recorded.
func TestRegistry_Collect_PanicRecovery_NilPointer(t *testing.T) {
	r := newTestRegistry()
	r.Register("nil_panic", &panicCollector{panicVal: (*int)(nil)})

	// Must not panic.
	metrics := collectMetrics(r)

	if len(metrics) == 0 {
		t.Fatal("expected error metric from nil-pointer panicking collector")
	}
}

// TestRegistry_Collect_PanicRecovery_OtherCollectorsUnaffected verifies that
// a panic in one collector does not prevent other collectors from running.
func TestRegistry_Collect_PanicRecovery_OtherCollectorsUnaffected(t *testing.T) {
	r := newTestRegistry()
	good := prometheus.NewGauge(prometheus.GaugeOpts{Name: "darwin_good", Help: "good"})
	good.Set(99)
	r.Register("good", &mockCollector{metrics: []prometheus.Metric{good}})
	r.Register("panicky", &panicCollector{panicVal: "boom"})

	metrics := collectMetrics(r)

	// We expect at least 2: one real metric + one error metric.
	if len(metrics) < 2 {
		t.Errorf("expected >=2 metrics (real + error), got %d", len(metrics))
	}
}

// TestRegistry_Close_Closeable verifies that Close() is called on Closeable collectors.
func TestRegistry_Close_Closeable(t *testing.T) {
	r := newTestRegistry()
	mc := &closeableCollector{}
	r.Register("closeable", mc)
	r.Close()
	if !mc.closed {
		t.Error("expected Close() to be called on closeable collector")
	}
}

// TestRegistry_Close_NonCloseable verifies that Close() is a no-op for non-Closeable collectors.
func TestRegistry_Close_NonCloseable(t *testing.T) {
	r := newTestRegistry()
	mc := &mockCollector{}
	r.Register("plain", mc)
	// Should not panic.
	r.Close()
}

// closeableCollector is a test double that tracks Close() calls.
type closeableCollector struct {
	mockCollector
	closed bool
}

func (c *closeableCollector) Close() error {
	c.closed = true
	return nil
}

// TestRegistry_Collect_ErrorMetricOnUpdate verifies that returning an error from
// Update emits a darwin_collector_error metric.
func TestRegistry_Collect_ErrorMetricOnUpdate(t *testing.T) {
	r := newTestRegistry()
	r.Register("failing", &mockCollector{err: fmt.Errorf("collector exploded")})

	metrics := collectMetrics(r)

	found := false
	for _, m := range metrics {
		if descContains(m, "darwin_collector_error") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected darwin_collector_error metric when collector returns error")
	}
}

func TestRegistry_Collect_CollectorUpMetric(t *testing.T) {
	r := newTestRegistry()
	g := prometheus.NewGauge(prometheus.GaugeOpts{Name: "darwin_ok", Help: "ok"})
	g.Set(1)
	r.Register("ok", &mockCollector{metrics: []prometheus.Metric{g}})
	r.Register("bad", &mockCollector{err: fmt.Errorf("boom")})

	metrics := collectMetrics(r)

	got := map[string]float64{}
	for _, m := range metrics {
		if !descContains(m, "darwin_collector_up") {
			continue
		}
		collectorName, ok := metricLabelValue(m, "collector")
		if !ok {
			t.Fatal("collector_up metric missing collector label")
		}
		v, ok := metricGaugeValue(m)
		if !ok {
			t.Fatal("collector_up metric is not a gauge")
		}
		got[collectorName] = v
	}

	if got["ok"] != 1 {
		t.Fatalf("expected collector_up=1 for ok collector, got %v", got["ok"])
	}
	if got["bad"] != 0 {
		t.Fatalf("expected collector_up=0 for bad collector, got %v", got["bad"])
	}
}
