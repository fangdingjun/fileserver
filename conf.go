package main

import (
	"github.com/go-yaml/yaml"
	"io/ioutil"
)

type conf struct {
	Listen   []listen
	Docroot  string
	URLRules []rule
}

type listen struct {
	Host        string
	Port        string
	Cert        string
	Key         string
	EnableProxy bool
}

type rule struct {
	URLPrefix string
	IsRegex   bool
	Type      string
	Target    target
}

type target struct {
	Type string
	Host string
	Port int
	Path string
}

func loadConfig(fn string) (*conf, error) {
	data, err := ioutil.ReadFile(fn)
	if err != nil {
		return nil, err
	}

	var c conf
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}

	return &c, nil
}
