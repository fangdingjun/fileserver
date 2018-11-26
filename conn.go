package main

import (
	"bufio"
	"net"

	proxyproto "github.com/pires/go-proxyproto"
)

type listener struct {
	net.Listener
}

type conn struct {
	net.Conn
	headerDone bool
	r          *bufio.Reader
	proxy      *proxyproto.Header
}

func (l *listener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &conn{Conn: c}, err
}

func (c *conn) Read(buf []byte) (int, error) {
	var err error
	if !c.headerDone {
		c.r = bufio.NewReader(c.Conn)
		c.proxy, err = proxyproto.Read(c.r)
		if err != nil && err != proxyproto.ErrNoProxyProtocol {
			return 0, err
		}
		c.headerDone = true
		return c.r.Read(buf)
	}
	return c.r.Read(buf)
}

func (c *conn) RemoteAddr() net.Addr {
	if c.proxy == nil {
		return c.Conn.RemoteAddr()
	}
	return &net.TCPAddr{
		IP:   c.proxy.SourceAddress,
		Port: int(c.proxy.SourcePort)}
}
