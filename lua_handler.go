package main

import (
	"io/ioutil"
	"net"
	"net/http"

	"github.com/fangdingjun/go-log"
	luar "layeh.com/gopher-luar"
)

type luaHandler struct {
	scriptFile string
}

func (l *luaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	vm := luaPool.Get()
	defer luaPool.Put(vm)

	vm.SetGlobal("request", luar.New(vm, &req{r}))
	vm.SetGlobal("response", luar.New(vm, w))

	if err := runFile(vm, l.scriptFile); err != nil {
		log.Errorln(err)
		http.Error(w, "server error", http.StatusInternalServerError)
	}
}

type req struct {
	*http.Request
}

func (r1 *req) GetBody() (string, error) {
	d, err := ioutil.ReadAll(r1.Body)
	return string(d), err
}

func (r1 *req) GetIP() string {
	ip := r1.Header.Get("x-real-ip")
	if ip != "" {
		return ip
	}
	ip = r1.Header.Get("x-forwarded-for")
	if ip != "" {
		return ip
	}
	host, _, _ := net.SplitHostPort(r1.RemoteAddr)
	return host
}
