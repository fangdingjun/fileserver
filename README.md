fileserver
=========

fileserver is a golang http server which serves the  static  files and supports http forward proxy


usage
====

    go get github.com/fangdingjun/fileserver
    $GOPATH/bin/fileserver -port 8080 -docroot $HOME/public_html -enable_proxy

