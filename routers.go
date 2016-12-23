package main

import (
	"fmt"
	"github.com/gorilla/mux"
	"net/http"
	//"net/url"
	"os"
	"regexp"
	//"path/filepath"
	"strings"
)

func initRouters(cfg *conf) {
	router := mux.NewRouter()

	for _, r := range cfg.URLRules {
		switch r.Type {
		case "alias":
			registerAliasHandler(r, router)
		case "uwsgi":
			registerUwsgiHandler(r, router)
		case "fastcgi":
			registerFastCGIHandler(r, cfg.Docroot, router)
		case "http":
			registerHTTPHandler(r, router)
		default:
			fmt.Printf("invalid type: %s\n", r.Type)
		}
	}

	router.PathPrefix("/").Handler(http.FileServer(http.Dir(cfg.Docroot)))

	http.Handle("/", router)
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
