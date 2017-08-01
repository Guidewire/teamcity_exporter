package main

import (
	"time"

	tc "github.com/guidewire/teamcity-go-bindings"
	"github.com/prometheus/client_golang/prometheus"
)

type Instance struct {
	Name             string        `yaml:"name"`
	URL              string        `yaml:"url"`
	Username         string        `yaml:"username"`
	Password         string        `yaml:"password"`
	ScrapeInterval   int           `yaml:"scrape_interval"`
	ConcurrencyLimit int           `yaml:"concurrency_limit"`
	BuildsFilters    []BuildFilter `yaml:"builds_filters"`
}

type BuildFilter struct {
	Name     string          `yaml:"name"`
	Filter   tc.BuildLocator `yaml:"filter"`
	instance string
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

type Ticker struct {
	c chan time.Time
}

type BuildStatistics struct {
	Build Build
	Stat  tc.BuildStatistics
}

type Build struct {
	Details tc.Build
	Filter  BuildFilter
}
