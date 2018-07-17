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
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"sync"
	"time"

	"github.com/fangdingjun/gnutls"
	"github.com/fangdingjun/nghttp2-go"
)

type timeoutConn struct {
	net.Conn
	timeout time.Duration
}

func (tc *timeoutConn) Read(b []byte) (n int, err error) {
	if err = tc.Conn.SetReadDeadline(time.Now().Add(tc.timeout)); err != nil {
		return 0, err
	}
	n, err = tc.Conn.Read(b)
	//log.Printf("read %d bytes from network", n)
	return
}

func (tc *timeoutConn) Write(b []byte) (n int, err error) {
	if err = tc.Conn.SetWriteDeadline(time.Now().Add(tc.timeout)); err != nil {
		return 0, err
	}
	n, err = tc.Conn.Write(b)
	//log.Printf("write %d bytes to network", n)
	return
}

type handler struct {
	h2conn   *nghttp2.Conn
	addr     string
	hostname string
	insecure bool
	lock     *sync.Mutex
}

func (h *handler) createConnection() (*nghttp2.Conn, error) {
	log.Println("create connection to ", h.addr)
	c, err := net.DialTimeout("tcp", h.addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	conn, err := gnutls.Client(
		&timeoutConn{c, 20 * time.Second},
		&gnutls.Config{
			ServerName:         h.hostname,
			InsecureSkipVerify: h.insecure,
			NextProtos:         []string{"h2"},
		})
	if err != nil {
		return nil, err
	}
	if err := conn.Handshake(); err != nil {
		return nil, err
	}
	client, err := nghttp2.Client(conn)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (h *handler) getConn() (*nghttp2.Conn, error) {
	h.lock.Lock()
	defer h.lock.Unlock()

	if h.h2conn != nil {
		if h.h2conn.CanTakeNewRequest() {
			return h.h2conn, nil
		}
		h.h2conn.Close()
	}

	for i := 0; i < 2; i++ {
		h2conn, err := h.createConnection()
		if err == nil {
			h.h2conn = h2conn
			return h2conn, nil
		}
	}
	return nil, fmt.Errorf("create conn failed")
}

func (h *handler) checkError() {
	h.lock.Lock()
	defer h.lock.Unlock()

	if h.h2conn == nil {
		return
	}

	if err := h.h2conn.Error(); err != nil {
		//log.Println("connection has error ", err)
		h.h2conn.Close()
		h.h2conn = nil
	}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	if r.Method == http.MethodConnect {
		h.handleConnect(w, r)
	} else {
		h.handleHTTP(w, r)
	}
}

func (h *handler) handleConnect(w http.ResponseWriter, r *http.Request) {
	var err error
	var h2conn *nghttp2.Conn
	var code int
	//var resp *http.Response

	var cs net.Conn

	for i := 0; i < 2; i++ {
		h2conn, err = h.getConn()
		if err != nil {
			log.Println("connection error ", err)
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		cs, code, err = h2conn.Connect(r.RequestURI)
		if cs != nil {
			break
		}
		h.checkError()
	}

	if err != nil || cs == nil {
		log.Println("send connect error ", err)
		h.checkError()
		w.WriteHeader(http.StatusBadGateway)
		return
	}

	defer cs.Close()
	if code != http.StatusOK {
		log.Println("code", code)
		w.WriteHeader(code)
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
		io.Copy(cs, c)
		ch <- struct{}{}
	}()

	go func() {
		io.Copy(c, cs)
		ch <- struct{}{}
	}()

	<-ch
}

func (h *handler) handleHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp *http.Response
	var h2conn *nghttp2.Conn

	if r.RequestURI[0] == '/' {
		http.DefaultServeMux.ServeHTTP(w, r)
		return
	}

	for i := 0; i < 2; i++ {
		h2conn, err = h.getConn()
		if err != nil {
			//log.Println("create connection ", err)
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		resp, err = h2conn.RoundTrip(r)
		if resp != nil {
			break
		}
		h.checkError()
	}

	if err != nil || resp == nil {
		log.Println("create request error ", err)
		h.checkError()
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "%s", err)
		return
	}

	defer func() {
		if resp.Body != nil {
			resp.Body.Close()
		}
	}()

	hdr := w.Header()
	for k, v := range resp.Header {
		for _, v1 := range v {
			hdr.Add(k, v1)
		}
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

var insecure bool

func main() {
	var addr string
	var hostname string
	var listen string
	flag.StringVar(&addr, "server", "", "server address")
	flag.StringVar(&hostname, "name", "", "server 's SNI name")
	flag.StringVar(&listen, "listen", ":8080", "listen address")
	flag.BoolVar(&insecure, "insecure", false, "insecure mode, not verify the server's certificate")
	flag.Parse()

	if addr == "" {
		fmt.Println("please specify the server address")
		os.Exit(-1)
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
		addr = fmt.Sprintf("%s:443", addr)
	}

	if hostname == "" {
		hostname = host
	}

	log.Printf("listen on %s", listen)

	hdr := &handler{
		addr:     addr,
		hostname: hostname,
		insecure: insecure,
		lock:     new(sync.Mutex),
	}
	if err := http.ListenAndServe(listen, hdr); err != nil {
		log.Fatal(err)
	}
}
