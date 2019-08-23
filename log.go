package main

import (
	"github.com/fangdingjun/go-log"
)

func infoLog(msg string) {
	log.Println(msg)
}

func errorLog(msg string) {
	log.Errorln(msg)
}

func debugLog(msg string) {
	log.Debugln(msg)
}
