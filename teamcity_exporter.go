package main

import (
	"errors"
	"flag"
	"fmt"
	tc "github.com/guidewire/teamcity-go-bindings"
	"github.com/orcaman/concurrent-map"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
	"github.com/rs/xid"
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

var (
	instanceStatus = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "instance_status"),
		"Teamcity instance status",
		[]string{"instance"}, nil,
	)
	instanceLastScrapeFinishTime = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "instance_last_scrape_finish_time"),
		"Teamcity instance last scrape finish time",
		[]string{"instance"}, nil,
	)
	filterLastScrapeFinishTime = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "filter_last_scrape_finish_time"),
		"Teamcity instance filter last scrape finish time",
		[]string{"instance", "filter"}, nil,
	)
	instanceLastScrapeDuration = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "instance_last_scrape_duration"),
		"Teamcity instance last scrape duration",
		[]string{"instance"}, nil,
	)
	filterLastScrapeDuration = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "filter_last_scrape_duration"),
		"Teamcity instance filter last scrape duration",
		[]string{"instance", "filter"}, nil,
	)
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	prometheus.MustRegister(version.NewCollector("teamcity_exporter"))
}

func main() {
	var (
		showVersion   = flag.Bool("version", false, "Print version information")
		listenAddress = flag.String("web.listen-address", ":9107", "Address to listen on for web interface and telemetry")
		metricsPath   = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics")
		configPath    = flag.String("config", "config.yaml", "Path to configuration file")
	)
	flag.Parse()

	logrus.Info("Starting teamcity_exporter" + version.Info())
	logrus.Info("Build context", version.BuildContext())

	level, err := logrus.ParseLevel(strings.Replace(flag.Lookup("log.level").Value.String(), "\"", "", -1))
	if err != nil {
		logrus.Fatal(err)
	}
	logrus.SetLevel(level)
	logrus.Infof("Log level was set to %s", level.String())

	if *showVersion {
		fmt.Fprintln(os.Stdout, version.Print("teamcity_exporter"))
		return
	}

	collector := NewCollector()
	prometheus.MustRegister(collector)

	config := Configuration{}
	err = config.parseConfig(*configPath)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"config": *configPath,
		}).Fatal(err)
	}

	if err := config.validateConfig(); err != nil {
		logrus.WithFields(logrus.Fields{
			"config": *configPath,
		}).Fatal(err)
	}

	for i := range config.Instances {
		logrus.WithFields(logrus.Fields{
			"config":         *configPath,
			"instance":       config.Instances[i].Name,
			"scrapeInterval": config.Instances[i].ScrapeInterval,
			"filtersNumber":  len(config.Instances[i].BuildsFilters),
		}).Debug("Reading configuration, found teamcity instance")
		go config.Instances[i].collectInstancesStat()
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

func (i *Instance) validateStatus() error {
	client := &http.Client{}
	req, err := http.NewRequest("GET", i.URL, nil)
	if err != nil {
		metricsStorage.Set(getHash(instanceStatus.String(), i.Name), prometheus.MustNewConstMetric(instanceStatus, prometheus.GaugeValue, 0, i.Name))
		return err
	}
	req.SetBasicAuth(i.Username, i.Password)
	resp, err := client.Do(req)
	if err != nil {
		metricsStorage.Set(getHash(instanceStatus.String(), i.Name), prometheus.MustNewConstMetric(instanceStatus, prometheus.GaugeValue, 0, i.Name))
		return err
	}
	if resp.StatusCode == 401 {
		metricsStorage.Set(getHash(instanceStatus.String(), i.Name), prometheus.MustNewConstMetric(instanceStatus, prometheus.GaugeValue, 0, i.Name))
		return errors.New(fmt.Sprintf("Unauthorized %s", i.Name))
	}
	metricsStorage.Set(getHash(instanceStatus.String(), i.Name), prometheus.MustNewConstMetric(instanceStatus, prometheus.GaugeValue, 1, i.Name))
	return nil
}

func (i Instance) collectInstancesStat() {
	client := tc.New(i.URL, i.Username, i.Password)
	wg := &sync.WaitGroup{}

	ticker := newTicker(time.Duration(i.ScrapeInterval) * time.Second)
	for _ = range ticker.C {
		i.startProcessing = time.Now().Unix()

		routineID := xid.New().String()

		logrus.WithFields(logrus.Fields{
			"instance":          i.Name,
			"filtersNumber":     len(i.BuildsFilters),
			"timestamp":         i.startProcessing,
			"time":              time.Now(),
			"instanceRoutineID": routineID,
		}).Debug("Starting metrics collection")

		if err := i.validateStatus(); err != nil {
			logrus.WithFields(logrus.Fields{
				"instance":          i.Name,
				"instanceRoutineID": routineID,
			}).Error(err)
			continue
		}
		for v := range i.BuildsFilters {
			wg.Add(1)
			i.BuildsFilters[v].instance = i.Name
			go i.BuildsFilters[v].parseBuildsFilter(client, wg)
		}
		wg.Wait()
		metricsStorage.Set(getHash(instanceLastScrapeFinishTime.String(), i.Name), prometheus.MustNewConstMetric(instanceLastScrapeFinishTime, prometheus.GaugeValue, float64(time.Now().Unix()), i.Name))
		metricsStorage.Set(getHash(instanceLastScrapeDuration.String(), i.Name), prometheus.MustNewConstMetric(instanceLastScrapeDuration, prometheus.GaugeValue, float64(time.Now().Unix()-i.startProcessing), i.Name))
		logrus.WithFields(logrus.Fields{
			"instance":          i.Name,
			"instanceRoutineID": routineID,
			"storageSize":       metricsStorage.Count(),
			"timestamp":         time.Now().Unix(),
			"time":              time.Now(),
			"duration":          time.Now().Unix() - i.startProcessing,
		}).Debug("Successfully collected metrics for instance")
	}
}

func (f BuildFilter) parseBuildsFilter(c *tc.Client, wg *sync.WaitGroup) {
	defer wg.Done()

	f.startProcessing = time.Now().Unix()

	wgFilter := &sync.WaitGroup{}

	if f.Filter.BuildType != "" {
		wgFilter.Add(1)
		go f.collectBuildsStat(c, wgFilter)
	} else {
		buildCfgs, err := c.GetAllBuildConfigurations()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"instance": f.instance,
				"filter":   f.Name,
			}).Error(err)
		}
		for z := range buildCfgs.BuildType {
			f.Filter.BuildType = buildCfgs.BuildType[z].ID
			wgFilter.Add(1)
			go f.collectBuildsStat(c, wgFilter)
		}
	}
	wgFilter.Wait()
	metricsStorage.Set(getHash(filterLastScrapeFinishTime.String(), f.instance, f.Name), prometheus.MustNewConstMetric(filterLastScrapeFinishTime, prometheus.GaugeValue, float64(time.Now().Unix()), f.instance, f.Name))
	metricsStorage.Set(getHash(filterLastScrapeDuration.String(), f.instance, f.Name), prometheus.MustNewConstMetric(filterLastScrapeDuration, prometheus.GaugeValue, float64(time.Now().Unix()-f.startProcessing), f.instance, f.Name))
}

func (f BuildFilter) collectBuildsStat(c *tc.Client, wg *sync.WaitGroup) {
	defer wg.Done()

	routineID := xid.New().String()

	stat, err := c.GetBuildStatistics(f.Filter)
	if err != nil {
		logrus.WithFields(logsFormatter(f.Filter)).WithFields(logrus.Fields{
			"filter":          f.Name,
			"instance":        f.instance,
			"filterRoutineID": routineID,
		}).Error(err)
	}

	if len(stat.Property) == 0 {
		logrus.WithFields(logsFormatter(f.Filter)).WithFields(logrus.Fields{
			"filter":           f.Name,
			"instance":         f.instance,
			"metricsCollected": len(stat.Property),
			"filterRoutineID":  routineID,
			"timestamp":        time.Now().Unix(),
			"time":             time.Now(),
			"duration":         time.Now().Unix() - f.startProcessing,
		}).Debug("No metrics collected for filter")
		return
	}
	logrus.WithFields(logsFormatter(f.Filter)).WithFields(logrus.Fields{
		"filter":           f.Name,
		"instance":         f.instance,
		"metricsCollected": len(stat.Property),
		"filterRoutineID":  routineID,
		"timestamp":        time.Now().Unix(),
		"time":             time.Now(),
		"duration":         time.Now().Unix() - f.startProcessing,
	}).Debug("Successfully collected metrics for filter")

	for k := range stat.Property {
		value, _ := strconv.ParseFloat(stat.Property[k].Value, 64)
		metric := strings.SplitN(stat.Property[k].Name, ":", 2)
		title := toSnakeCase(metric[0])

		labels := []Label{
			{"exporter_instance", f.instance},
			{"exporter_filter", f.Name},
			{"build_configuration", stat.UsedFilter.BuildType},
		}
		if len(metric) > 1 {
			labels = append(labels, Label{"other", metric[1]})
		}

		labelsTitles, labelsValues := []string{}, []string{}
		for v := range labels {
			labelsTitles = append(labelsTitles, labels[v].Name)
			labelsValues = append(labelsValues, labels[v].Value)
		}

		desc := prometheus.NewDesc(title, title, labelsTitles, nil)
		metricsStorage.Set(getHash(title, labelsValues...), prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, value, labelsValues...))
		logrus.WithFields(logrus.Fields{
			"name":                title,
			"value":               value,
			"labels":              labels,
			"filter":              f.Name,
			"instance":            f.instance,
			"build_configuration": stat.UsedFilter.BuildType,
			"filterRoutineID":     routineID,
			"timestamp":           time.Now().Unix(),
			"time":                time.Now(),
		}).Debug("Saving metric to temporary storage")
	}
}
