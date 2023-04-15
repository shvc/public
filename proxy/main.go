package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gobike/envflag"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"golang.org/x/exp/slog"
)

var (
	addr         string = ":80"
	backend      string = "http://192.168.56.2:9000"
	provider     string = "http://192.168.56.2:14268"
	serviceName  string = "svc-xxx"
	readTimeout  uint   = 30
	writeTimeout uint   = 30
	debug        bool
)

var (
	providerURL *url.URL
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
	tr := otel.Tracer("component-main")
	spanCtx, span := tr.Start(r.Context(), "proxy-handler")
	defer span.End()

	proxy := httputil.ReverseProxy{
		Transport: defaultTransport,
		Director: func(req *http.Request) {
			req.Host = r.Host
			req.URL = r.URL
			req.URL.Scheme = providerURL.Scheme
			req.URL.Host = providerURL.Host
			req.Header.Set("User-Agent", r.UserAgent())

			p := otel.GetTextMapPropagator()
			p.Inject(spanCtx, propagation.HeaderCarrier(req.Header))
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
			slog.Info("request",
				slog.String("remote", r.RemoteAddr),
				slog.String("method", r.Method),
				slog.String("host", r.Host),
				slog.String("url", r.URL.String()),
				slog.String("uri", r.RequestURI),
				slog.Int("response", resp.StatusCode),
			)
			return nil
		},
	}
	proxy.ServeHTTP(rw, r)
}

func InitLog(debug bool) error {
	logger := slog.New(slog.NewTextHandler(os.Stdout))
	if debug {
		logger.Enabled(context.Background(), slog.LevelDebug)
	}
	slog.SetDefault(logger)

	// slog.LogAttrs(slog.ErrorLevel, "oops",
	// 	slog.Int("status", 500),
	// 	slog.Any("err", net.ErrClosed),
	// )
	return nil
}

func tracerProvider(url string) (tp *tracesdk.TracerProvider, err error) {
	fmt.Println("init traceProvider")
	var exporter tracesdk.SpanExporter
	if url == "" {
		exporter, err = stdouttrace.New()
		if err != nil {
			return
		}
	} else {
		exporter, err = jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(url)))
		if err != nil {
			return
		}
	}
	tp = tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(exporter),
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
			attribute.String("environment", "environment"),
			attribute.Int64("ID", 9999),
		)),
	)
	return
}
func main() {
	flag.BoolVar(&debug, "debug", debug, "debug log level")
	flag.StringVar(&addr, "addr", addr, "server serve address")
	flag.StringVar(&backend, "b", backend, "backend server address")
	flag.StringVar(&provider, "tp", provider, "trace provider address")
	flag.UintVar(&readTimeout, "read-timeout", readTimeout, "server read timeout")
	flag.UintVar(&writeTimeout, "write-timeout", writeTimeout, "server write timeout")
	envflag.Parse()

	var err error
	providerURL, err = url.Parse(backend)
	if err != nil {
		panic(err)
	}

	tp, err := tracerProvider(provider)
	if err != nil {
		panic(err)
	}
	otel.SetTracerProvider(tp)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func(ctx context.Context) {
		ctx, cancel = context.WithTimeout(ctx, time.Second*5)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			slog.Error("stop trace provider",
				slog.String("err", err.Error()),
			)
		}
	}(ctx)

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
	slog.Info("server stopped",
		slog.String("addr", addr),
	)
}
