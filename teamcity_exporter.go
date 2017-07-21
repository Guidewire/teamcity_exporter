package main

import (
	"errors"
	"flag"
	"fmt"
	tc "github.com/guidewire/teamcity-go-bindings"
	"github.com/orcaman/concurrent-map"
	"github.com/prometheus/client_golang/prometheus"
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
		logLevel      = flag.String("log.level", "info", "Set log level")
	)
	flag.Parse()

	logrus.Info("Starting teamcity_exporter" + version.Info())
	logrus.Info("Build context", version.BuildContext())

	level, err := logrus.ParseLevel(*logLevel)
	if err != nil {
		logrus.Fatal(err)
	}
	logrus.SetLevel(level)
	logrus.Infof("Log level was set to %s", level.String())

	if *showVersion {
		logrus.Info(os.Stdout, version.Print("teamcity_exporter"))
		return
	}

	collector := NewCollector()
	prometheus.MustRegister(collector)

	config := Configuration{}
	if err := config.parseConfig(*configPath); err != nil {
		logrus.WithFields(logrus.Fields{"config": *configPath}).Fatal(err)
	}
	if err := config.validateConfig(); err != nil {
		logrus.WithFields(logrus.Fields{"config": *configPath}).Fatal(err)
	}

	for i := range config.Instances {
		logrus.WithFields(logrus.Fields{
			"config":         *configPath,
			"instance":       config.Instances[i].Name,
			"scrapeInterval": config.Instances[i].ScrapeInterval,
		}).Debug("Reading configuration, found teamcity instance")
		go config.Instances[i].collectInstanceStat()
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
	logrus.Infoln("Listening on", *listenAddress)
	logrus.Fatal(http.ListenAndServe(*listenAddress, nil))
}

func (i Instance) collectInstanceStat() {
	client := tc.New(i.URL, i.Username, i.Password)
	wg := &sync.WaitGroup{}

	ticker := newTicker(time.Duration(i.ScrapeInterval) * time.Second)
	for _ = range ticker.C {
		startProcessing := time.Now()
		routineID := xid.New().String()

		logrus.WithFields(logrus.Fields{
			"instance":          i.Name,
			"timestamp":         startProcessing,
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

		if len(i.BuildsFilters) == 0 {
			filter := BuildFilter{
				Name:     "default",
				instance: i.Name,
				Filter:   *tc.NewBuildLocator(),
			}
			i.BuildsFilters = append(i.BuildsFilters, filter)
		}

		for v := range i.BuildsFilters {
			go i.BuildsFilters[v].getBuildsByFilter(client, wg)
		}

		wg.Wait()
		finishProcessing := time.Now()
		metricsStorage.Set(getHash(instanceLastScrapeFinishTime.String(), i.Name), prometheus.MustNewConstMetric(instanceLastScrapeFinishTime, prometheus.GaugeValue, float64(finishProcessing.Unix()), i.Name))
		metricsStorage.Set(getHash(instanceLastScrapeDuration.String(), i.Name), prometheus.MustNewConstMetric(instanceLastScrapeDuration, prometheus.GaugeValue, float64(finishProcessing.Sub(startProcessing)/time.Second), i.Name))
		logrus.WithFields(logrus.Fields{
			"instance":          i.Name,
			"instanceRoutineID": routineID,
			"storageSize":       metricsStorage.Count(),
			"timestamp":         finishProcessing.Unix(),
			"time":              finishProcessing,
			"duration":          finishProcessing.Sub(startProcessing) / time.Second,
		}).Debug("Successfully collected metrics for instance")
	}
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
		return errors.New("Unauthorized")
	}
	metricsStorage.Set(getHash(instanceStatus.String(), i.Name), prometheus.MustNewConstMetric(instanceStatus, prometheus.GaugeValue, 1, i.Name))
	return nil
}

func (f BuildFilter) getBuildsByFilter(c *tc.Client, wg *sync.WaitGroup) {
	defer wg.Done()
	f.startProcessing = time.Now()
	wgFilter := &sync.WaitGroup{}

	builds := tc.Builds{}

	if f.Filter.Branch == "" {
		f.Filter.Branch = "default:any"
	}

	// default filter
	if f.Name == "default" && f.Filter.Count == "1" {
		buildCfgs, err := c.GetAllBuildConfigurations()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"instance": f.instance,
				"filter":   f.Name,
			}).Error(err)
		}
		for z := range buildCfgs.BuildType {
			f.Filter.BuildType = buildCfgs.BuildType[z].ID
			b, err := c.GetBuildsByParams(f.Filter)
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"instance":            f.instance,
					"filter":              f.Name,
					"build_configuration": f.Filter.BuildType,
				}).Error(err)
			}
			for i := range b.Build {
				builds.Build = append(builds.Build, b.Build[i])
				builds.Count += b.Count
			}
		}
	}

	b, err := c.GetBuildsByParams(f.Filter)
	if err != nil {
		logrus.WithFields(logsFormatter(f.Filter)).WithFields(logrus.Fields{
			"filter":   f.Name,
			"instance": f.instance,
		}).Error(err)
	}

	for i := range b.Build {
		builds.Build = append(builds.Build, b.Build[i])
		builds.Count += b.Count
	}

	for i := range builds.Build {
		wgFilter.Add(1)
		go f.collectBuildStat(builds.Build[i], c, wgFilter)
	}

	wgFilter.Wait()
	finishProcessing := time.Now()
	metricsStorage.Set(getHash(filterLastScrapeFinishTime.String(), f.instance, f.Name), prometheus.MustNewConstMetric(filterLastScrapeFinishTime, prometheus.GaugeValue, float64(finishProcessing.Unix()), f.instance, f.Name))
	metricsStorage.Set(getHash(filterLastScrapeDuration.String(), f.instance, f.Name), prometheus.MustNewConstMetric(filterLastScrapeDuration, prometheus.GaugeValue, float64(finishProcessing.Sub(f.startProcessing)/time.Second), f.instance, f.Name))
}

func (f BuildFilter) collectBuildStat(build tc.Build, c *tc.Client, wg *sync.WaitGroup) {
	defer wg.Done()
	routineID := xid.New().String()

	stat, err := c.GetBuildStat(build.ID)
	if err != nil {
		logrus.WithFields(logsFormatter(f.Filter)).WithFields(logrus.Fields{
			"filter":          f.Name,
			"instance":        f.instance,
			"filterRoutineID": routineID,
		}).Error(err)
	}

	t := time.Now()
	if len(stat.Property) == 0 {
		logrus.WithFields(logsFormatter(f.Filter)).WithFields(logrus.Fields{
			"filter":           f.Name,
			"instance":         f.instance,
			"metricsCollected": len(stat.Property),
			"filterRoutineID":  routineID,
			"timestamp":        t.Unix(),
			"time":             t,
			"duration":         t.Sub(f.startProcessing) / time.Second,
		}).Debug("No metrics collected for filter")
		return
	}
	logrus.WithFields(logsFormatter(f.Filter)).WithFields(logrus.Fields{
		"filter":           f.Name,
		"instance":         f.instance,
		"metricsCollected": len(stat.Property),
		"filterRoutineID":  routineID,
		"timestamp":        t.Unix(),
		"time":             t,
		"duration":         t.Sub(f.startProcessing) / time.Second,
	}).Debug("Successfully collected metrics for filter")

	for k := range stat.Property {
		value, _ := strconv.ParseFloat(stat.Property[k].Value, 64)
		metric := strings.SplitN(stat.Property[k].Name, ":", 2)
		title := fmt.Sprint(namespace, "_", toSnakeCase(metric[0]))

		labels := []Label{
			{"exporter_instance", f.instance},
			{"exporter_filter", f.Name},
			{"build_configuration", build.BuildTypeID},
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
		t := time.Now()
		logrus.WithFields(logrus.Fields{
			"name":                title,
			"value":               value,
			"labels":              labelsToString(labels),
			"filter":              f.Name,
			"instance":            f.instance,
			"build_configuration": build.BuildTypeID,
			"filterRoutineID":     routineID,
			"timestamp":           t.Unix(),
			"time":                t,
		}).Debug("Saving metric to temporary storage")
	}
}
