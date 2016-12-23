package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

func initListeners(c *conf) {
	for _, l := range c.Listen {
		go func(l listen) {
			addr := fmt.Sprintf("%s:%d", l.Host, l.Port)
			if l.Cert != "" && l.Key != "" {
				if err := http.ListenAndServeTLS(addr, l.Cert, l.Key,
					&handler{enableProxy: l.EnableProxy}); err != nil {
					log.Fatal(err)
				}
			} else {
				if err := http.ListenAndServe(addr,
					&handler{enableProxy: l.EnableProxy}); err != nil {
					log.Fatal(err)
				}
			}
		}(l)
	}
}

func main() {
	var configfile string
	flag.StringVar(&configfile, "-c", "config.yaml", "config file")
	flag.Parse()
	c, err := loadConfig(configfile)
	if err != nil {
		log.Fatal(err)
	}
	initRouters(c)
	initListeners(c)
	select {}
}
