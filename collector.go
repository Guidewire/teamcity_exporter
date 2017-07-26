package main

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func NewCollector() *Collector {
	col := &Collector{
		startupTime: prometheus.NewDesc(namespace+"_collector_startup_time", "Collector startup time", nil, nil),
	}
	metricsStorage.Set(getHash(col.startupTime.String()), prometheus.MustNewConstMetric(col.startupTime, prometheus.GaugeValue, float64(time.Now().Unix())))
	return col
}

func (col *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- col.startupTime
}

func (col *Collector) Collect(ch chan<- prometheus.Metric) {
	for i := range metricsStorage.IterBuffered() {
		ch <- i.Val.(prometheus.Metric)
	}
}
