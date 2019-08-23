package main

/*
this is a example of using http2 proxy
this applet act http proxy and forward the request through http2 proxy

usage example

	go build -o proxy http2_proxy.go
	./proxy -server www.example.com -listen :8088
	curl --proxy http://localhost:8088/ https://httpbin.org/ip

*/

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"sync"
	"time"

	log "github.com/fangdingjun/go-log"
	"golang.org/x/net/http2"
)

type clientConn struct {
	host      string
	port      string
	hostname  string
	transport *http2.Transport
	conn      *http2.ClientConn
	lock      *sync.Mutex
}

type timeoutConn struct {
	net.Conn
	timeout time.Duration
}

func (tc *timeoutConn) Read(b []byte) (n int, err error) {
	if err = tc.Conn.SetReadDeadline(time.Now().Add(tc.timeout)); err != nil {
		return 0, err
	}
	return tc.Conn.Read(b)
}

func (tc *timeoutConn) Write(b []byte) (n int, err error) {
	if err = tc.Conn.SetWriteDeadline(time.Now().Add(tc.timeout)); err != nil {
		return 0, err
	}
	return tc.Conn.Write(b)
}

type handler struct {
	transport *http2.Transport
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if debug {
		req, _ := httputil.DumpRequest(r, false)
		log.Debugf("%s", string(req))
	}

	if r.Method == http.MethodConnect {
		h.handleConnect(w, r)
	} else {
		h.handleHTTP(w, r)
	}
}

func (h *handler) handleConnect(w http.ResponseWriter, r *http.Request) {
	pr, pw := io.Pipe()

	defer pr.Close()
	defer pw.Close()

	r.Body = ioutil.NopCloser(pr)
	r.URL.Scheme = "https"

	r.Header.Del("proxy-connection")

	resp, err := h.transport.RoundTrip(r)
	if err != nil {
		log.Errorf("roundtrip: %s", err)
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	defer resp.Body.Close()

	if debug {
		d, _ := httputil.DumpResponse(resp, false)
		log.Debugf("%s", string(d))
	}

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
		resp.Body.Close()
		log.Infof("%s %s %d %s %s", r.Method, r.RequestURI, resp.StatusCode, r.Proto, r.UserAgent())
		return
	}

	c, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		log.Errorf("hijack: %s", err)
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		log.Infof("%s %s %d %s %s", r.Method, r.RequestURI, 500, r.Proto, r.UserAgent())
		return
	}

	defer c.Close()
	fmt.Fprintf(c, "%s 200 connection established\r\n\r\n", r.Proto)

	ch := make(chan struct{}, 2)

	go func() {
		io.Copy(pw, c)
		ch <- struct{}{}
	}()

	go func() {
		io.Copy(c, resp.Body)
		ch <- struct{}{}
	}()

	<-ch
	log.Infof("%s %s %d %s %s", r.Method, r.RequestURI, 200, r.Proto, r.UserAgent())
}

func (h *handler) handleHTTP(w http.ResponseWriter, r *http.Request) {
	resp, err := h.transport.RoundTrip(r)
	if err != nil {
		log.Errorln(err)
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	if debug {
		d, _ := httputil.DumpResponse(resp, false)
		log.Debugf("%s", string(d))
	}

	hdr := w.Header()
	for k, v := range resp.Header {
		for _, v1 := range v {
			hdr.Add(k, v1)
		}
	}

	w.WriteHeader(resp.StatusCode)
	n, _ := io.Copy(w, resp.Body)
	log.Infof("%s %s %d %s %d %s", r.Method, r.RequestURI, resp.StatusCode, r.Proto, n, r.UserAgent())
}

func newClientConn(host string, port string, hostname string, t *http2.Transport) *clientConn {
	cc := &clientConn{
		host:      host,
		port:      port,
		hostname:  hostname,
		transport: t,
		lock:      new(sync.Mutex),
	}
	go cc.ping()
	return cc
}

func (p *clientConn) ping() {
	for {
		select {
		case <-time.After(time.Duration(idleTimeout-5) * time.Second):
		}

		p.lock.Lock()
		conn := p.conn
		p.lock.Unlock()

		if conn == nil {
			continue
		}
		if err := conn.Ping(context.Background()); err != nil {
			p.MarkDead(conn)
		}
	}
}

func (p *clientConn) GetClientConn(req *http.Request, addr string) (*http2.ClientConn, error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	if p.conn != nil && p.conn.CanTakeNewRequest() {
		return p.conn, nil
	}

	if p.conn != nil {
		p.conn.Close()
		p.conn = nil
	}

	log.Infof("dial to %s:%s", p.host, p.port)

	c, err := net.Dial("tcp", net.JoinHostPort(p.host, p.port))
	if err != nil {
		log.Errorln(err)
		return nil, err
	}

	cc := &timeoutConn{c, time.Duration(idleTimeout) * time.Second}
	// cc := c
	config := &tls.Config{
		ServerName:         p.hostname,
		NextProtos:         []string{"h2"},
		InsecureSkipVerify: insecure,
	}

	conn := tls.Client(cc, config)
	if err := conn.Handshake(); err != nil {
		log.Errorln(err)
		return nil, err
	}

	http2conn, err := p.transport.NewClientConn(conn)
	if err != nil {
		conn.Close()
		log.Errorln(err)
		return nil, err
	}

	p.conn = http2conn

	return http2conn, err
}

func (p *clientConn) MarkDead(conn *http2.ClientConn) {
	p.lock.Lock()
	defer p.lock.Unlock()

	if p.conn != nil {
		log.Errorln("mark dead")
		p.conn.Close()
		p.conn = nil
	}
}

var debug bool
var insecure bool
var idleTimeout int

func main() {
	var addr string
	var hostname string
	var listen string
	var logfile string

	flag.StringVar(&addr, "server", "", "server address")
	flag.StringVar(&hostname, "name", "", "server 's SNI name")
	flag.StringVar(&listen, "listen", ":8080", "listen address")
	flag.BoolVar(&debug, "debug", false, "verbose mode")
	flag.BoolVar(&insecure, "insecure", false, "insecure mode, not verify the server's certificate")
	flag.IntVar(&idleTimeout, "idletime", 20, "idle timeout, close connection when no data transfer")
	flag.StringVar(&logfile, "log_file", "", "log file")
	flag.Parse()

	if addr == "" {
		fmt.Println("please specify the server address")
		os.Exit(-1)
	}

	if idleTimeout < 10 {
		idleTimeout = 10
	}

	if logfile != "" {
		log.Default.Out = &log.FixedSizeFileWriter{
			MaxCount: 4,
			Name:     logfile,
			MaxSize:  10 * 1024 * 1024,
		}
	}

	if debug {
		log.Default.Level = log.DEBUG
	} else {
		log.Default.Level = log.INFO
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
		port = "443"
	}

	if hostname == "" {
		hostname = host
	}

	transport := &http2.Transport{
		AllowHTTP: true,
	}

	p := newClientConn(host, port, hostname, transport)
	transport.ConnPool = p

	log.Printf("listen on %s", listen)

	log.Printf("use parent proxy https://%s:%s/", host, port)
	log.Printf("server SNI name %s", hostname)

	if err := http.ListenAndServe(listen, &handler{transport}); err != nil {
		log.Fatal(err)
	}
}
