package main

import (
	"errors"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"regexp"
)

func (c *Configuration) parseConfig(path string) error {
	file, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(file, c)
	if err != nil {
		return err
	}
	return nil
}

func (c *Configuration) validateConfig() error {
	for i := range c.Instances {
		if c.Instances[i].Name == "" {
			return errors.New(fmt.Sprintf("Configuration error. Field 'name' is empty for instance %d\n", i))
		}
		if c.Instances[i].URL == "" {
			return errors.New(fmt.Sprintf("Configuration error. Field 'url' is empty for instance '%s'\n", c.Instances[i].Name))
		}
		if flag, _ := regexp.Match("^http://|https://", []byte(c.Instances[i].URL)); !flag {
			return errors.New(fmt.Sprintf("Configuration error. Field 'url' must start from http:// or https:// prefix in instance '%s'\n", c.Instances[i].Name))
		}
		if c.Instances[i].Username == "" {
			return errors.New(fmt.Sprintf("Configuration error. Field 'username' is empty for instance '%s'\n", c.Instances[i].Name))
		}
		if c.Instances[i].Password == "" {
			return errors.New(fmt.Sprintf("Configuration error. Field 'password' is empty for instance '%s'\n", c.Instances[i].Name))
		}
		if c.Instances[i].ScrapeInterval == int64(0) {
			return errors.New(fmt.Sprintf("Configuration error. Field 'scrape_interval' is empty for instance '%s'\n", c.Instances[i].Name))
		}
		if len(c.Instances[i].BuildsFilters) == 0 {
			return errors.New(fmt.Sprintf("Configuration error. No builds filters found for instance is empty for instance '%s'\n", c.Instances[i].Name))
		}

		for v := range c.Instances[i].BuildsFilters {
			for k := range c.Instances[i].BuildsFilters {
				if v == k {
					continue
				}
				if c.Instances[i].BuildsFilters[v].Name == c.Instances[i].BuildsFilters[k].Name {
					return errors.New(fmt.Sprintf("Configuration error. Several filters in instance '%s' have the same name '%s'\n", c.Instances[i].Name, c.Instances[i].BuildsFilters[v].Name))
				}
			}
		}
	}
	return nil
}
