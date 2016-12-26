// +build unix

package main

import (
	"fmt"
	"github.com/miekg/dns"
	"log"
	"time"
)

var clientConfig *dns.ClientConfig
var dnsClient *dns.Client

func lookupHost(host string) ([]string, error) {
	var result = []string{}
	var err error

	ret, err1 := getAAAA(host)
	if err1 == nil {
		result = append(result, ret...)
	} else {
		err = err1
	}

	ret1, err2 := getA(host)
	if err2 == nil {
		result = append(result, ret1...)
	} else {
		err = err2
	}

	if len(result) > 0 {
		return result, nil
	}

	if err == nil {
		return nil, fmt.Errorf("dns lookup failed for %s", host)
	}

	return nil, err
}

func getA(host string) ([]string, error) {
	var err error
	var msg *dns.Msg
	var result = []string{}

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(host), dns.TypeA)

	for _, srv := range clientConfig.Servers {
		dnsserver := fmt.Sprintf("%s:%s", srv, clientConfig.Port)
		msg, _, err = dnsClient.Exchange(m, dnsserver)
		if err == nil {
			break
		} else {
			log.Println(err)
		}
	}

	if err != nil {
		return result, err
	}

	for _, rr := range msg.Answer {
		if a, ok := rr.(*dns.A); ok {
			result = append(result, a.A.String())
		}

	}

	return result, nil
}

func getAAAA(host string) ([]string, error) {
	var err error
	var msg *dns.Msg
	var result = []string{}

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(host), dns.TypeAAAA)

	for _, srv := range clientConfig.Servers {
		dnsserver := fmt.Sprintf("%s:%s", srv, clientConfig.Port)

		msg, _, err = dnsClient.Exchange(m, dnsserver)
		if err == nil {
			break
		} else {
			log.Println(err)
		}
	}
	if err != nil {
		return result, err
	}

	for _, rr := range msg.Answer {
		if aaaa, ok := rr.(*dns.AAAA); ok {
			result = append(result, aaaa.AAAA.String())
		}

	}

	return result, nil
}

func init() {
	var err error
	clientConfig, err = dns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil {
		clientConfig = &dns.ClientConfig{
			Servers:  []string{"8.8.8.8", "4.2.2.2"},
			Port:     "53",
			Ndots:    1,
			Timeout:  2,
			Attempts: 3,
		}
	}

	//clientConfig.Port = "53"
	dnsClient = &dns.Client{
		Net:     "udp",
		Timeout: time.Duration(clientConfig.Timeout) * time.Second,
		UDPSize: 4096,
	}
}
