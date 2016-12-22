package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

var defaultTransport http.RoundTripper = &http.Transport{
	DialContext:         dialContext,
	MaxIdleConns:        50,
	IdleConnTimeout:     30 * time.Second,
	MaxIdleConnsPerHost: 3,
	//ResponseHeaderTimeout: 2 * time.Second,
}

func main() {
	var docroot string
	var enableProxy bool
	var port int

	curdir, err := os.Getwd()
	if err != nil {
		curdir = "."
	}

	flag.StringVar(&docroot, "docroot", curdir, "document root")
	flag.BoolVar(&enableProxy, "enable_proxy", false, "enable proxy function")
	flag.IntVar(&port, "port", 8080, "the port listen to")
	flag.Parse()

	http.Handle("/", http.FileServer(http.Dir(docroot)))

	log.Printf("Listen on :%d", port)
	log.Printf("document root %s", docroot)
	if enableProxy {
		log.Println("proxy enabled")
	}
	err = http.ListenAndServe(fmt.Sprintf(":%d", port), &handler{
		enableProxy: enableProxy,
	})
	if err != nil {
		log.Fatal(err)
	}
}

type handler struct {
	enableProxy bool
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.RequestURI[0] == '/' {
		http.DefaultServeMux.ServeHTTP(w, r)
		return
	}

	if !h.enableProxy {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "<h1>page not found!</h1>")
		return
	}
	if r.Method == http.MethodConnect {
		h.handleCONNECT(w, r)
	} else {
		h.handleHTTP(w, r)
	}
}

func (h *handler) handleHTTP(w http.ResponseWriter, r *http.Request) {

	var resp *http.Response
	var err error

	r.Header.Del("proxy-connection")

	resp, err = defaultTransport.RoundTrip(r)
	if err != nil {
		log.Printf("RoundTrip: %s", err)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "%s", err)
		return
	}

	defer resp.Body.Close()

	hdr := w.Header()

	//resp.Header.Del("connection")

	for k, v := range resp.Header {
		for _, v1 := range v {
			hdr.Add(k, v1)
		}
	}

	w.WriteHeader(resp.StatusCode)

	io.Copy(w, resp.Body)
}

func (h *handler) handleCONNECT(w http.ResponseWriter, r *http.Request) {
	host := r.RequestURI
	if !strings.Contains(host, ":") {
		host = fmt.Sprintf("%s:443", host)
	}

	var conn net.Conn
	var err error

	conn, err = dial("tcp", host)
	if err != nil {
		log.Printf("net.dial: %s", err)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "dial to %s failed: %s", host, err)
		return
	}

	hj, _ := w.(http.Hijacker)
	conn1, _, _ := hj.Hijack()

	fmt.Fprintf(conn1, "%s 200 connection established\r\n\r\n", r.Proto)

	pipeAndClose(conn, conn1)
}

func pipeAndClose(r1, r2 io.ReadWriteCloser) {
	ch := make(chan int, 2)
	go func() {
		io.Copy(r1, r2)
		ch <- 1
	}()

	go func() {
		io.Copy(r2, r1)
		ch <- 1
	}()

	<-ch
}
