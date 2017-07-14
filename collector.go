package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/xid"
)

func NewCollector() *teamcityCollector {
	return &teamcityCollector{
		startTime: prometheus.NewDesc(xid.New().String(), "collector ID", nil, nil),
	}
}

func (col *teamcityCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- col.startTime
}

func (col *teamcityCollector) Collect(ch chan<- prometheus.Metric) {
	for i := range metricsStorage.IterBuffered() {
		ch <- i.Val.(prometheus.Metric)
	}
	// desc := prometheus.NewDesc("lala",
	// 													 "lala",
	// 													 []string{"buildConfiguration", "name"},
	// 													 nil)
	// ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, float64(1), "111", "222")
}
