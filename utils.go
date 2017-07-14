package main

import (
	"crypto/sha256"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/fatih/structs"
	"regexp"
	"strings"
)

func splitMetricsTitle(s string) map[string]string {
	res := strings.SplitAfterN(s, ":", 2)
	res[0] = strings.Replace(res[0], ":", "", -1)
	if len(res) > 1 {
		return map[string]string{res[0]: res[1]}
	}
	return map[string]string{res[0]: ""}
}

func convertCamelCaseToSnakeCase(s string) string {
	a := []string{}
	camel := regexp.MustCompile("(^[^A-Z]*|[A-Z]*)([A-Z][^A-Z]+|$)")

	for _, sub := range camel.FindAllStringSubmatch(s, -1) {
		if sub[1] != "" {
			a = append(a, sub[1])
		}
		if sub[2] != "" {
			a = append(a, sub[2])
		}
	}
	return strings.ToLower(strings.Join(a, "_"))
}

func logsFormatter(f interface{}) logrus.Fields {
	m := structs.Map(f)
	r := logrus.Fields{}
	for k, v := range m {
		if v == "" {
			continue
		}
		r[strings.ToLower(k)] = v
	}
	return r
}

func getHash(s ...string) string {
	str := ""
	for i := range s {
		str = fmt.Sprintf(str, s[i])
	}
	hash := sha256.Sum256([]byte(str))
	return string(hash[:len(hash)])
}
