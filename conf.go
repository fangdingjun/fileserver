package main

import (
	"github.com/go-yaml/yaml"
	"io/ioutil"
)

type conf []server

type server struct {
	Host        string
	Port        int
	Docroot     string
	URLRules    []rule
	EnableProxy bool
	Vhost       []vhost
}

type vhost struct {
	Docroot  string
	Hostname string
	Cert     string
	Key      string
	URLRules []rule
}

type rule struct {
	URLPrefix string
	IsRegex   bool
	Docroot   string
	Type      string
	Target    target
}

type target struct {
	Type string
	Host string
	Port int
	Path string
}

func loadConfig(fn string) (conf, error) {
	data, err := ioutil.ReadFile(fn)
	if err != nil {
		return nil, err
	}

	var c conf
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}

	return c, nil
}
