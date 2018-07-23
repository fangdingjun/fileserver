package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/fangdingjun/gnutls"
	auth "github.com/fangdingjun/go-http-auth"
	"github.com/fangdingjun/gofast"
	loghandler "github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

type logwriter struct {
	w io.Writer
	l *sync.Mutex
}

func (lw *logwriter) Write(buf []byte) (int, error) {
	lw.l.Lock()
	defer lw.l.Unlock()
	return lw.w.Write(buf)
}

func initRouters(cfg conf) {

	logout := os.Stdout
	if logfile != "" {
		fp, err := os.OpenFile(logfile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			log.Println(err)
		} else {
			logout = fp
		}
	}

	w := &logwriter{logout, new(sync.Mutex)}

	for _, l := range cfg {
		router := mux.NewRouter()
		domains := []string{}
		certs := []*gnutls.Certificate{}

		// initial virtual host
		for _, h := range l.Vhost {
			h2 := h.Hostname
			if h1, _, err := net.SplitHostPort(h.Hostname); err == nil {
				h2 = h1
			}
			domains = append(domains, h2)
			if h.Cert != "" && h.Key != "" {
				if cert, err := gnutls.LoadX509KeyPair(h.Cert, h.Key); err == nil {
					certs = append(certs, cert)
				} else {
					log.Fatal(err)
				}
			}
			r := router.Host(h2).Subrouter()
			for _, rule := range h.URLRules {
				switch rule.Type {
				case "alias":
					registerAliasHandler(rule, r)
				case "uwsgi":
					registerUwsgiHandler(rule, r)
				case "fastcgi":
					registerFastCGIHandler(rule, h.Docroot, r)
				case "reverse":
					registerHTTPHandler(rule, r)
				default:
					fmt.Printf("invalid type: %s\n", rule.Type)
				}
			}
			r.PathPrefix("/").Handler(http.FileServer(http.Dir(h.Docroot)))
		}

		// default host config
		for _, rule := range l.URLRules {
			switch rule.Type {
			case "alias":
				registerAliasHandler(rule, router)
			case "uwsgi":
				registerUwsgiHandler(rule, router)
			case "fastcgi":
				docroot := l.Docroot
				if rule.Docroot != "" {
					docroot = rule.Docroot
				}
				registerFastCGIHandler(rule, docroot, router)
			case "reverse":
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
				enableAuth:   l.EnableAuth,
				localDomains: domains,
			}

			if l.EnableAuth {
				if l.PasswdFile == "" {
					log.Fatal("passwdfile required")
				}
				du, err := newDigestSecret(l.PasswdFile)
				if err != nil {
					log.Fatal(err)
				}
				digestAuth := auth.NewDigestAuthenticator(l.Realm, du.getPw)
				digestAuth.Headers = auth.ProxyHeaders
				hdlr.authMethod = digestAuth
			}

			if len(certs) > 0 {
				tlsconfig := &gnutls.Config{
					Certificates: certs,
					NextProtos:   []string{"h2", "http/1.1"},
				}
				listener, err := gnutls.Listen("tcp", addr, tlsconfig)
				if err != nil {
					log.Fatal(err)
				}

				handler := loghandler.CombinedLoggingHandler(w, hdlr)
				//handler := hdlr
				log.Printf("listen https on %s", addr)
				go func() {
					defer listener.Close()
					for {
						conn, err := listener.Accept()
						if err != nil {
							log.Println(err)
							break
						}
						go handleHTTPClient(conn, handler)
					}
				}()

			} else {
				log.Printf("listen http on %s", addr)
				handler := loghandler.CombinedLoggingHandler(w, hdlr)
				//handler := hdlr
				if err := http.ListenAndServe(
					addr, handler,
				); err != nil {
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
	var n, p string
	switch r.Target.Type {
	case "unix":
		n = "unix"
		p = r.Target.Path
	case "tcp":
		n = "tcp"
		p = fmt.Sprintf("%s:%d", r.Target.Host, r.Target.Port)
	default:
		fmt.Printf("invalid scheme: %s, only support unix, tcp", r.Target.Type)
		os.Exit(-1)
	}

	u := gofast.NewHandler(gofast.NewPHPFS(docroot), n, p)
	if r.IsRegex {
		m1 := myURLMatch{regexp.MustCompile(r.URLPrefix)}
		router.MatcherFunc(m1.match).Handler(u)
	} else {
		router.PathPrefix(r.URLPrefix).Handler(u)
	}
}

func registerHTTPHandler(r rule, router *mux.Router) {
	var u http.Handler
	var addr string
	switch r.Target.Type {
	case "unix":
		addr = r.Target.Path
		u = newProxy(addr, r.URLPrefix)
	case "http":
		addr = fmt.Sprintf("%s:%d", r.Target.Host, r.Target.Port)
		u1 := &url.URL{
			Scheme: "http",
			Host:   addr,
			Path:   r.Target.Path,
		}
		u = httputil.NewSingleHostReverseProxy(u1)
	default:
		fmt.Printf("invalid scheme: %s, only support unix, http", r.Target.Type)
		os.Exit(-1)
	}
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
