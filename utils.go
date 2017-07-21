package main

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/fatih/structs"
	"github.com/sirupsen/logrus"
)

func toSnakeCase(s string) string {
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

func getHash(t string, s ...string) string {
	for i := range s {
		t = fmt.Sprintf(t, s[i])
	}
	hash := sha256.Sum256([]byte(t))
	return string(hash[:len(hash)])
}

func newTicker(d time.Duration) *ticker {
	stdTicker := time.NewTicker(d)
	c := make(chan time.Time, 1)
	newTicker := &ticker{C: c}
	go func() {
		newTicker.C <- time.Now()
		for _ = range stdTicker.C {
			newTicker.C <- time.Now()
		}
	}()
	return newTicker
}

func labelsToString(l []Label) string {
	res := ""
	for i := range l {
		res += l[i].Name + "→" + l[i].Value + ","
	}
	return strings.TrimRight(res, ",")
}
