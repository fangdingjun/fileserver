package main

import (
	"context"
	"net"
	"time"
)

var dialer *net.Dialer

func dial(network, addr string) (net.Conn, error) {
	var err error

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	if ip := net.ParseIP(host); ip != nil {
		return dialer.Dial(network, addr)
	}

	ips, err := lookupHost(host)
	if err != nil {
		return nil, err
	}

	var conn net.Conn

	for _, ip := range ips {
		address := net.JoinHostPort(ip, port)
		if conn, err = dialer.Dial(network, address); err == nil {
			return conn, err
		}
	}

	// return last error
	return conn, err
}

func dialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	var err error

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	if ip := net.ParseIP(host); ip != nil {
		return dialer.DialContext(ctx, network, addr)
	}

	ips, err := lookupHost(host)
	if err != nil {
		return nil, err
	}

	var conn net.Conn

	for _, ip := range ips {
		address := net.JoinHostPort(ip, port)
		if conn, err = dialer.DialContext(ctx, network, address); err == nil {
			return conn, err
		}
	}

	// return last error
	return conn, err
}

func init() {
	dialer = &net.Dialer{
		Timeout: 2 * time.Second,
	}
}
