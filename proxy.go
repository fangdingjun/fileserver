package main

import (
	"context"
	"net"
	"net/http"
	//"bufio"
	//"fmt"
	"io"
	"log"
	//"strings"
	"time"
)

type proxy struct {
	transport http.RoundTripper
	addr      string
	prefix    string
	dialer    *net.Dialer
}

func newProxy(addr string, prefix string) *proxy {
	p := &proxy{
		addr:   addr,
		prefix: prefix,
		dialer: &net.Dialer{Timeout: 2 * time.Second},
	}
	p.transport = &http.Transport{
		DialContext:     p.dialContext,
		MaxIdleConns:    5,
		IdleConnTimeout: 30 * time.Second,
		//Dial: p.dial,
	}
	return p
}

func (p *proxy) dialContext(ctx context.Context,
	network, addr string) (net.Conn, error) {
	return p.dialer.DialContext(ctx, network, p.addr)
}

func (p *proxy) dial(network, addr string) (conn net.Conn, err error) {
	return p.dialer.Dial(network, p.addr)
}

func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	r.Header.Add("X-Forwarded-For", host)
	r.URL.Scheme = "http"
	r.URL.Host = r.Host
	resp, err := p.transport.RoundTrip(r)
	if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("<h1>502 Bad Gateway</h1>"))
		return
	}
	header := w.Header()
	for k, v := range resp.Header {
		for _, v1 := range v {
			header.Add(k, v1)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
	resp.Body.Close()
}
