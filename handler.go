package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

type handler struct {
	enableProxy bool
}

var defaultTransport http.RoundTripper = &http.Transport{
	DialContext:         dialContext,
	MaxIdleConns:        50,
	IdleConnTimeout:     30 * time.Second,
	MaxIdleConnsPerHost: 3,
	//ResponseHeaderTimeout: 2 * time.Second,
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

	if r.ProtoMajor == 2 {
		r.URL.Scheme = "http"
		r.URL.Host = r.Host
		r.RequestURI = r.URL.String()
	}

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

	resp.Header.Del("connection")

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

	if r.ProtoMajor == 2 {
		host = r.URL.Host
	}

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

	if r.ProtoMajor == 1 {
		// HTTP/1.1
		hj, _ := w.(http.Hijacker)
		conn1, _, _ := hj.Hijack()

		fmt.Fprintf(conn1, "%s 200 connection established\r\n\r\n", r.Proto)

		pipeAndClose(conn, conn1)
		return
	}

	// HTTP/2.0
	defer conn.Close()

	w.WriteHeader(http.StatusOK)
	w.(http.Flusher).Flush()

	ch := make(chan int, 2)
	go func() {
		io.Copy(conn, r.Body)
		ch <- 1
	}()

	go func() {
		io.Copy(w, conn)
		ch <- 1
	}()

	<-ch
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
