// Package collector defines the Collector interface and registry.
package collector

import (
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

// Collector is the interface that all darwin-exporter collectors must implement.
type Collector interface {
	// Update sends current metric values to the channel.
	Update(ch chan<- prometheus.Metric) error
}

// Closeable is implemented by collectors that hold background resources
// (goroutines, OS handles) and must be shut down gracefully.
type Closeable interface {
	Close() error
}

// Registry holds registered collectors and implements prometheus.Collector.
type Registry struct {
	mu         sync.Mutex
	collectors map[string]Collector
	log        *logrus.Logger
}

// NewRegistry creates an empty collector registry.
func NewRegistry(log *logrus.Logger) *Registry {
	return &Registry{
		collectors: make(map[string]Collector),
		log:        log,
	}
}

// Register adds a named collector to the registry.
func (r *Registry) Register(name string, c Collector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.collectors[name] = c
}

// Describe implements prometheus.Collector.
// darwin-exporter uses an unchecked collector pattern: metric descriptors are
// sent dynamically, so we emit a generic descriptor here.
func (r *Registry) Describe(ch chan<- *prometheus.Desc) {
	// Unchecked collector — no static descriptors.
}

// Collect implements prometheus.Collector. It calls Update on each registered
// collector and forwards metrics to the Prometheus channel.
func (r *Registry) Collect(ch chan<- prometheus.Metric) {
	r.mu.Lock()
	collectors := make(map[string]Collector, len(r.collectors))
	for k, v := range r.collectors {
		collectors[k] = v
	}
	r.mu.Unlock()

	var wg sync.WaitGroup
	for name, c := range collectors {
		wg.Add(1)
		go func(name string, c Collector) {
			defer wg.Done()
			defer func() {
				if rec := recover(); rec != nil {
					stack := debug.Stack()
					r.log.WithField("collector", name).
						WithField("panic", rec).
						Errorf("collector panicked; stack:\n%s", stack)
					ch <- prometheus.NewInvalidMetric(
						prometheus.NewDesc(
							"darwin_collector_error",
							"Indicates that the named collector returned an error.",
							[]string{"collector"},
							nil,
						),
						fmt.Errorf("collector %s panicked: %v", name, rec),
					)
				}
			}()
			if err := c.Update(ch); err != nil {
				r.log.WithField("collector", name).WithError(err).Warn("collector update failed")
				// Record scrape error as a metric.
				ch <- prometheus.NewInvalidMetric(
					prometheus.NewDesc(
						"darwin_collector_error",
						"Indicates that the named collector returned an error.",
						[]string{"collector"},
						nil,
					),
					fmt.Errorf("collector %s: %w", name, err),
				)
			}
		}(name, c)
	}
	wg.Wait()
}

// Names returns the list of registered collector names.
func (r *Registry) Names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, 0, len(r.collectors))
	for name := range r.collectors {
		names = append(names, name)
	}
	return names
}

// Close calls Close() on all registered collectors that implement Closeable.
// Errors are logged but do not stop processing of the remaining collectors.
func (r *Registry) Close() {
	r.mu.Lock()
	collectors := make(map[string]Collector, len(r.collectors))
	for k, v := range r.collectors {
		collectors[k] = v
	}
	r.mu.Unlock()

	for name, c := range collectors {
		if cl, ok := c.(Closeable); ok {
			if err := cl.Close(); err != nil {
				r.log.WithField("collector", name).WithError(err).Warn("collector close failed")
			}
		}
	}
}
