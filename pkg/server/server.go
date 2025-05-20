package server

import (
	"errors"
	"fmt"
	"github.com/kivra/gocc/pkg/config"
	"github.com/kivra/gocc/pkg/server/endpoints"
	"github.com/labstack/echo/v4"
	slogecho "github.com/samber/slog-echo"
	"github.com/valyala/fasthttp"
	"golang.org/x/net/http2"
	"log/slog"
	"net"
	"net/http"
	"time"
)

type Handle struct {
	Port  int
	Close func()
}

func CreateNew(
	globalCfg *config.GlobalCfg,
	logger *slog.Logger,
) *echo.Echo {

	srv := echo.New()

	if globalCfg.LogFormat.Value() != "system-default" {
		srv.Use(slogecho.NewWithConfig(logger, slogecho.Config{
			DefaultLevel:      slog.LevelInfo,
			ClientErrorLevel:  slog.LevelWarn,
			ServerErrorLevel:  slog.LevelError,
			WithRequestID:     true,
			WithRequestHeader: true,
			Filters: []slogecho.Filter{
				func(ctx echo.Context) bool {
					if ctx != nil && ctx.Response() != nil {
						switch ctx.Response().Status / 100 {
						case 2:
							return globalCfg.Log2xx.Value()
						case 4:
							return globalCfg.Log4xx.Value()
						case 5:
							return globalCfg.Log5xx.Value()
						default:
							slog.Error("Unknown status code returned internally :S", slog.Int("status", ctx.Response().Status))
							return true
						}
					} else {
						// Should not happen?
						slog.Error("No response context found, logging all requests", slog.String("context", fmt.Sprintf("%+v", ctx)))
						return true
					}
				},
			},
		}))
		srv.HidePort = globalCfg.Port.Value() != 0
		srv.HideBanner = true
	} else {
		srv.Use(slogecho.New(logger))
	}

	return srv
}

func StartListening(
	globalCfg *config.GlobalCfg,
	server *echo.Echo,
	appCreatedListener chan<- Handle,
) {

	// start a goroutine that monitors the echo server and emits the port when it has been bound
	// This is an ugly way of doing it, but unfortunately, echo offers no other way to get the port
	// when using ephemeral ports. See https://github.com/labstack/echo/issues/1065
	go func() {
		for server.Listener == nil {
			time.Sleep(100 * time.Millisecond)
		}
		appCreatedListener <- Handle{
			Port:  server.Listener.Addr().(*net.TCPAddr).Port,
			Close: func() { _ = server.Close() },
		}
	}()

	switch config.ServerType(globalCfg.ServerType.Value()) {
	case config.ServerTypeEcho:
		err := server.Start(fmt.Sprintf(":%d", globalCfg.Port.Value()))
		if err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				panic(fmt.Sprintf("Failed to start server on port %d due to %v", globalCfg.Port.Value(), err))
			}
		}
	case config.ServerTypeEchohttp2:
		// Testing http2, without TLS
		// Unfortunately, the standard library's http2 implementation is
		// quite slow for non-http2 optimized content, so it's not a silver bullet for us unfortunately
		// see https://github.com/golang/go/issues/47840
		// It does show slight performance benefits in highly parallel scenarios, so we still keep it
		http2Backend := &http2.Server{
			MaxConcurrentStreams: 250,
			IdleTimeout:          30 * time.Second,
		}
		err := server.StartH2CServer(fmt.Sprintf(":%d", globalCfg.Port.Value()), http2Backend)
		if err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				panic(fmt.Sprintf("Failed to start server on port %d due to %v", globalCfg.Port.Value(), err))
			}
		}
	case config.ServerTypeFast:
		slog.Warn("Server implementation not supported/incomplete. Just starting /healthz endpoint for performance testing", slog.String("impl", "fast"))

		slog.Info("Try binding port", slog.Int("port", globalCfg.Port.Value()))
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", globalCfg.Port.Value()))
		if err != nil {
			panic(fmt.Sprintf("Failed to start server on port %d due to %v", globalCfg.Port.Value(), err))
		}

		port := listener.Addr().(*net.TCPAddr).Port
		slog.Info(fmt.Sprintf("Bound to port %d", port))

		// For hacky backwards compatibility with the old server in tests
		server.Listener = listener
		if err := fasthttp.Serve(listener, endpoints.FastHttpServerTesthandler); err != nil {
			panic(fmt.Sprintf("Failed to start server on port %d due to %v", globalCfg.Port.Value(), err))
		}
		slog.Info(fmt.Sprintf("Server stopped on port %d", globalCfg.Port.Value()))
	default:
		panic(fmt.Sprintf("Unknown server implementation: %v", globalCfg.ServerType.Value()))
	}
}
