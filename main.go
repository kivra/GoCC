package main

import (
	"fmt"
	"github.com/GiGurra/boa/pkg/boa"
	"github.com/google/uuid"
	"github.com/kivra/gocc/cmd/benchmark"
	"github.com/kivra/gocc/pkg/config"
	"github.com/kivra/gocc/pkg/limiter/limiter_api"
	"github.com/kivra/gocc/pkg/limiter/limiter_manager"
	"github.com/kivra/gocc/pkg/logging"
	"github.com/kivra/gocc/pkg/server"
	endpoints2 "github.com/kivra/gocc/pkg/server/endpoints"
	"github.com/spf13/cobra"
	"log/slog"
	"strings"
	"time"
)

const (
	AppName = "gocc"
)

func main() {
	cfg := config.NewGlobalCfg()
	boa.Cmd{
		Use:   AppName,
		Short: "An in-memory rate limiter with a fixed time window",
		Long: strings.Join([]string{
			"An in-memory rate limiter with a fixed time window. Runs 1 go-routine per key.",
			"- GET|POST to /rate/:key to rate limit for a key. Returns a generated request ID",
			" - optionally: ?canWait=true waits (FIFO) before returning, when the rate limit is exceeded.",
			" - optionally: ?maxRequests=200 sets max requests per window for the key.",
			" - optionally: ?maxRequestsInQueue=400 sets max requests in queue for the key after the window is full.",
			"- DELETE to /rate/:key/:requestId to decrement the rate limiter for a key.",
			"- GET to /healthz to check if the server is up.",
			"- GET to /debug|/debug/:key introspect the state of limiters.",
		}, "\n"),
		Params:      cfg,
		ParamEnrich: boa.ParamEnricherDefault,
		RunFunc:     func(cmd *cobra.Command, args []string) { StartApplication(cfg, false) },
		SubCmds: []*cobra.Command{
			benchmark.Cmd(),
		},
	}.Run()
}

type AppHandle struct {
	Port  int
	Close func()
}

func StartApplication(
	globalCfg *config.GlobalCfg,
	async bool,
) AppHandle { // need to return the chosen port if using ephemeral port in configuration

	appCreatedCh := make(chan server.Handle, 1)

	go func() {

		logger := logging.ConfigureDefaultLogger(globalCfg.LogFormat.Value(), globalCfg.LogLevel.Value(), globalCfg.LogIncludesSource.Value())

		// Makes uuid generation 4x faster (has a significant impact on our performance)
		slog.Info("Enabling fast uuid generation")
		uuid.EnableRandPool()

		slog.Info(strings.Join([]string{"Starting " + AppName + ", global config:",
			fmt.Sprintf("           globalCfg.ServerType: %v", globalCfg.ServerType.Value()),
			fmt.Sprintf(" globalCfg.MaxRequestsPerWindow: %v", globalCfg.MaxRequests.Value()),
			fmt.Sprintf("   globalCfg.MaxRequestsInQueue: %v", globalCfg.MaxRequestsInQueue.Value()),
			fmt.Sprintf("         globalCfg.WindowMillis: %v", globalCfg.WindowMillis.Value()),
			fmt.Sprintf("   globalCfg.RequestsCanSetRate: %v", globalCfg.RequestsCanSetRate.Value()),
			fmt.Sprintf("  globalCfg.RequestsCanModQueue: %v", globalCfg.RequestsCanModQueue.Value()),
			fmt.Sprintf("           globalCfg.ConfigFile: %v", globalCfg.ConfigFile.Value()),
			fmt.Sprintf("                 globalCfg.Port: %v", globalCfg.Port.Value()),
			fmt.Sprintf("            globalCfg.LogFormat: %v", globalCfg.LogFormat.Value()),
			fmt.Sprintf("             globalCfg.LogLevel: %v", globalCfg.LogLevel.Value()),
			fmt.Sprintf("    globalCfg.LogIncludesSource: %v", globalCfg.LogIncludesSource.Value()),
			fmt.Sprintf("         globalCfg.InstanceUrls: %v", globalCfg.InstanceUrls.Value()),
		}, "\n"))

		// Check if we should run distributed mode
		validCfg, err := globalCfg.ValidateInstanceUrls()
		if err != nil {
			panic(fmt.Sprintf("Failed to parse instance urls: %v", err))
		}
		if validCfg.DistributedMode() {
			slog.Info("Service is starting in distributed mode")
		} else {
			slog.Info("Service is starting in single instance mode")
		}

		slog.Info("Checking config file", slog.String("configFile", globalCfg.ConfigFile.Value()))
		initConfigFromFile, configFileChangeMonitor := config.MonitorConfigFromFile(globalCfg.ConfigFile.Value())
		defer configFileChangeMonitor.Close()

		slog.Info("Starting limiter manager set")
		limiterManager := limiter_manager.NewManagerSet(
			toLimiterConfig(globalCfg),
			initConfigFromFile,
			configFileChangeMonitor.Changes(),
			limiter_manager.DefaultSharding,
		)
		defer limiterManager.Close()

		slog.Info(fmt.Sprintf("Creating server of type %v", globalCfg.ServerType.Value()))
		srv := server.CreateNew(globalCfg, logger)
		defer func() { _ = srv.Close() }()

		slog.Info("Setting up routes")

		srv.POST("/rate/:key", endpoints2.HandleRateRequest(validCfg, limiterManager))
		srv.GET("/rate/:key", endpoints2.HandleRateRequest(validCfg, limiterManager))
		srv.DELETE("/rate/:key/:id", endpoints2.HandleReleaseRequest(validCfg, limiterManager))

		srv.GET("/debug", endpoints2.HandleDebugRequest(limiterManager))
		srv.GET("/debug/:key", endpoints2.HandleDebugRequest(limiterManager))

		srv.GET("/healthz", endpoints2.HandleHealthRequest)

		slog.Info("Starting http server")
		server.StartListening(globalCfg, srv, appCreatedCh)
	}()

	select {
	case app := <-appCreatedCh:
		slog.Info(fmt.Sprintf("Server started on port %d", app.Port), slog.Int("port", app.Port))
		if async {
			return AppHandle(app)
		} else {
			select {} // block forever
		}
	case <-time.After(5 * time.Second):
		panic("Server did not start within 5 seconds")
	}
}

func toLimiterConfig(cfg *config.GlobalCfg) *limiter_api.Config {
	return &limiter_api.Config{
		MaxRequestsPerWindow: cfg.MaxRequests.Value(),
		MaxRequestsInQueue:   cfg.MaxRequestsInQueue.Value(),
		WindowMillis:         cfg.WindowMillis.Value(),
	}
}
