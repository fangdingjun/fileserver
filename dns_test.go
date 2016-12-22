package main

import (
	"fmt"
	//"net"
	"testing"
)

func TestLookuphost(t *testing.T) {
	for _, h := range []string{"www.ifeng.com", "www.taobao.com",
		"www.baidu.com", "www.sina.com.cn", "www.163.com", "www.qq.com",
		"www.google.com", "www.facebook.com", "twitter.com",
	} {

		ret, err := lookupHost(h)
		if err != nil {
			t.Errorf("%s: %s", h, err)
		}
		fmt.Printf("%#v\n", ret)
	}
}
