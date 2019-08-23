package main

import (
	"bufio"
	"os"
	"sync"

	"github.com/fangdingjun/go-log"
	lua "github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"
)

// from https://github.com/yuin/gophoer-lua

// CompileLua reads the passed lua file from disk and compiles it.
func CompileLua(filePath string) (*lua.FunctionProto, error) {
	file, err := os.Open(filePath)
	defer file.Close()
	if err != nil {
		return nil, err
	}
	reader := bufio.NewReader(file)
	chunk, err := parse.Parse(reader, filePath)
	if err != nil {
		return nil, err
	}
	proto, err := lua.Compile(chunk, filePath)
	if err != nil {
		return nil, err
	}
	return proto, nil
}

// DoCompiledFile takes a FunctionProto, as returned by CompileLua, and runs it in the LState. It is equivalent
// to calling DoFile on the LState with the original source file.
func DoCompiledFile(L *lua.LState, proto *lua.FunctionProto) error {
	lfunc := L.NewFunctionFromProto(proto)
	L.Push(lfunc)
	return L.PCall(0, lua.MultRet, nil)
}

type filecache struct {
	filepath string
	proto    *lua.FunctionProto
	lastMod  int64
}

var cache = map[string]*filecache{}
var mu = new(sync.Mutex)

func runFile(vm *lua.LState, filepath string) error {
	mu.Lock()
	c, ok := cache[filepath]
	if !ok {
		c = &filecache{
			filepath: filepath,
			lastMod:  0,
		}
		cache[filepath] = c
	}

	fi, err := os.Stat(filepath)
	if err != nil {
		mu.Unlock()
		return err
	}

	t := fi.ModTime().Unix()
	if t > c.lastMod {
		log.Debugf("file %s changed, reload", filepath)
		proto, err := CompileLua(filepath)
		if err != nil {
			mu.Unlock()
			return err
		}
		c.proto = proto
		c.lastMod = t
	}
	mu.Unlock()
	// log.Println(c.proto)
	return DoCompiledFile(vm, c.proto)
}
