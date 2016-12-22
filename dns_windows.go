package main

import (
	"net"
)

func lookupHost(host string) ([]string, error) {
	return net.LookupHost(host)
}
