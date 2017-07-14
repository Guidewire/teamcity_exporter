package main

import (
	"flag"
	"fmt"
	"github.com/Sirupsen/logrus"
	tc "github.com/guidewire/teamcity-go-bindings"
	"github.com/orcaman/concurrent-map"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"
)

const (
	namespace = "teamcity"
)

var (
	metricsStorage = cmap.New()
	config         = Configuration{
		Instances: []Instance{
			{"instance1",
				"https://teamcity",
				"login",
				"password",
				30, /* Scrape interval */
				[]BuildFilter{
					{Name: "name1",
						Filter: tc.BuildLocator{"", /* BuildType */
							"",        /* Branch */
							"success", /* Status */
							"false",   /* Running */
							"false",   /* Canceled */
							"1" /* Count */}},
				},
			},
			{"instance2",
				"https://teamcity2",
				"login",
				"password",
				30, /* Scrape interval */
				[]BuildFilter{
					{Name: "name2",
						Filter: tc.BuildLocator{"ca_generation_1", /* BuildType */
							"",        /* Branch */
							"success", /* Status */
							"false",   /* Running */
							"false",   /* Canceled */
							"1" /* Count */}},
					{Name: "name3",
						Filter: tc.BuildLocator{"ca_generation_2", /* BuildType */
							"",        /* Branch */
							"success", /* Status */
							"false",   /* Running */
							"false",   /* Canceled */
							"1" /* Count */}},
				},
			},
		},
	}
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	prometheus.MustRegister(version.NewCollector("teamcity_exporter"))
}

func main() {
	var (
		showVersion   = flag.Bool("version", false, "Print version information.")
		listenAddress = flag.String("web.listen-address", ":9107", "Address to listen on for web interface and telemetry.")
		metricsPath   = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
		logLevel      = flag.String("loglevel", "info", "Changes log level, available values: info/debug")
	)
	flag.Parse()

	switch {
	case *logLevel == "debug":
		logrus.SetLevel(logrus.DebugLevel)
		logrus.Debug("Debug logging was enabled")
	default:
		logrus.SetLevel(logrus.InfoLevel)
	}

	if *showVersion {
		fmt.Fprintln(os.Stdout, version.Print("teamcity_exporter"))
		os.Exit(0)
	}

	logrus.Info("Starting teamcity_exporter" + version.Info())
	logrus.Info("Build context", version.BuildContext())

	collector := NewCollector()
	prometheus.MustRegister(collector)

	for i := range config.Instances {
		logrus.WithFields(logrus.Fields{
			"instance":       config.Instances[i].Name,
			"scrapeInterval": config.Instances[i].ScrapeInterval,
		}).Debug("Found Teamcity instance, preparing for metrics collection")
		go collectInstancesStat(config.Instances[i])
	}

	http.Handle(*metricsPath, prometheus.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
					 <head><title>Teamcity Exporter</title></head>
					 <body>
					 <h1>Teamcity Exporter</h1>
					 <p><a href='` + *metricsPath + `'>Metrics</a></p>
					 </body>
					 </html>`))
	})
	log.Infoln("Listening on", *listenAddress)
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}

func collectInstancesStat(i Instance) {
	client := tc.New(i.URL, i.Username, i.Password)
	wg := &sync.WaitGroup{}

	ticker := time.NewTicker(time.Duration(i.ScrapeInterval) * time.Second).C
	for _ = range ticker {
		logrus.WithFields(logrus.Fields{
			"instance":    i.Name,
			"timestamp":   time.Now().Unix(),
			"storageSize": len(metricsStorage.Keys()),
		}).Debug("Got message from ticker, starting metrics collection")
		logrus.WithFields(logrus.Fields{
			"instance": i.Name,
		}).Debug(fmt.Sprintf("Found %d build filters for instance, looping against filters", len(i.BuildsFilters)))
		for v := range i.BuildsFilters {
			logrus.WithFields(logsFormatter(i.BuildsFilters[v].Filter)).WithFields(logrus.Fields{
				"instance": i.Name,
			}).Debug("Found build filter, preparing request for Teamcity")
			if i.BuildsFilters[v].Filter.BuildType != "" {
				wg.Add(1)
				go collectBuildsStat(client, i.Name, i.BuildsFilters[v], wg)
			} else {
				logrus.WithFields(logrus.Fields{
					"instance": i.Name,
				}).Debug("Filter has no build configuration specified, will run against all available configurations")
				buildCfgs, err := client.GetAllBuildConfigurations()
				if err != nil {
					logrus.WithFields(logrus.Fields{
						"instance": i.URL,
					}).Error(err)
				}
				for z := range buildCfgs.BuildType {
					i.BuildsFilters[v].Filter.BuildType = buildCfgs.BuildType[z].ID
					logrus.WithFields(logsFormatter(buildCfgs.BuildType[z])).WithFields(logrus.Fields{
						"instance": i.Name,
					}).Debug("Found build configuration")
					wg.Add(1)
					go collectBuildsStat(client, i.Name, i.BuildsFilters[v], wg)
				}
			}
		}
		wg.Wait()
		logrus.WithFields(logrus.Fields{
			"instance": i.Name,
		}).Debug("Scraping job finished, waiting for a signal from ticker")
	}
}

func collectBuildsStat(c *tc.Client, inst string, filter BuildFilter, wg *sync.WaitGroup) {
	defer wg.Done()

	stat, err := c.GetBuildStatistics(filter.Filter)
	if err != nil {
		logrus.WithFields(logsFormatter(filter.Filter)).WithFields(logrus.Fields{
			"instance": inst,
			"filter":   filter.Name,
		}).Error(err)
	}

	logrus.WithFields(logsFormatter(filter.Filter)).WithFields(logrus.Fields{
		"instance":        inst,
		"filter":          filter.Name,
		"metricsGathered": stat.Count,
	}).Debug("Gathered build metrics based on provided filter")

	for i := range stat.Property {
		value, _ := strconv.ParseFloat(stat.Property[i].Value, 64)
		metricWithLabels := splitMetricsTitle(stat.Property[i].Name)
		for m, l := range metricWithLabels {
			desc := prometheus.NewDesc(convertCamelCaseToSnakeCase(m),
				convertCamelCaseToSnakeCase(m),
				[]string{"instance", "filter", "buildConfiguration", "name"},
				nil)
			metricsStorage.Set(getHash(convertCamelCaseToSnakeCase(m)+inst+filter.Name+stat.UsedFilter.BuildType+l),
				prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, value, inst, filter.Name, stat.UsedFilter.BuildType, l))
			logrus.WithFields(logrus.Fields{
				"name":  convertCamelCaseToSnakeCase(m),
				"value": value,
			}).Debug("Saving metric to temporary storage")
		}
	}
}
