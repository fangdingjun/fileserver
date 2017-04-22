// +build ignore

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
	"crypto/tls"
	"flag"
	"fmt"
	"golang.org/x/net/http2"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"sync"
	"time"
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
		log.Printf("%s", string(req))
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
		log.Printf("roundtrip: %s", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "%s", err)
		return
	}

	defer resp.Body.Close()

	if debug {
		d, _ := httputil.DumpResponse(resp, false)
		log.Printf("%s", string(d))
	}

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		return
	}

	c, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		log.Println("hijack: %s", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "%s", err)
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
}

func (h *handler) handleHTTP(w http.ResponseWriter, r *http.Request) {
	resp, err := h.transport.RoundTrip(r)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "%s", err)
		return
	}
	defer resp.Body.Close()

	if debug {
		d, _ := httputil.DumpResponse(resp, false)
		log.Printf("%s", string(d))
	}

	hdr := w.Header()
	for k, v := range resp.Header {
		for _, v1 := range v {
			hdr.Add(k, v1)
		}
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func newClientConn(host string, port string, hostname string, t *http2.Transport) *clientConn {
	return &clientConn{
		host:      host,
		port:      port,
		hostname:  hostname,
		transport: t,
		lock:      new(sync.Mutex),
	}
}

func (p *clientConn) GetClientConn(req *http.Request, addr string) (*http2.ClientConn, error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	if p.conn != nil && p.conn.CanTakeNewRequest() {
		return p.conn, nil
	}

	if debug {
		log.Printf("dial to %s:%s", p.host, p.port)
	}

	c, err := net.Dial("tcp", net.JoinHostPort(p.host, p.port))
	if err != nil {
		log.Println(err)
		return nil, err
	}

	cc := &timeoutConn{c, time.Duration(idleTimeout) * time.Second}
	config := &tls.Config{
		ServerName:         p.hostname,
		NextProtos:         []string{"h2"},
		InsecureSkipVerify: insecure,
	}

	conn := tls.Client(cc, config)
	if err := conn.Handshake(); err != nil {
		log.Println(err)
		return nil, err
	}

	http2conn, err := p.transport.NewClientConn(conn)
	if err != nil {
		conn.Close()
		log.Println(err)
		return nil, err
	}

	p.conn = http2conn

	return http2conn, err
}

func (p *clientConn) MarkDead(conn *http2.ClientConn) {
	//p.lock.Lock()
	//defer p.lock.Unlock()

	if debug {
		log.Println("mark dead")
	}

	//p.conn = nil
}

var debug bool
var insecure bool
var idleTimeout int

func main() {
	var addr string
	var hostname string
	var listen string
	flag.StringVar(&addr, "server", "", "server address")
	flag.StringVar(&hostname, "name", "", "server 's SNI name")
	flag.StringVar(&listen, "listen", ":8080", "listen address")
	flag.BoolVar(&debug, "debug", false, "verbose mode")
	flag.BoolVar(&insecure, "insecure", false, "insecure mode, not verify the server's certificate")
	flag.IntVar(&idleTimeout, "idletime", 30, "idle timeout, close connection when no data transfer")
	flag.Parse()

	if addr == "" {
		fmt.Println("please specify the server address")
		os.Exit(-1)
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

	if debug {
		log.Printf("use parent proxy https://%s:%s/", host, port)
		log.Printf("server SNI name %s", hostname)
	}

	if err := http.ListenAndServe(listen, &handler{transport}); err != nil {
		log.Fatal(err)
	}
}
