package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/fangdingjun/go-log"
	"github.com/fangdingjun/protolistener"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"golang.org/x/net/http2"
	"golang.org/x/net/trace"
)

func initServer(c *conf) error {

	mux := mux.NewRouter()

	for _, vh := range c.Vhosts {
		subroute := mux.Host(vh.Hostname)
		subroute.PathPrefix("/").Handler(http.FileServer(http.Dir(vh.Docroot)))
	}

	mux.PathPrefix("/debug/").Handler(http.DefaultServeMux)

	if len(c.Vhosts) > 0 {
		mux.PathPrefix("/").Handler(http.FileServer(http.Dir(c.Vhosts[0].Docroot)))
	} else {
		mux.PathPrefix("/").Handler(http.FileServer(http.Dir("/var/www/html")))
	}

	for _, _l := range c.Listens {
		var err error
		certs := []tls.Certificate{}
		tlsconfig := &tls.Config{}
		for _, cert := range _l.Certificates {
			if cert.CertFile != "" && cert.KeyFile != "" {
				_cert, err := tls.LoadX509KeyPair(cert.CertFile, cert.KeyFile)
				if err != nil {
					return err
				}
				certs = append(certs, _cert)
			}
		}

		var h http.Handler

		h = &handler{
			handler: mux,
			cfg:     c,
			events:  trace.NewEventLog("http", fmt.Sprintf("%s:%d", _l.Addr, _l.Port)),
		}
		h = handlers.CombinedLoggingHandler(&logout{}, h)

		srv := &http.Server{
			Addr:    fmt.Sprintf("%s:%d", _l.Addr, _l.Port),
			Handler: h,
		}

		var l net.Listener

		l, err = net.Listen("tcp", srv.Addr)
		if err != nil {
			return err
		}

		l = protolistener.New(l)

		if len(certs) > 0 {
			tlsconfig.Certificates = certs
			tlsconfig.BuildNameToCertificate()
			srv.TLSConfig = tlsconfig
			http2.ConfigureServer(srv, nil)
			l = tls.NewListener(l, srv.TLSConfig)
		}

		go func(l net.Listener) {
			defer l.Close()
			err = srv.Serve(l)
			if err != nil {
				log.Errorln(err)
			}
		}(l)
	}
	return nil
}

type logout struct{}

func (l *logout) Write(buf []byte) (int, error) {
	log.Debugf("%s", buf)
	return len(buf), nil
}

func main() {
	var configfile string
	var loglevel string
	var logfile string
	var logFileCount int
	var logFileSize int64
	flag.StringVar(&logfile, "log_file", "", "log file, default stdout")
	flag.IntVar(&logFileCount, "log_count", 10, "max count of log to keep")
	flag.Int64Var(&logFileSize, "log_size", 10, "max log file size MB")
	flag.StringVar(&loglevel, "log_level", "INFO",
		"log level, values:\nOFF, FATAL, PANIC, ERROR, WARN, INFO, DEBUG")
	flag.StringVar(&configfile, "c", "config.yaml", "config file")
	flag.Parse()

	if logfile != "" {
		log.Default.Out = &log.FixedSizeFileWriter{
			MaxCount: logFileCount,
			Name:     logfile,
			MaxSize:  logFileSize * 1024 * 1024,
		}
	}

	if loglevel != "" {
		lv, err := log.ParseLevel(loglevel)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		log.Default.Level = lv
	}

	c, err := loadConfig(configfile)
	if err != nil {
		log.Fatal(err)
	}

	log.Infof("%+v", c)
	err = initServer(c)
	if err != nil {
		log.Fatalln(err)
	}

	trace.AuthRequest = func(r *http.Request) (bool, bool) {
		return true, true
	}

	ch := make(chan os.Signal, 2)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-ch:
		log.Errorf("received signal %s, exit", sig)
	}
	log.Debug("exited.")
}
