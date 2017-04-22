package main

import (
	"bufio"
	"os"
	"strings"
	"sync"
	"time"
)

type digestPwFile struct {
	path  string
	entry []pwEntry
	mtime time.Time
	mu    *sync.Mutex
}

type pwEntry struct {
	user   string
	realm  string
	hashPw string
}

func newDigestSecret(f string) (*digestPwFile, error) {
	a := &digestPwFile{path: f, mu: new(sync.Mutex)}
	if err := a.loadFile(); err != nil {
		return nil, err
	}
	go a.tryReload()
	return a, nil
}

func (df *digestPwFile) tryReload() {
	for {
		time.Sleep(10 * time.Second)
		fi, _ := os.Stat(df.path)
		t1 := fi.ModTime()
		if t1 != df.mtime {
			df.loadFile()
		}
	}
}

func (df *digestPwFile) loadFile() error {
	df.mu.Lock()
	defer df.mu.Unlock()

	fp, err := os.Open(df.path)
	if err != nil {
		return err
	}

	defer fp.Close()

	entry := []pwEntry{}

	r := bufio.NewReader(fp)
	for {
		line, err := r.ReadString('\n')

		if err != nil {
			break
		}

		line1 := strings.Trim(line, " \r\n")
		if line1 == "" || line1[0] == '#' {
			continue
		}
		fields := strings.SplitN(line1, ":", 3)
		entry = append(entry, pwEntry{fields[0], fields[1], fields[2]})
	}

	df.entry = entry
	fi, _ := os.Stat(df.path)
	df.mtime = fi.ModTime()

	return nil
}

func (df *digestPwFile) getPw(user, realm string) string {
	df.mu.Lock()
	defer df.mu.Unlock()

	for i := range df.entry {
		if df.entry[i].user == user && df.entry[i].realm == realm {
			return df.entry[i].hashPw
		}
	}
	return ""
}
