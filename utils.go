package main

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
	"time"
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

func getHash(t string, s ...string) string {
	for i := range s {
		t = fmt.Sprintf(t, s[i])
	}
	hash := sha256.Sum256([]byte(t))
	return string(hash[:len(hash)])
}

func newTicker(d time.Duration) *Ticker {
	stdTicker := time.NewTicker(d)
	ch := make(chan time.Time, 1)
	newTicker := &Ticker{c: ch}
	go func() {
		newTicker.c <- time.Now()
		for range stdTicker.C {
			newTicker.c <- time.Now()
		}
	}()
	return newTicker
}
