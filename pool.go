package main

import (
	"net/http"
	"sync"

	gluahttp "github.com/cjoudrey/gluahttp"
	lua "github.com/yuin/gopher-lua"
	luajson "layeh.com/gopher-json"
	luar "layeh.com/gopher-luar"
	mysql "github.com/tengattack/gluasql/mysql"
	redis "github.com/fangdingjun/gopher-redis"
)

// from https://github.com/yuin/gophoer-lua

type lStatePool struct {
	m     sync.Mutex
	saved []*lua.LState
}

func (pl *lStatePool) Get() *lua.LState {
	pl.m.Lock()
	defer pl.m.Unlock()
	n := len(pl.saved)
	if n == 0 {
		return pl.New()
	}
	x := pl.saved[n-1]
	pl.saved = pl.saved[0 : n-1]
	return x
}

func (pl *lStatePool) New() *lua.LState {
	L := lua.NewState()

	luajson.Preload(L)
	L.PreloadModule("http", gluahttp.NewHttpModule(&http.Client{}).Loader)
	L.PreloadModule("mysql", mysql.Loader)
	L.PreloadModule("redis", redis.Loader)

	L.SetGlobal("infolog", luar.New(L, infoLog))
	L.SetGlobal("errorlog", luar.New(L, errorLog))
	L.SetGlobal("debuglog", luar.New(L, debugLog))

	// setting the L up here.
	// load scripts, set global variables, share channels, etc...
	return L
}

func (pl *lStatePool) Put(L *lua.LState) {
	pl.m.Lock()
	defer pl.m.Unlock()
	pl.saved = append(pl.saved, L)
}

func (pl *lStatePool) Shutdown() {
	pl.m.Lock()
	defer pl.m.Unlock()
	for _, L := range pl.saved {
		L.Close()
	}
}

// Global LState pool
var luaPool = &lStatePool{
	saved: make([]*lua.LState, 0, 4),
}
