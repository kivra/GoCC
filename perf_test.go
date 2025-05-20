package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/kivra/gocc/pkg/config"
	"github.com/kivra/gocc/pkg/logging"
	"github.com/samber/lo"
	lop "github.com/samber/lo/parallel"
	"github.com/valyala/fasthttp"
	"golang.org/x/net/http2"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func newDefaultPerfTestCfg() *config.GlobalCfg {
	cfg := config.NewGlobalCfg()
	cfg.WindowMillis.Default = lo.ToPtr(1000)
	cfg.MaxRequests.Default = lo.ToPtr(100)
	cfg.MaxRequestsInQueue.Default = lo.ToPtr(100)
	cfg.RequestsCanSetRate.Default = lo.ToPtr(true)
	cfg.RequestsCanModQueue.Default = lo.ToPtr(true)
	cfg.LogIncludesSource.Default = lo.ToPtr(false)
	cfg.InstanceUrls.Default = lo.ToPtr([]string{})
	cfg.ServerType.Default = lo.ToPtr("echo")
	cfg.ConfigFile.Default = lo.ToPtr("")
	cfg.Port.Default = lo.ToPtr(0)
	cfg.LogFormat.Default = lo.ToPtr("json")
	cfg.LogLevel.Default = lo.ToPtr("WARN")
	return cfg
}

func TestPerf_health_endpoint_single_request(t *testing.T) {

	cfg := newDefaultPerfTestCfg()

	app := StartApplication(cfg, true)
	defer app.Close()

	slog.Info(fmt.Sprintf("Server started on port %d", app.Port))

	if !makePerfTestRequest(app.Port, "GET", "/healthz", "", http1Client) {
		t.Errorf("Failed to make GET request to /healthz")
	}
}

func TestPerf_health_endpoint_single_request_fast_http_client(t *testing.T) {

	cfg := newDefaultPerfTestCfg()
	cfg.ServerType.Default = lo.ToPtr("fast")

	app := StartApplication(cfg, true)
	defer app.Close()

	slog.Info(fmt.Sprintf("Server started on port %d", app.Port))

	if !makePerfTestRequestFastHttp(app.Port, "GET", "/healthz", "") {
		t.Errorf("Failed to make GET request to /healthz")
	}
}

func TestPerf_health_endpoint_single_thread(t *testing.T) {

	cfg := newDefaultPerfTestCfg()

	app := StartApplication(cfg, true)
	defer app.Close()

	slog.Info(fmt.Sprintf("Server started on port %d", app.Port))

	if !makePerfTestRequest(app.Port, "GET", "/healthz", "", http1Client) {
		t.Errorf("Failed to make GET request to /healthz")
	}

	nRequests := 100_000

	t0 := time.Now()

	for i := 0; i < nRequests; i++ {
		if !makePerfTestRequest(app.Port, "GET", "/healthz", "", http1Client) {
			t.Errorf("Failed to make GET request to /healthz")
		}
	}

	logging.ConfigureDefaultLogger("json", "INFO", false)

	elapsed := time.Since(t0)

	slog.Info(fmt.Sprintf("Elapsed time for %d requests: %v", nRequests, elapsed))
}

func TestPerf_health_endpoint_single_thread_fast_http_client(t *testing.T) {

	cfg := newDefaultPerfTestCfg()
	cfg.ServerType.Default = lo.ToPtr("fast")

	app := StartApplication(cfg, true)
	defer app.Close()

	slog.Info(fmt.Sprintf("Server started on port %d", app.Port))

	if !makePerfTestRequestFastHttp(app.Port, "GET", "/healthz", "") {
		t.Errorf("Failed to make GET request to /healthz")
	}

	nRequests := 100_000

	t0 := time.Now()

	for i := 0; i < nRequests; i++ {
		if !makePerfTestRequestFastHttp(app.Port, "GET", "/healthz", "") {
			t.Errorf("Failed to make GET request to /healthz")
		}
	}

	logging.ConfigureDefaultLogger("json", "INFO", false)

	elapsed := time.Since(t0)

	slog.Info(fmt.Sprintf("Elapsed time for %d requests: %v", nRequests, elapsed))
}

func TestPerf_health_endpoint_many_threads(t *testing.T) {

	cfg := newDefaultPerfTestCfg()

	app := StartApplication(cfg, true)
	defer app.Close()

	slog.Info(fmt.Sprintf("Server started on port %d", app.Port))

	if !makePerfTestRequest(app.Port, "GET", "/healthz", "", http1Client) {
		t.Errorf("Failed to make GET request to /healthz")
	}

	nRequests := 100_000
	nThreads := 2

	t0 := time.Now()

	lop.ForEach(lo.Range(nThreads), func(_ int, _ int) {
		for i := 0; i < nRequests/nThreads; i++ {
			if !makePerfTestRequest(app.Port, "GET", "/healthz", "", http1Client) {
				t.Errorf("Failed to make GET request to /healthz")
			}
		}
	})

	logging.ConfigureDefaultLogger("json", "INFO", false)

	elapsed := time.Since(t0)

	slog.Info(fmt.Sprintf("Elapsed time for %d requests: %v", nRequests, elapsed))
}

func TestPerf_health_endpoint_many_threads_fast_http_client(t *testing.T) {

	cfg := newDefaultPerfTestCfg()
	cfg.ServerType.Default = lo.ToPtr("fast")

	app := StartApplication(cfg, true)
	defer app.Close()

	slog.Info(fmt.Sprintf("Server started on port %d", app.Port))

	if !makePerfTestRequestFastHttp(app.Port, "GET", "/healthz", "") {
		t.Errorf("Failed to make GET request to /healthz")
	}

	nRequests := 100_000
	nThreads := 3 // 3 seems to be optimal for fast http client impl

	t0 := time.Now()

	lop.ForEach(lo.Range(nThreads), func(_ int, _ int) {
		for i := 0; i < nRequests/nThreads; i++ {
			if !makePerfTestRequestFastHttp(app.Port, "GET", "/healthz", "") {
				t.Errorf("Failed to make GET request to /healthz")
			}
		}
	})

	logging.ConfigureDefaultLogger("json", "INFO", false)

	elapsed := time.Since(t0)

	slog.Info(fmt.Sprintf("Elapsed time for %d requests: %v", nRequests, elapsed))
}

func TestPerf_health_endpoint_many_threads_http2(t *testing.T) {

	cfg := newDefaultPerfTestCfg()
	cfg.ServerType.Default = lo.ToPtr("echo-http2")

	app := StartApplication(cfg, true)
	defer app.Close()

	slog.Info(fmt.Sprintf("Server started on port %d", app.Port))

	if !makePerfTestRequest(app.Port, "GET", "/healthz", "", http2Client) {
		t.Errorf("Failed to make GET request to /healthz")
	}

	nRequests := 100_000
	nThreads := 10

	t0 := time.Now()

	lop.ForEach(lo.Range(nThreads), func(_ int, _ int) {
		for i := 0; i < nRequests/nThreads; i++ {
			if !makePerfTestRequest(app.Port, "GET", "/healthz", "", http2Client) {
				t.Errorf("Failed to make GET request to /healthz")
			}
		}
	})

	logging.ConfigureDefaultLogger("json", "INFO", false)

	elapsed := time.Since(t0)

	slog.Info(fmt.Sprintf("Elapsed time for %d requests: %v", nRequests, elapsed))
}

func TestPerf_post_endpoint_many_threads(t *testing.T) {

	cfg := newDefaultPerfTestCfg()
	cfg.MaxRequests.Default = lo.ToPtr(1_000_000)

	app := StartApplication(cfg, true)
	defer app.Close()

	slog.Info(fmt.Sprintf("Server started on port %d", app.Port))

	nRequests := 100_000
	nThreads := 2

	t0 := time.Now()

	lop.ForEach(lo.Range(nThreads), func(iThread int, _ int) {
		for i := 0; i < nRequests/nThreads; i++ {
			if !makePerfTestRequest(app.Port, "POST", "/rate/"+strconv.Itoa(iThread), "", http1Client) {
				t.Errorf("Failed to make POST request to /healthz")
			}
		}
	})

	logging.ConfigureDefaultLogger("json", "INFO", false)

	elapsed := time.Since(t0)

	slog.Info(fmt.Sprintf("Elapsed time for %d requests: %v", nRequests, elapsed))
}

func makePerfTestRequest(
	port int,
	method string,
	path string,
	query string,
	client *http.Client,
) bool {
	switch method {
	case "GET":
		resp, err := client.Get(fmt.Sprintf("http://localhost:%d%s%s", port, path, query))
		if err != nil {
			panic(fmt.Sprintf("Error making GET request: %v", err))
		}
		drainBody(resp)

		if resp.StatusCode != 200 {
			slog.Warn(fmt.Sprintf("GET request to %s failed with status code %d", path, resp.StatusCode))
		}

		if resp.StatusCode/100 == 2 {
			return true
		} else if resp.StatusCode == 429 {
			return false
		} else {
			panic(fmt.Sprintf("Unexpected status code: %d", resp.StatusCode))
		}

	case "POST":
		resp, err := client.Post(fmt.Sprintf("http://localhost:%d%s%s", port, path, query), "application/json", nil)
		if err != nil {
			panic(fmt.Sprintf("Error making POST request: %v", err))
		}
		drainBody(resp)

		if resp.StatusCode != 200 {
			slog.Warn(fmt.Sprintf("POST request to %s failed with status code %d", path, resp.StatusCode))
		}

		if resp.StatusCode/100 == 2 {
			return true
		} else if resp.StatusCode == 429 {
			return false
		} else {
			panic(fmt.Sprintf("Unexpected status code: %d", resp.StatusCode))
		}
	default:
		panic(fmt.Sprintf("Unsupported method: %s", method))
	}

}

func makePerfTestRequestFastHttp(
	port int,
	method string,
	path string,
	query string,
) bool {

	req := fasthttp.AcquireRequest()
	req.SetRequestURI(fmt.Sprintf("http://localhost:%d%s%s", port, path, query))
	switch method {
	case "GET":
		req.Header.SetMethod(fasthttp.MethodGet)
	default:
		panic(fmt.Sprintf("Unsupported method: %s", method))
	}
	resp := fasthttp.AcquireResponse()
	err := fastHttpClient.Do(req, resp)
	defer fasthttp.ReleaseResponse(resp)
	defer fasthttp.ReleaseRequest(req)
	if err == nil {
		slog.Debug(fmt.Sprintf("GET request to %s succeeded", path))
	} else {
		slog.Error(fmt.Sprintf("GET request to %s failed: %v", path, err))
	}

	return resp.StatusCode()/100 == 2
}

func drainBody(resp *http.Response) {
	if resp.ProtoMajor == 1 { // cheating to make http2 tests go faster. Stupid go client :D
		_, err := io.Copy(io.Discard, resp.Body)
		if err != nil {
			panic(fmt.Sprintf("Error reading response body: %v", err))
		}

		err = resp.Body.Close()
		if err != nil {
			panic(fmt.Sprintf("Error closing response body: %v", err))
		}
	}
}

var http1Client = newHttp1Client()
var http2Client = newHttp2Client()
var fastHttpClient = newFastHttpClient()

func newHttp1Client() *http.Client {
	// Create a custom transport with connection pooling enabled
	transport := &http.Transport{
		MaxIdleConns:        1000,             // Maximum number of idle connections to maintain
		MaxIdleConnsPerHost: 1000,             // Maximum number of idle connections per host
		IdleConnTimeout:     90 * time.Second, // Timeout for idle connections
	}

	// Create a new http.Client with the custom transport
	return &http.Client{
		Transport: transport,
	}
}

func newHttp2Client() *http.Client {
	client := &http.Client{
		//Transport: http2.ConfigureTransport(http.DefaultTransport.(*http.Transport)),
		Transport: &http2.Transport{
			AllowHTTP: true,
			// Pretend we are dialing a TLS endpoint.
			// Note, we ignore the passed tls.Config
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
				return net.Dial("tcp", addr)
			},
		},
		//Transport: http.DefaultTransport,
		Timeout: 10 * time.Second,
	}

	return client
}

// can't compile the internals of dgrr/http2 for some reason to try http2 with fasthttp impl.
// go get github.com/dgrr/http2@v0.3.5 seems broken
//func newFastHttp2Client(baseurl string) *fasthttp.HostClient {
//
//	hc := &fasthttp.HostClient{
//		//Addr:  "api.binance.com:443",
//		Addr:  baseurl,
//		IsTLS: false,
//	}
//
//	if err := fasthttp2.ConfigureClient(hc, fasthttp2.ClientOpts{}); err != nil {
//		log.Printf("%s doesn't support http/2\n", hc.Addr)
//	}
//
//	return hc
//}

func newFastHttpClient() *fasthttp.Client {
	readTimeout, _ := time.ParseDuration("30s")
	writeTimeout, _ := time.ParseDuration("30s")
	maxIdleConnDuration, _ := time.ParseDuration("1h")
	return &fasthttp.Client{
		ReadTimeout:                   readTimeout,
		WriteTimeout:                  writeTimeout,
		MaxIdleConnDuration:           maxIdleConnDuration,
		NoDefaultUserAgentHeader:      true, // Don't send: User-Agent: fasthttp
		DisableHeaderNamesNormalizing: true, // If you set the case on your headers correctly you can enable this
		DisablePathNormalizing:        true,
		// increase DNS cache time to an hour instead of default minute
		Dial: (&fasthttp.TCPDialer{
			Concurrency:      4096,
			DNSCacheDuration: time.Hour,
		}).Dial,
	}
}
