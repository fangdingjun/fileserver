package main

import (
	"crypto/tls"
	"fmt"
	"github.com/gorilla/mux"
	"net"
	"net/http"
	//"net/url"
	"log"
	"os"
	"regexp"
	//"path/filepath"
	"strings"
)

func initRouters(cfg conf) {

	for _, l := range cfg {
		router := mux.NewRouter()
		domains := []string{}
		certs := []tls.Certificate{}

		// initial virtual host
		for _, h := range l.Vhost {
			h2 := h.Hostname
			if h1, _, err := net.SplitHostPort(h.Hostname); err == nil {
				h2 = h1
			}
			domains = append(domains, h2)
			if h.Cert != "" && h.Key != "" {
				if cert, err := tls.LoadX509KeyPair(h.Cert, h.Key); err == nil {
					certs = append(certs, cert)
				} else {
					log.Fatal(err)
				}
			}
			r := router.Host(h2).Subrouter()
			for _, rule := range h.UrlRules {
				switch rule.Type {
				case "alias":
					registerAliasHandler(rule, r)
				case "uwsgi":
					registerUwsgiHandler(rule, r)
				case "fastcgi":
					registerFastCGIHandler(rule, h.Docroot, r)
				case "http":
					registerHTTPHandler(rule, r)
				default:
					fmt.Printf("invalid type: %s\n", rule.Type)
				}
			}
			r.PathPrefix("/").Handler(http.FileServer(http.Dir(h.Docroot)))
		}

		// default host config
		for _, rule := range l.UrlRules {
			switch rule.Type {
			case "alias":
				registerAliasHandler(rule, router)
			case "uwsgi":
				registerUwsgiHandler(rule, router)
			case "fastcgi":
				registerFastCGIHandler(rule, l.Docroot, router)
			case "http":
				registerHTTPHandler(rule, router)
			default:
				fmt.Printf("invalid type: %s\n", rule.Type)
			}
		}

		router.PathPrefix("/").Handler(http.FileServer(http.Dir(l.Docroot)))

		go func(l server) {
			addr := fmt.Sprintf("%s:%d", l.Host, l.Port)
			hdlr := &handler{
				handler:      router,
				enableProxy:  l.EnableProxy,
				localDomains: domains,
			}
			if len(certs) > 0 {
				tlsconfig := &tls.Config{
					Certificates: certs,
				}

				tlsconfig.BuildNameToCertificate()

				srv := http.Server{
					Addr:      addr,
					TLSConfig: tlsconfig,
					Handler:   hdlr,
				}
				log.Printf("listen https on %s", addr)
				if err := srv.ListenAndServeTLS("", ""); err != nil {
					log.Fatal(err)
				}

			} else {
				log.Printf("listen http on %s", addr)
				if err := http.ListenAndServe(addr, hdlr); err != nil {
					log.Fatal(err)
				}
			}
		}(l)
	}
}

func registerAliasHandler(r rule, router *mux.Router) {
	switch r.Target.Type {
	case "file":
		registerFileHandler(r, router)
	case "dir":
		registerDirHandler(r, router)
	default:
		fmt.Printf("invalid type: %s, only file, dir allowed\n", r.Target.Type)
		os.Exit(-1)
	}
}

func registerFileHandler(r rule, router *mux.Router) {
	router.HandleFunc(r.URLPrefix,
		func(w http.ResponseWriter, req *http.Request) {
			http.ServeFile(w, req, r.Target.Path)
		})
}

func registerDirHandler(r rule, router *mux.Router) {
	p := strings.TrimRight(r.URLPrefix, "/")
	router.PathPrefix(r.URLPrefix).Handler(
		http.StripPrefix(p,
			http.FileServer(http.Dir(r.Target.Path))))
}

func registerUwsgiHandler(r rule, router *mux.Router) {
	var p string
	switch r.Target.Type {
	case "unix":
		p = r.Target.Path
	case "tcp":
		p = fmt.Sprintf("%s:%d", r.Target.Host, r.Target.Port)
	default:
		fmt.Printf("invalid scheme: %s, only support unix, tcp", r.Target.Type)
		os.Exit(-1)
	}

	if r.IsRegex {
		m1 := myURLMatch{regexp.MustCompile(r.URLPrefix)}
		u := NewUwsgi(r.Target.Type, p, "")
		router.MatcherFunc(m1.match).Handler(u)
	} else {
		u := NewUwsgi(r.Target.Type, p, r.URLPrefix)
		router.PathPrefix(r.URLPrefix).Handler(u)
	}
}

func registerFastCGIHandler(r rule, docroot string, router *mux.Router) {
	var p string
	switch r.Target.Type {
	case "unix":
		p = r.Target.Path
	case "tcp":
		p = fmt.Sprintf("%s:%d", r.Target.Host, r.Target.Port)
	default:
		fmt.Printf("invalid scheme: %s, only support unix, tcp", r.Target.Type)
		os.Exit(-1)
	}
	if r.IsRegex {
		m1 := myURLMatch{regexp.MustCompile(r.URLPrefix)}
		u, _ := NewFastCGI(r.Target.Type, p, docroot, "")
		router.MatcherFunc(m1.match).Handler(u)
	} else {
		u, _ := NewFastCGI(r.Target.Type, p, docroot, r.URLPrefix)
		router.PathPrefix(r.URLPrefix).Handler(u)
	}
}

func registerHTTPHandler(r rule, router *mux.Router) {
	var u http.Handler
	var addr string
	switch r.Target.Type {
	case "unix":
		addr = r.Target.Path
	case "http":
		addr = fmt.Sprintf("%s:%d", r.Target.Host, r.Target.Port)
	default:
		fmt.Printf("invalid scheme: %s, only support unix, http", r.Target.Type)
		os.Exit(-1)
	}
	u = newProxy(addr, r.URLPrefix)
	p := strings.TrimRight(r.URLPrefix, "/")
	router.PathPrefix(r.URLPrefix).Handler(
		http.StripPrefix(p, u))
}

type myURLMatch struct {
	re *regexp.Regexp
}

func (m myURLMatch) match(r *http.Request, route *mux.RouteMatch) bool {
	ret := m.re.MatchString(r.URL.Path)
	return ret
}
