# Teamcity Exporter

Export Teamcity builds metrics to Prometheus.

To build it:

```bash
$ docker run -it --rm -v "$PWD":/go/src/github.com/guidewire/teamcity_exporter -w /go/src/github.com/guidewire/teamcity_exporter -e GOOS=linux -e GOARCH=amd64 -e GOPATH=/go/src/github.com/guidewire/teamcity_exporter -e GOBIN=/go/src/github.com/guidewire/teamcity_exporter/bin golang:1.8 go get
```

To run it:

```bash
$ ./teamcity_exporter [flags]
```

## Exported Metrics

Export all the metrics that Teamcity's _/statistics_ endpoint [provides](https://confluence.jetbrains.com/display/TCD10/Custom+Chart#CustomChart-listOfDefaultStatisticValues).

## Flags

```bash
./teamcity_exporter --help
```

* __`config`:__ Path to configuration file
* __`web.listen-address`:__ Address to listen on for web interface and telemetry
* __`web.telemetry-path`:__ Path under which to expose metrics
* __`log.level`:__ Only log messages with the given severity or above. Valid levels: [debug, info, warn, error, fatal]
* __`log.format`:__ Set the log target and format. Example: `logger:syslog?appname=bob&local=7` or `logger:stdout?json=true` (default `logger:stderr`)
* __`version`:__ Print current version

## Configuration
Configuration template:

```yaml
instances:
- name: prod
  url: https://teamcity-prod.com
  username: login
  password: password
  scrape_interval: 100 # seconds
  concurrency_limit: 10 # simultaneous Teamcity connections
  builds_filters:
  - name: prod-filter1
    filter:
      status: success
      running: false
      canceled: false
- name: dev
  url: https://teamcity-dev.com
  username: login
  password: password
  scrape_interval: 80 # seconds
  concurrency_limit: 10 # simultaneous Teamcity connections
  builds_filters:
  - name: dev-filter1
    filter:
      build_type: buildtype1
      status: failure
      running: false
      canceled: false
  - name: dev-filter2
    filter:
      build_type: buildtype2
      branch: master
      status: success
      running: false
      canceled: false
```

### Available builds filters
| Filter name | Possible values | Description |
|-------------|-----------------|-------------|
| build_type  | depends on your setup, example `myBuildConfiguration` | builds of the specified build configuration. By default, the filter will run against all available configurations. |
| branch      | depends on your setup, example `master` | limit the builds by branch |
| status      | `success`/`failure`/`error` | list builds with the specified status only |
| running     | `true`/`false`/`any` | limit builds by the running flag. By default, running builds are not included. |
| canceled    | `true`/`false`/`any` | limit builds by the canceled flag. By default, canceled builds are not included. |

For more details, read official Teamcity [documentation](https://confluence.jetbrains.com/display/TCD10/REST+API#RESTAPI-BuildLocator). At the moment not all the filters' parameters are available in this project.
