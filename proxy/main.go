package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gobike/envflag"
	"golang.org/x/exp/slog"
)

var (
	addr         string = ":80"
	readTimeout  uint   = 30
	writeTimeout uint   = 30
	debug        bool
)

var defaultTransport = &http.Transport{
	DialContext: (&net.Dialer{
		Timeout:   20 * time.Second,
		KeepAlive: 30 * time.Second,
		DualStack: true,
	}).DialContext,
	MaxConnsPerHost:       4096,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          2048,
	MaxIdleConnsPerHost:   512,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

func proxy(rw http.ResponseWriter, r *http.Request) {
	slog.Info("request",
		slog.String("host", r.Host),
		slog.String("url", r.URL.String()),
		slog.String("uri", r.RequestURI),
	)
	scheme := "http"
	if strings.HasSuffix(r.RequestURI, ":443") {
		scheme = "https"
	}
	proxy := httputil.ReverseProxy{
		Transport: defaultTransport,
		Director: func(req *http.Request) {
			req.Host = r.Host
			req.URL = r.URL
			req.URL.Scheme = scheme
			req.Header.Set("User-Agent", r.UserAgent())
		},
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
			if errors.Is(err, context.Canceled) || (r.Method == http.MethodPut && errors.Is(err, io.ErrUnexpectedEOF)) {
				slog.Warn("proxy client error",
					slog.String("error", err.Error()),
				)
			} else {
				slog.Warn("proxy server error",
					slog.String("error", err.Error()),
				)
			}
			rw.WriteHeader(http.StatusBadGateway)
		},
		ModifyResponse: func(resp *http.Response) error {
			// data, err := httputil.DumpResponse(resp, respBody)
			// if err != nil {
			// 	fmt.Printf("dump response %s error: %s", r.RemoteAddr, err)
			// } else {
			// 	os.Stdout.Write(data)
			// }
			return nil
		},
	}
	proxy.ServeHTTP(rw, r)
}

func InitLog(debug bool) error {
	logger := slog.New(slog.NewTextHandler(os.Stdout))
	if debug {
		logger.Enabled(slog.DebugLevel)
	}
	slog.SetDefault(logger)

	// slog.LogAttrs(slog.ErrorLevel, "oops",
	// 	slog.Int("status", 500),
	// 	slog.Any("err", net.ErrClosed),
	// )
	return nil
}
func main() {
	flag.BoolVar(&debug, "debug", debug, "debug log level")
	flag.StringVar(&addr, "addr", addr, "server serve address")
	flag.UintVar(&readTimeout, "read-timeout", readTimeout, "server read timeout")
	flag.UintVar(&writeTimeout, "write-timeout", writeTimeout, "server write timeout")
	envflag.Parse()

	exit := make(chan os.Signal, 3)
	signal.Notify(exit, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)
	server := http.Server{
		Addr:         addr,
		ReadTimeout:  time.Duration(readTimeout) * time.Minute,
		WriteTimeout: time.Duration(writeTimeout) * time.Minute,
		Handler:      http.HandlerFunc(proxy),
	}

	go func() {
		<-exit
		if err := server.Shutdown(context.TODO()); err != nil {
			slog.Error("server shutdown error", err,
				slog.String("addr", addr),
			)
		}
	}()
	slog.Info("server starting",
		slog.String("addr", addr),
	)
	if err := server.ListenAndServe(); err != nil {
		if err == http.ErrServerClosed {
			slog.Info("server shutdown",
				slog.String("error", err.Error()),
			)
		} else {
			slog.Error("server starting error", err,
				slog.String("addr", addr),
			)
		}
	}
	slog.Info("server stoped",
		slog.String("addr", addr),
	)
}
