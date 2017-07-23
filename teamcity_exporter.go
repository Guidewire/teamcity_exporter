package main

import (
	"errors"
	"flag"
	"fmt"
	tc "github.com/guidewire/teamcity-go-bindings"
	"github.com/orcaman/concurrent-map"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/version"
	// "github.com/rs/xid"
	// "github.com/fatih/structs"
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
		go config.Instances[i].collectStat()
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

func (i *Instance) collectStat() {
	// client := tc.New(i.URL, i.Username, i.Password)
	// wg := &sync.WaitGroup{}

	ticker := newTicker(time.Duration(i.ScrapeInterval) * time.Second)
	for _ = range ticker.C {
		go i.collectStatHandler()
	}
	// 	startProcessing := time.Now()
	// 	routineID := xid.New().String()
	//
	// 	logrus.WithFields(logrus.Fields{
	// 		"instance":          i.Name,
	// 		"timestamp":         startProcessing,
	// 		"time":              time.Now(),
	// 		"instanceRoutineID": routineID,
	// 	}).Debug("Starting metrics collection")
	//
	// 	if err := i.validateStatus(); err != nil {
	// 		logrus.WithFields(logrus.Fields{
	// 			"instance":          i.Name,
	// 			"instanceRoutineID": routineID,
	// 		}).Error(err)
	// 		continue
	// 	}
	//
	// 	if len(i.BuildsFilters) == 0 {
	// 		i.addDefaultFilter()
	// 	}
	//
	// 	for v := range i.BuildsFilters {
	// 		wg.Add(1)
	// 		i.BuildsFilters[v].instance = i.Name
	// 		go i.BuildsFilters[v].getBuildsByFilter(client, wg)
	// 	}
	//
	// 	wg.Wait()
	// 	finishProcessing := time.Now()
	// 	metricsStorage.Set(getHash(instanceLastScrapeFinishTime.String(), i.Name), prometheus.MustNewConstMetric(instanceLastScrapeFinishTime, prometheus.GaugeValue, float64(finishProcessing.Unix()), i.Name))
	// 	metricsStorage.Set(getHash(instanceLastScrapeDuration.String(), i.Name), prometheus.MustNewConstMetric(instanceLastScrapeDuration, prometheus.GaugeValue, float64(finishProcessing.Sub(startProcessing)/time.Second), i.Name))
	// 	logrus.WithFields(logrus.Fields{
	// 		"instance":          i.Name,
	// 		"instanceRoutineID": routineID,
	// 		"storageSize":       metricsStorage.Count(),
	// 		"timestamp":         finishProcessing.Unix(),
	// 		"time":              finishProcessing,
	// 		"duration":          int(finishProcessing.Sub(startProcessing) / time.Second),
	// 	}).Debug("Successfully collected metrics for instance")
	// }
}

func (i Instance) collectStatHandler() {
	client := tc.New(i.URL, i.Username, i.Password)
	chBuildFilter := make(chan BuildFilter)
	chBuild := make(chan tc.Build)
	chBuildStat := make(chan BuildStatistics)

	wg := &sync.WaitGroup{}
	wg.Add(4)
	go i.prepareFilters(client, wg, chBuildFilter)
	go i.getBuildsByFilters(client, wg, chBuildFilter, chBuild)
	go i.getBuildStat(client, wg, chBuild, chBuildStat)
	go i.parseStat(wg, chBuildStat)

	// for i := range chBuildStat {
	// 	fmt.Println(structs.Map(i))
	// }
	wg.Wait()
}

func (i *Instance) prepareFilters(c *tc.Client, wg *sync.WaitGroup, ch chan<- BuildFilter) {
	defer wg.Done()

	if len(i.BuildsFilters) == 0 {
		i.addDefaultFilter()
	}

	counter := 0

	for k := range i.BuildsFilters {
		bt := tc.BuildConfiguration{}
		b := map[tc.BuildTypeID][]tc.Branch{}

		if i.BuildsFilters[k].Filter.BuildType == "" {
			bt, _ = c.GetAllBuildConfigurations()
		} else {
			//fmt.Println(i.BuildsFilters[k].Filter.BuildType)
			bt = tc.BuildConfiguration{BuildTypes: []tc.BuildType{{ID: tc.BuildTypeID(i.BuildsFilters[k].Filter.BuildType)}}}
		}
		// fmt.Println(bt)

		if i.BuildsFilters[k].Filter.Branch == "" {
			for v := range bt.BuildTypes {
				// fmt.Println(buildTypes.BuildTypes[v].ID)
				branches, _ := c.GetAllBranches(bt.BuildTypes[v].ID)
				b[bt.BuildTypes[v].ID] = branches.Branch
			}
		} else {
			for v := range bt.BuildTypes {
				b[bt.BuildTypes[v].ID] = []tc.Branch{{Name: i.BuildsFilters[k].Filter.Branch}}
			}
		}

		// fmt.Println(b)
		for bt, branches := range b {
			for z := range branches {
				f := BuildFilter{
					Name: i.BuildsFilters[k].Name,
					Filter: tc.BuildLocator{
						BuildType: string(bt),
						Branch:    branches[z].Name,
						Count:     "1"},
				}
				ch <- f
				counter++
			}
		}
	}
	close(ch)
	fmt.Println("prepareFilters | closing channel,", counter, " records")
}

func (i *Instance) getBuildsByFilters(c *tc.Client, wg *sync.WaitGroup, chIn <-chan BuildFilter, chOut chan<- tc.Build) {
	defer wg.Done()
	wg1 := &sync.WaitGroup{}

	counter := 0
	for i := range chIn {
		wg1.Add(1)
		go func(i BuildFilter) {
			defer wg1.Done()
			b, _ := c.GetBuildsByParams(i.Filter)
			for v := range b.Build {
				chOut <- b.Build[v]
				counter++
			}
		}(i)
	}

	wg1.Wait()
	close(chOut)
	fmt.Println("getBuildsByFilters | closing channel,", counter, " records")
}

func (i *Instance) getBuildStat(c *tc.Client, wg *sync.WaitGroup, chIn <-chan tc.Build, chOut chan<- BuildStatistics) {
	defer wg.Done()
	wg1 := &sync.WaitGroup{}
	counter := 0
	for i := range chIn {
		wg1.Add(1)
		go func(i tc.Build) {
			defer wg1.Done()
			s, _ := c.GetBuildStat(i.ID)
			chOut <- BuildStatistics{Build: i, Stat: s}
			counter++
		}(i)
	}

	wg1.Wait()
	close(chOut)
	fmt.Println("getBuildStat | closing channel,", counter, " records")
}

func (instance *Instance) parseStat(wg *sync.WaitGroup, chIn <-chan BuildStatistics) {
	defer wg.Done()

	for i := range chIn {
		for k := range i.Stat.Property {
			value, _ := strconv.ParseFloat(i.Stat.Property[k].Value, 64)
			metric := strings.SplitN(i.Stat.Property[k].Name, ":", 2)
			title := fmt.Sprint(namespace, "_", toSnakeCase(metric[0]))

			labels := []Label{
				{"exporter_instance", instance.Name},
				// {"exporter_filter", f.Name},
				{"build_configuration", string(i.Build.BuildTypeID)},
				{"branch", i.Build.BranchName},
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
		}
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

func (i *Instance) addDefaultFilter() BuildFilter {
	f := BuildFilter{
		Name:     "default",
		instance: i.Name,
		Filter:   *tc.NewBuildLocator(),
	}
	i.BuildsFilters = append(i.BuildsFilters, f)
	return f
}

// func (f BuildFilter) getBuildsByFilter(c *tc.Client, wg *sync.WaitGroup) {
// 	defer wg.Done()
// 	f.startProcessing = time.Now()
// 	wg1 := &sync.WaitGroup{}
//
// 	builds := tc.Builds{}
//
// 	if f.Filter.Branch == "" {
// 		f.Filter.Branch = "default:any"
// 	}
// 	if f.Filter.Count == "" {
// 		f.Filter.Count = "1"
// 	}
//
// 	wg2 := &sync.WaitGroup{}
// 	if f.Filter.Count == "1" && f.Filter.Branch == "default:any" && f.Filter.BuildType == "" {
// 		buildCfgs, err := c.GetAllBuildConfigurations()
// 		if err != nil {
// 			logrus.WithFields(logrus.Fields{
// 				"instance": f.instance,
// 				"filter":   f.Name,
// 			}).Error(err)
// 		}
// 		for z := range buildCfgs.BuildType {
// 			f.Filter.BuildType = buildCfgs.BuildType[z].ID
// 			wg2.Add(1)
// 			go func(f BuildFilter) {
// 				defer wg2.Done()
// 				b, err := c.GetBuildsByParams(f.Filter)
// 				if err != nil {
// 					logrus.WithFields(logrus.Fields{
// 						"instance":            f.instance,
// 						"filter":              f.Name,
// 						"build_configuration": f.Filter.BuildType,
// 					}).Error(err)
// 				}
// 				for i := range b.Build {
// 					builds.Build = append(builds.Build, b.Build[i])
// 					builds.Count += b.Count
// 				}
// 			}(f)
// 		}
// 	}
//
// 	b, err := c.GetBuildsByParams(f.Filter)
// 	if err != nil {
// 		logrus.WithFields(logsFormatter(f.Filter)).WithFields(logrus.Fields{
// 			"filter":   f.Name,
// 			"instance": f.instance,
// 		}).Error(err)
// 	}
//
// 	for i := range b.Build {
// 		builds.Build = append(builds.Build, b.Build[i])
// 		builds.Count += b.Count
// 	}
//
// 	wg2.Wait()
// 	for i := range builds.Build {
// 		logrus.Info(builds.Build[i])
// 		wg1.Add(1)
// 		go f.collectBuildStat(builds.Build[i], c, wg1)
// 	}
//
// 	wg1.Wait()
// 	finishProcessing := time.Now()
// 	metricsStorage.Set(getHash(filterLastScrapeFinishTime.String(), f.instance, f.Name), prometheus.MustNewConstMetric(filterLastScrapeFinishTime, prometheus.GaugeValue, float64(finishProcessing.Unix()), f.instance, f.Name))
// 	metricsStorage.Set(getHash(filterLastScrapeDuration.String(), f.instance, f.Name), prometheus.MustNewConstMetric(filterLastScrapeDuration, prometheus.GaugeValue, float64(finishProcessing.Sub(f.startProcessing)/time.Second), f.instance, f.Name))
// }

// func (f BuildFilter) collectBuildStat(build tc.Build, c *tc.Client, wg *sync.WaitGroup) {
// 	defer wg.Done()
// 	routineID := xid.New().String()
//
// 	stat, err := c.GetBuildStat(build.ID)
// 	if err != nil {
// 		logrus.WithFields(logsFormatter(f.Filter)).WithFields(logrus.Fields{
// 			"filter":          f.Name,
// 			"instance":        f.instance,
// 			"filterRoutineID": routineID,
// 		}).Error(err)
// 	}
//
// 	t := time.Now()
// 	if len(stat.Property) == 0 {
// 		logrus.WithFields(logrus.Fields{
// 			"filter":           f.Name,
// 			"instance":         f.instance,
// 			"metricsCollected": len(stat.Property),
// 			"filterRoutineID":  routineID,
// 			"timestamp":        t.Unix(),
// 			"time":             t,
// 			"duration":         int(t.Sub(f.startProcessing) / time.Second),
// 			"buildtype":        build.BuildTypeID,
// 		}).Debug("No metrics collected for filter")
// 		return
// 	}
// 	logrus.WithFields(logrus.Fields{
// 		"filter":           f.Name,
// 		"instance":         f.instance,
// 		"metricsCollected": len(stat.Property),
// 		"filterRoutineID":  routineID,
// 		"timestamp":        t.Unix(),
// 		"time":             t,
// 		"duration":         int(t.Sub(f.startProcessing) / time.Second),
// 		"buildtype":        build.BuildTypeID,
// 	}).Debug("Successfully collected metrics for filter")
//
// 	for k := range stat.Property {
// 		value, _ := strconv.ParseFloat(stat.Property[k].Value, 64)
// 		metric := strings.SplitN(stat.Property[k].Name, ":", 2)
// 		title := fmt.Sprint(namespace, "_", toSnakeCase(metric[0]))
//
// 		labels := []Label{
// 			{"exporter_instance", f.instance},
// 			{"exporter_filter", f.Name},
// 			{"build_configuration", build.BuildTypeID},
// 		}
// 		if len(metric) > 1 {
// 			labels = append(labels, Label{"other", metric[1]})
// 		}
//
// 		labelsTitles, labelsValues := []string{}, []string{}
// 		for v := range labels {
// 			labelsTitles = append(labelsTitles, labels[v].Name)
// 			labelsValues = append(labelsValues, labels[v].Value)
// 		}
//
// 		desc := prometheus.NewDesc(title, title, labelsTitles, nil)
// 		metricsStorage.Set(getHash(title, labelsValues...), prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, value, labelsValues...))
// 		t := time.Now()
// 		logrus.WithFields(logrus.Fields{
// 			"name":                title,
// 			"value":               value,
// 			"labels":              labelsToString(labels),
// 			"filter":              f.Name,
// 			"instance":            f.instance,
// 			"build_configuration": build.BuildTypeID,
// 			"filterRoutineID":     routineID,
// 			"timestamp":           t.Unix(),
// 			"time":                t,
// 		}).Debug("Saving metric to temporary storage")
// 	}
// }
