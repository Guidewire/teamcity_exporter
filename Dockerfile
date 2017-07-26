FROM        quay.io/prometheus/busybox:latest
MAINTAINER  Roman Tishchenko <vala4i@gmail.com>

COPY teamcity_exporter /bin/teamcity_exporter

EXPOSE     9107
ENTRYPOINT [ "/bin/teamcity_exporter" ]
