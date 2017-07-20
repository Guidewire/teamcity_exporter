# Teamcity Exporter

Export Teamcity builds metrics to Prometheus.

To build it:

```bash
$ docker run --rm -v "$PWD":/go/src/github.com/guidewire/teamcity_exporter -w /go/src/github.com/guidewire/teamcity_exporter -e GOOS=linux -e GOARCH=amd64 golang:1.8 go build -o bin/teamcity_exporter -v
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

* __`config`:__ Path to configuration file.
* __`web.listen-address`:__ Address to listen on for web interface and telemetry.
* __`web.telemetry-path`:__ Path under which to expose metrics.
* __`log.level`:__ Logging level. `info` by default.
* __`log.format`:__ Set the log target and format. Example: `logger:syslog?appname=bob&local=7` or `logger:stdout?json=true` (default `logger:stderr`)
* __`version`:__ Print current version.

## Configuration
Configuration template:

```yaml
instances:
- name: instance1
  url: https://teamcity-instance1.com
  username: login
  password: password
  scrape_interval: 100 # seconds
  builds_filters:
  - name: filter1
    filter:
      status: success
      running: false
      canceled: false
      count: 1
- name: instance2
  url: https://teamcity-instance2.com
  username: login
  password: password
  scrape_interval: 80 # seconds
  builds_filters:
  - name: filter1
    filter:
      build_type: filter1
      status: failure
      running: false
      canceled: false
      count: 1
  - name: filter2
    filter:
      build_type: filter1
      branch: master
      status: success
      running: false
      canceled: false
      count: 1
```

### Available builds filters
| Filter name | Possible values | Description |
|-------------|-----------------|-------------|
| build_type  | depends on your setup, example `myBuildConfiguration` | builds of the specified build configuration. By default, the filter will run against all available configurations. |
| branch      | depends on your setup, example `master` | limit the builds by branch |
| status      | `success`/`failure`/`error` | list builds with the specified status only |
| running     | `true`/`false`/`any` | limit builds by the running flag. By default, running builds are not included. |
| canceled    | `true`/`false`/`any` | limit builds by the canceled flag. By default, canceled builds are not included. |
| count       | any number, example `1` | serve only the specified number of builds |

For more details, read official Teamcity [documentation](https://confluence.jetbrains.com/display/TCD10/REST+API#RESTAPI-BuildLocator). At the moment not all the filters' parameters are available in this project.
