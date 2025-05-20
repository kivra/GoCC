package main

import (
	"encoding/json"
	"fmt"
	"github.com/kivra/gocc/pkg/config"
	"github.com/kivra/gocc/pkg/config/experimental/svc_discovery"
	"github.com/kivra/gocc/pkg/limiter/limiter_api"
	"github.com/samber/lo"
	lop "github.com/samber/lo/parallel"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func newDefaultTestCfg() *config.GlobalCfg {
	cfg := config.NewGlobalCfg()
	cfg.WindowMillis.Default = lo.ToPtr(1000)
	cfg.MaxRequests.Default = lo.ToPtr(100)
	cfg.MaxRequestsInQueue.Default = lo.ToPtr(100)
	cfg.RequestsCanSetRate.Default = lo.ToPtr(true)
	cfg.RequestsCanModQueue.Default = lo.ToPtr(true)
	cfg.LogIncludesSource.Default = lo.ToPtr(false)
	cfg.InstanceUrls.Default = lo.ToPtr([]string{})
	cfg.ServerType.Default = lo.ToPtr("echo-http2")
	cfg.ConfigFile.Default = lo.ToPtr("")
	cfg.Port.Default = lo.ToPtr(0)
	cfg.LogFormat.Default = lo.ToPtr("json")
	cfg.LogLevel.Default = lo.ToPtr("WARN")
	return cfg
}

func TestRun_server_starts(t *testing.T) {

	cfg := newDefaultTestCfg()

	app := StartApplication(cfg, true)
	defer app.Close()

	slog.Info(fmt.Sprintf("Server started on port %d", app.Port))
}

func TestRun_can_make_requests(t *testing.T) {

	cfg := newDefaultTestCfg()

	app := StartApplication(cfg, true)
	defer app.Close()

	if !makeTestRequest(app.Port, "my-id", false) {
		t.Fatalf("Failed to make request")
	}
}

func TestRun_can_make_requests_and_get_debug_output(t *testing.T) {

	cfg := newDefaultTestCfg()
	cfg.WindowMillis.Default = lo.ToPtr(10_000)

	app := StartApplication(cfg, true)
	defer app.Close()

	makeTestRequest(app.Port, "id1", false)
	makeTestRequest(app.Port, "id2", false)
	makeTestRequest(app.Port, "id3", false)
	makeTestRequest(app.Port, "id4", false)

	allData := makeDebugRequest(app.Port, "")

	snapshot := limiter_api.DebugSnapshotAll{}
	err := json.Unmarshal([]byte(allData), &snapshot)
	if err != nil {
		t.Fatalf("Failed to unmarshal debug data: %v", err)
	}

	if len(snapshot.Instances) != 4 {
		t.Fatalf("Expected 4 instances, got %d", len(snapshot.Instances))
	}

	if makeDebugRequest(app.Port, "id1") == "" {
		t.Fatalf("Expected debug data for id1")
	}

	if makeDebugRequest(app.Port, "id2") == "" {
		t.Fatalf("Expected debug data for id2")
	}

	if makeDebugRequest(app.Port, "id3") == "" {
		t.Fatalf("Expected debug data for id3")
	}

	if makeDebugRequest(app.Port, "id4") == "" {
		t.Fatalf("Expected debug data for id4")
	}

	if makeDebugRequest(app.Port, "id5") != "" {
		t.Fatalf("Expected no debug data for id5")
	}
}

func TestRun_can_make_many_requests_on_diff_keys(t *testing.T) {

	cfg := newDefaultTestCfg()
	cfg.MaxRequests.Default = lo.ToPtr(1)

	app := StartApplication(cfg, true)
	defer app.Close()

	t0 := time.Now()

	for i := 0; i < 100; i++ {

		key := fmt.Sprintf("my-id-%d", i)

		if !makeTestRequest(app.Port, key, false) {
			t.Fatalf("Failed to make request")
		}
	}

	t1 := time.Now()
	if t1.Sub(t0) > 1*time.Second {
		t.Fatalf("Expected all requests to be served in less than 1s, took %v", t1.Sub(t0))
	}
}

func TestRun_can_make_many_requests_on_same_key(t *testing.T) {

	cfg := newDefaultTestCfg()

	app := StartApplication(cfg, true)
	defer app.Close()

	t0 := time.Now()

	for i := 0; i < 100; i++ {

		key := "my-id"

		if !makeTestRequest(app.Port, key, false) {
			t.Fatalf("Failed to make request")
		}
	}

	t1 := time.Now()
	if t1.Sub(t0) > 1*time.Second {
		t.Fatalf("Expected all requests to be served in less than 1s, took %v", t1.Sub(t0))
	}
}

func TestRun_cant_make_to_many_requests(t *testing.T) {

	cfg := newDefaultTestCfg()

	app := StartApplication(cfg, true)
	defer app.Close()

	t0 := time.Now()
	key := "my-id"

	for i := 0; i < 100; i++ {
		if !makeTestRequest(app.Port, key, false) {
			t.Fatalf("Failed to make request")
		}
	}

	// Make one more requests, that should fail
	if makeTestRequest(app.Port, key, false) {
		t.Fatalf("Expected request to fail")
	}

	t1 := time.Now()
	if t1.Sub(t0) > 1*time.Second {
		t.Fatalf("Expected all requests to be served in less than 1s, took %v", t1.Sub(t0))
	}
}

func TestStartServer_10000_simultaneous_requests_same_key(t *testing.T) {

	cfg := newDefaultTestCfg()
	cfg.WindowMillis.Default = lo.ToPtr(100)
	cfg.MaxRequests.Default = lo.ToPtr(5000)
	cfg.MaxRequestsInQueue.Default = lo.ToPtr(10000)

	app := StartApplication(cfg, true)
	defer app.Close()

	t0 := time.Now()

	key := "my-id"

	errs := make(chan error, 10000)

	successes := atomic.Int32{}

	numRequests := 10000
	nThreads := 10

	lop.ForEach(lo.Range(nThreads), func(i int, _ int) {
		for j := 0; j < numRequests/nThreads; j++ {
			if makeTestRequest(app.Port, key, true) {
				successes.Add(1)
			}
		}
	})

	close(errs)

	if successes.Load() != 10000 {
		t.Errorf("expected 10000 successes, got %d", successes.Load())
		for err := range errs {
			t.Fatalf("one or more requests failed: %v", err)
		}
	}

	// takes 250 ms on my machine, but 10s on gh actions CI :S
	t1 := time.Now()
	if t1.Sub(t0) > 30*time.Second {
		t.Fatalf("Expected all requests to be served in less than 30s, took %v", t1.Sub(t0))
	}

}

func TestStartServer_config_change(t *testing.T) {

	tempDir := os.TempDir() + "/gocc-test"
	if !config.DirExists(tempDir) {
		err := os.Mkdir(tempDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
	}

	configFileName := "app-config.json"
	configFilePath := tempDir + "/" + configFileName

	globalConfig := newDefaultTestCfg()
	globalConfig.WindowMillis.Default = lo.ToPtr(1000)
	globalConfig.MaxRequests.Default = lo.ToPtr(1)
	globalConfig.MaxRequestsInQueue.Default = lo.ToPtr(0)
	globalConfig.LogLevel.Default = lo.ToPtr("INFO")
	globalConfig.LogFormat.Default = lo.ToPtr("text")
	globalConfig.ConfigFile.Default = lo.ToPtr(configFilePath)

	configFromFile := &config.CfgFromFile{
		Keys: []config.CfgFromFileKey{
			{
				KeyPattern:           "key1",
				KeyPatternIsRegex:    false,
				MaxRequestsPerWindow: 1,
				MaxRequestsInQueue:   0,
				WindowMillis:         1_000_000,
			},
			{
				KeyPattern:           "key2",
				KeyPatternIsRegex:    false,
				MaxRequestsPerWindow: 1,
				MaxRequestsInQueue:   0,
				WindowMillis:         1_000_000,
			},
		},
	}

	err := config.WriteAppConfigFile(configFilePath, configFromFile)
	if err != nil {
		t.Fatalf("Failed to write app config file: %v", err)
	}

	app := StartApplication(globalConfig, true)
	defer app.Close()

	// Initial should go through
	if !makeTestRequest(app.Port, "key1", false) {
		t.Fatalf("Failed to make request")
	}
	if !makeTestRequest(app.Port, "key2", false) {
		t.Fatalf("Failed to make request")
	}

	// But then should get 429
	if makeTestRequest(app.Port, "key1", false) {
		t.Fatalf("Unexpectedly succeeded in making request")
	}
	if makeTestRequest(app.Port, "key2", false) {
		t.Fatalf("Unexpectedly succeeded in making request")
	}

	// Now we edit the config and overwrite the file, which
	// should trigger a reload of the config
	configFromFile.Keys[0].MaxRequestsPerWindow = 2
	configFromFile.Keys[1].MaxRequestsPerWindow = 2

	err = config.WriteAppConfigFile(configFilePath, configFromFile)
	if err != nil {
		t.Fatalf("Failed to write app config file: %v", err)
	}

	t0 := time.Now()

	key1Success := false
	key2Success := false

	for time.Since(t0) < 5*time.Second && (!key1Success || !key2Success) {

		time.Sleep(100 * time.Millisecond)

		// Initial should go through
		if !makeTestRequest(app.Port, "key1", false) && !key1Success {
			slog.Warn("Config has not been reloaded yet")
			continue
		}
		key1Success = true

		if !makeTestRequest(app.Port, "key2", false) && !key2Success {
			slog.Warn("Config has not been reloaded yet")
			continue
		}
		key2Success = true

	}

	if !key1Success || !key2Success {
		t.Fatalf("Failed to make more requests after config reload")
	}

}

func TestStartServer_10000_simultaneous_requests_diff_keys(t *testing.T) {

	cfg := newDefaultTestCfg()
	cfg.WindowMillis.Default = lo.ToPtr(100)
	cfg.MaxRequests.Default = lo.ToPtr(5000)
	cfg.MaxRequestsInQueue.Default = lo.ToPtr(5000)

	app := StartApplication(cfg, true)
	defer app.Close()

	t0 := time.Now()

	errs := make(chan error, 10000)

	nSuccesses := atomic.Int32{}

	numRequests := 10000
	nThreads := 10

	lop.ForEach(lo.Range(nThreads), func(i int, _ int) {
		for j := 0; j < numRequests/nThreads; j++ {

			key := "my-id-" + fmt.Sprintf("%d", rand.Intn(100))

			if !makeTestRequest(app.Port, key, true) {
				errs <- fmt.Errorf("failed to make request")
				continue
			}

			nSuccesses.Add(1)
		}
	})

	close(errs)

	if nSuccesses.Load() != 10000 {
		t.Errorf("expected 10000 successes, got %d", nSuccesses.Load())
		for err := range errs {
			t.Fatalf("%v", err)
		}
	}

	// takes 250 ms on my machine, but 10s on gh actions CI :S
	t1 := time.Now()
	if t1.Sub(t0) > 30*time.Second {
		t.Fatalf("Expected all requests to be served in less than 30s, took %v", t1.Sub(t0))
	}

}

// Too many parallel connections on windows and macos. need to keep to about 50 goroutines max
func TestStartServer_10000_truly_simultaneous_requests(t *testing.T) {

	cfg := newDefaultTestCfg()
	cfg.WindowMillis.Default = lo.ToPtr(100)
	cfg.MaxRequests.Default = lo.ToPtr(5000)
	cfg.MaxRequestsInQueue.Default = lo.ToPtr(10000)
	cfg.ServerType.Default = lo.ToPtr("echo-http2")

	app := StartApplication(cfg, true)
	defer app.Close()

	t0 := time.Now()

	errs := make(chan error, 10000)

	nSuccesses := atomic.Int32{}

	lop.ForEach(lo.Range(10000), func(i int, _ int) {

		key := "my-id-" + fmt.Sprintf("%d", rand.Intn(100))
		if makeHttp2TestRequest(app.Port, key, true) {
			nSuccesses.Add(1)
		}
	})

	close(errs)

	if nSuccesses.Load() != 10000 {
		t.Errorf("expected 10000 successes, got %d", nSuccesses.Load())
		for err := range errs {
			t.Fatalf("%v", err)
		}
	}

	// takes 100 ms on my machine, but 10s on gh actions CI :S
	elapsed := time.Since(t0)
	if elapsed > 10*time.Second {
		t.Fatalf("Expected all requests to be served in less than 30s, took %v", elapsed)
	}

}

func TestStartServer_100k_requests(t *testing.T) {

	cfg := newDefaultTestCfg()
	cfg.WindowMillis.Default = lo.ToPtr(100)
	cfg.MaxRequestsInQueue.Default = lo.ToPtr(10000)
	cfg.MaxRequests.Default = lo.ToPtr(1_000_000)
	cfg.ServerType.Default = lo.ToPtr("echo-http2")

	app := StartApplication(cfg, true)
	defer app.Close()

	t0 := time.Now()

	nRequests := 100_000
	nThreads := 10

	nSuccesses := atomic.Int32{}

	errs := make(chan error, nRequests)

	lop.ForEach(lo.Range(nThreads), func(_ int, _ int) {
		for j := 0; j < nRequests/nThreads; j++ {

			key := "my-id-" + fmt.Sprintf("%d", rand.Intn(100))

			if makeHttp2TestRequest(app.Port, key, true) {
				nSuccesses.Add(1)
			}
		}
	})

	close(errs)

	if nSuccesses.Load() != int32(nRequests) {
		t.Errorf("expected %d successes, got %d", nRequests, nSuccesses.Load())
		for err := range errs {
			t.Fatalf("%v", err)
		}
	}

	// This takes 2s on my machine, but 25 seconds on gh actions :D :D
	t1 := time.Now()
	if t1.Sub(t0) > 60*time.Second {
		t.Fatalf("Expected all requests to be served in less than 60s, took %v", t1.Sub(t0))
	}

}

func TestStartApplication_forwardToRightInstance(t *testing.T) {

	port := 8999
	portStr := fmt.Sprintf("%d", port)

	cfg := newDefaultTestCfg()
	cfg.Port.Default = lo.ToPtr(port)
	cfg.LogLevel.Default = lo.ToPtr("INFO")
	//goland:noinspection HttpUrlsUsage
	cfg.InstanceUrls.Default = lo.ToPtr([]string{"http://localhost:" + portStr, "http://" + svc_discovery.GetOwnHostName() + ":" + portStr})

	app := StartApplication(cfg, true)
	defer app.Close()

	// sends request to localhost, forwards to right address (real hostname of the machine)
	makeTestRequest(app.Port, "my-id", true)
	makeHttp2TestRequest(app.Port, "my-id", true)

}

func makeDebugRequest(port int, key string) string {

	var resp *http.Response
	var err error
	if key != "" {
		resp, err = http1Client.Get(fmt.Sprintf("http://localhost:%d/debug/%s", port, key))
	} else {
		resp, err = http1Client.Get(fmt.Sprintf("http://localhost:%d/debug", port))
	}
	if err != nil {
		panic(fmt.Sprintf("Error making GET request: %v", err))
	}

	if resp.StatusCode == 404 {
		return ""
	}

	if resp.StatusCode != 200 {
		panic(fmt.Sprintf("Expected status code 200, got %d", resp.StatusCode))
	}

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(fmt.Sprintf("Error reading response body: %v", err))
	}

	return string(bytes)
}

func makeTestRequestClient(port int, key string, canWait bool, client *http.Client) bool {
	query := ""
	if canWait {
		query = "?canWait=true"
	}
	return makePerfTestRequest(port, "POST", "/rate/"+key, query, client)
}

func makeTestRequest(port int, key string, canWait bool) bool {
	return makeTestRequestClient(port, key, canWait, http1Client)
}

func makeHttp2TestRequest(port int, key string, canWait bool) bool {
	return makeTestRequestClient(port, key, canWait, http2Client)
}
