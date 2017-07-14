package main

import (
	tc "github.com/guidewire/teamcity-go-bindings"
	"github.com/prometheus/client_golang/prometheus"
)

type Instance struct {
	Name           string
	URL            string        `json:"url"`
	Username       string        `json:"username"`
	Password       string        `json:"password"`
	ScrapeInterval int64         `json:"scrape_interval"`
	BuildsFilters  []BuildFilter `json:"builds_filters"`
}

type BuildFilter struct {
	Name   string
	Filter tc.BuildLocator
}

type Configuration struct {
	Instances []Instance `json:"instances"`
}

type Exporter struct {
	client     *tc.Client
	instanceId string
}

type teamcityCollector struct {
	startTime *prometheus.Desc
}
