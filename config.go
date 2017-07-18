package main

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
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
	return nil
}
