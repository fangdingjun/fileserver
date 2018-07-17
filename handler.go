package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/fangdingjun/gnutls"
	auth "github.com/fangdingjun/go-http-auth"
	nghttp2 "github.com/fangdingjun/nghttp2-go"
)

// handler process the proxy request first(if enabled)
// and route the request to the registered http.Handler
type handler struct {
	handler      http.Handler
	enableProxy  bool
	enableAuth   bool
	authMethod   *auth.DigestAuth
	localDomains []string
}

var defaultTransport http.RoundTripper = &http.Transport{
	//DialContext:         dialContext,
	MaxIdleConns:          50,
	IdleConnTimeout:       30 * time.Second,
	MaxIdleConnsPerHost:   3,
	DisableKeepAlives:     true,
	ResponseHeaderTimeout: 10 * time.Second,
}

// ServeHTTP implements the http.Handler interface
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// http/1.1 local request
	if r.ProtoMajor == 1 && r.RequestURI[0] == '/' {
		if h.handler != nil {
			h.handler.ServeHTTP(w, r)
		} else {
			http.DefaultServeMux.ServeHTTP(w, r)
		}
		return
	}

	// http/2.0 local request
	if r.ProtoMajor == 2 && h.isLocalRequest(r) {
		if h.handler != nil {
			h.handler.ServeHTTP(w, r)
		} else {
			http.DefaultServeMux.ServeHTTP(w, r)
		}
		return
	}

	// proxy request

	if !h.enableProxy {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "<h1>404 Not Found</h1>")
		return
	}

	if h.enableAuth {
		u, _ := h.authMethod.CheckAuth(r)
		if u == "" {
			h.authMethod.RequireAuth(w, r)
			return
		}
	}

	if r.Method == http.MethodConnect {
		// CONNECT request
		h.handleCONNECT(w, r)
	} else {
		// GET, POST, PUT, ....
		h.handleHTTP(w, r)
	}
}

func (h *handler) handleHTTP(w http.ResponseWriter, r *http.Request) {

	var resp *http.Response
	var err error

	r.Header.Del("proxy-connection")
	r.Header.Del("proxy-authorization")

	if r.ProtoMajor == 2 {
		r.URL.Scheme = "http"
		r.URL.Host = r.Host
		r.RequestURI = r.URL.String()
		if r.Method != http.MethodPost && r.Method != http.MethodPut {
			r.ContentLength = 0
			r.Body.Close()
			r.Body = nil
		}
	}
	if r.Method == http.MethodPost && r.ContentLength == 0 {
		r.Body = http.NoBody
	}
	//log.Println("content-length", r.Header.Get("content-length"))
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

	for k, v := range resp.Header {
		_k := strings.ToLower(k)
		if _k == "connection" || _k == "transfer-encoding" ||
			_k == "keep-alive" || _k == "upgrade" || _k == "te" {
			continue
		}
		if resp.StatusCode == 204 && _k == "content-length" {
			continue
		}
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
		host = r.Host
	}

	if !strings.Contains(host, ":") {
		host = fmt.Sprintf("%s:443", host)
	}

	var conn net.Conn
	var err error

	conn, err = net.Dial("tcp", host)
	if err != nil {
		log.Printf("net.dial: %s", err)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "dial to %s failed: %s", host, err)
		return
	}

	if r.ProtoMajor == 1 {
		// HTTP/1.1
		hj, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		conn1, _, _ := hj.Hijack()

		fmt.Fprintf(conn1, "%s 200 connection established\r\n\r\n", r.Proto)

		pipeAndClose(conn, conn1)
		return
	}

	// HTTP/2.0

	defer func() {
		if err := recover(); err != nil {
			log.Printf("recover %+v", err)
		}
	}()

	defer conn.Close()

	w.WriteHeader(http.StatusOK)
	//w.(http.Flusher).Flush()

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

// isLocalRequest determine the http2 request is local path request
// or the proxy request
func (h *handler) isLocalRequest(r *http.Request) bool {
	if !h.enableProxy {
		return true
	}

	if len(h.localDomains) == 0 {
		return true
	}

	host := r.Host
	if h1, _, err := net.SplitHostPort(r.Host); err == nil {
		host = h1
	}

	for _, s := range h.localDomains {
		if strings.HasSuffix(host, s) {
			return true
		}
	}

	return false
}

func pipeAndClose(r1, r2 io.ReadWriteCloser) {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("recover %+v", err)
		}
	}()

	defer r1.Close()
	defer r2.Close()

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

func handleHTTPClient(c net.Conn, handler http.Handler) {
	tlsconn := c.(*gnutls.Conn)
	if err := tlsconn.Handshake(); err != nil {
		log.Println(err)
		return
	}
	state := tlsconn.ConnectionState()
	if state.NegotiatedProtocol == "h2" {
		h2conn, err := nghttp2.Server(tlsconn, handler)
		if err != nil {
			log.Println(err)
		}
		h2conn.Run()
		return
	}

	defer c.Close()
	r := bufio.NewReader(tlsconn)
	buf := new(bytes.Buffer)
	for {
		req, err := http.ReadRequest(r)
		if err != nil {
			return
		}
		addr := tlsconn.RemoteAddr().String()
		req.RemoteAddr = addr
		rh := &responseHandler{
			c:      tlsconn,
			header: http.Header{},
			buf:    buf,
		}
		handler.ServeHTTP(rh, req)
		rh.Write(nil)
		rh.buf.WriteTo(rh.c)
	}
}

type responseHandler struct {
	c            net.Conn
	statusCode   int
	header       http.Header
	responseSend bool
	w            io.Writer
	buf          *bytes.Buffer
}

func (r *responseHandler) WriteHeader(statusCode int) {
	if r.responseSend {
		return
	}
	r.buf.Reset()
	r.statusCode = statusCode
	cl := r.header.Get("content-length")
	te := r.header.Get("transfer-encoding")
	if cl == "" || te != "" {
		if te == "" {
			r.header.Set("transfer-encoding", "chunked")
		}
		r.w = &chunkWriter{r.buf}
	} else {
		r.w = r.buf
	}
	fmt.Fprintf(r.buf, "HTTP/1.1 %d %s\r\n", statusCode,
		http.StatusText(statusCode))
	for k, v := range r.header {
		fmt.Fprintf(r.buf, "%s: %s\r\n", strings.Title(k), strings.Join(v, ","))
	}
	fmt.Fprintf(r.buf, "\r\n")
	r.responseSend = true
}

func (r *responseHandler) Header() http.Header {
	return r.header
}

func (r *responseHandler) Write(buf []byte) (int, error) {
	if !r.responseSend {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.w.Write(buf)
	if r.buf.Len() > 2048 {
		r.buf.WriteTo(r.c)
	}
	return n, err
}

var _ http.ResponseWriter = &responseHandler{}

type chunkWriter struct {
	w io.Writer
}

func (cw *chunkWriter) Write(buf []byte) (int, error) {
	n := len(buf)
	if n == 0 {
		return fmt.Fprintf(cw.w, "0\r\n\r\n")
	}
	return fmt.Fprintf(cw.w, "%x\r\n%s\r\n", n, string(buf))
}
