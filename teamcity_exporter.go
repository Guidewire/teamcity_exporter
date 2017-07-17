package main

import (
	"flag"
	"fmt"
	tc "github.com/guidewire/teamcity-go-bindings"
	"github.com/orcaman/concurrent-map"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
	"github.com/sirupsen/logrus"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	namespace = "teamcity"
)

var metricsStorage = cmap.New()

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
		configPath    = flag.String("config", "config.yaml", "Path to configuration file")
	)
	flag.Parse()

	config := Configuration{}
	err := config.parseConfig(*configPath)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"config": *configPath,
		}).Error(err)
		os.Exit(1)
	}

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

	ticker := newTicker(time.Duration(i.ScrapeInterval) * time.Second)
	for _ = range ticker.C {
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
					f := i.BuildsFilters[v]
					f.Filter.BuildType = buildCfgs.BuildType[z].ID
					logrus.WithFields(logsFormatter(f)).WithFields(logrus.Fields{
						"instance": i.Name,
					}).Debug("Found build configuration")
					wg.Add(1)
					go collectBuildsStat(client, i.Name, f, wg)
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
		metric := strings.SplitN(stat.Property[i].Name, ":", 2)
		title := toSnakeCase(metric[0])

		labels := []Label{
			{"exporter_instance", inst},
			{"exporter_filter", filter.Name},
			{"build_configuration", stat.UsedFilter.BuildType},
		}
		if len(metric) > 1 {
			labels = append(labels, Label{"other", metric[1]})
		}

		labelsTitles, labelsValues := []string{}, []string{}
		for i := range labels {
			labelsTitles = append(labelsTitles, labels[i].Name)
			labelsValues = append(labelsValues, labels[i].Value)
		}

		desc := prometheus.NewDesc(title, title, labelsTitles, nil)
		metricsStorage.Set(getHash(title, labelsValues...), prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, value, labelsValues...))
		logrus.WithFields(logrus.Fields{
			"name":  title,
			"value": value,
		}).Debug("Saving metric to temporary storage")
	}
}
