package main

import (
	"errors"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net/http"
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
		client := &http.Client{}
		req, err := http.NewRequest("GET", c.Instances[i].URL, nil)
		if err != nil {
			return err
		}
		req.SetBasicAuth(c.Instances[i].Username, c.Instances[i].Password)
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode == 401 {
			return errors.New(fmt.Sprintf("Unauthorized %s", c.Instances[i].Name))
		}
	}
	return nil
}
