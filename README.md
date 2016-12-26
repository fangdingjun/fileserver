gserver
=========

gserver is a golang http/https server

features
=======

- support UWSGI client protocol (python)
- support fastCGI client protocol (php)
- support act as resverse proxy
- support act as forward proxy
- support multiple virtual host
- support SNI (https virtual host)
- support http/2.0 (only on https)

usage
====

    go get github.com/fangdingjun/gserver
    cp $GOPATH/src/github.com/fangdingjun/gserver/config_example.yaml config.yaml
    vim config.yaml
    $GOPATH/bin/gserver -c config.yaml

