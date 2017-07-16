package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/xid"
)

func NewCollector() *Collector {
	return &Collector{
		startTime: prometheus.NewDesc(xid.New().String(), "collector ID", nil, nil),
	}
}

func (col *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- col.startTime
}

func (col *Collector) Collect(ch chan<- prometheus.Metric) {
	for i := range metricsStorage.IterBuffered() {
		ch <- i.Val.(prometheus.Metric)
	}
}
