package main

import (
	"flag"
	//"fmt"
	"log"
	//"net/http"
)

func main() {
	var configfile string
	flag.StringVar(&configfile, "c", "config.yaml", "config file")
	flag.Parse()
	c, err := loadConfig(configfile)
	if err != nil {
		log.Fatal(err)
	}
	initRouters(c)
	select {}
}
