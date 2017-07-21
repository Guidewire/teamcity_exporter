package main

import (
	tc "github.com/guidewire/teamcity-go-bindings"
	"github.com/prometheus/client_golang/prometheus"
	"time"
)

type Instance struct {
	Name           string        `yaml:"name"`
	URL            string        `yaml:"url"`
	Username       string        `yaml:"username"`
	Password       string        `yaml:"password"`
	ScrapeInterval int64         `yaml:"scrape_interval"`
	BuildsFilters  []BuildFilter `yaml:"builds_filters"`
}

type BuildFilter struct {
	Name            string          `yaml:"name"`
	Filter          tc.BuildLocator `yaml:"filter"`
	startProcessing time.Time
	instance        string
}

type Configuration struct {
	Instances []Instance `yaml:"instances"`
}

type Collector struct {
	startupTime *prometheus.Desc
}

type Label struct {
	Name  string
	Value string
}

type ticker struct {
	C chan time.Time
}
