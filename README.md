gserver
=========

gserver is a golang http/https server

features
=======

- support HTTP/1.1, HTTP/2.0
- support serve static files
- support UWSGI client protocol (python)
- support fastCGI client protocol (php)
- support act as resverse proxy
- support act as forward proxy
- support multiple locations

usage
====

    go get github.com/fangdingjun/gserver
    cp $GOPATH/src/github.com/fangdingjun/gserver/config_example.yaml config.yaml
    vim config.yaml
    $GOPATH/bin/gserver -c config.yaml

