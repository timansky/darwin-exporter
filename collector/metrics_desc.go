//go:build darwin

package collector

import "github.com/prometheus/client_golang/prometheus"

func newDesc(ns, sub, name, help string, labels ...string) *prometheus.Desc {
	return prometheus.NewDesc(prometheus.BuildFQName(ns, sub, name), help, labels, nil)
}
