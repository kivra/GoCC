package endpoints

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/google/uuid"
	"github.com/kivra/gocc/pkg/config"
	"github.com/kivra/gocc/pkg/limiter/limiter_api"
	"github.com/kivra/gocc/pkg/limiter/limiter_manager"
	"github.com/kivra/gocc/pkg/logging/logctx"
	"github.com/labstack/echo/v4"
	"golang.org/x/net/http2"
	"hash/fnv"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var http2Client = newHttp2Client()

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

func hashKey(key string) uint32 {
	h := fnv.New32a()
	_, err := h.Write([]byte(key))
	if err != nil {
		panic(fmt.Sprintf("BUG: Failed to hash %s", key))
	}
	return h.Sum32()
}

func getInstance(cfg *config.GlobalCfgValidated, key string) *url.URL {
	if !cfg.DistributedMode() {
		panic("getInstanceIndex called in non-distributed mode")
	}

	hash := hashKey(key)
	return cfg.Instances[int(hash)%len(cfg.Instances)]
}

func HandleRateRequest(
	cfg *config.GlobalCfgValidated,
	limiterManager *limiter_manager.LimiterManagerSet,
) echo.HandlerFunc {

	return func(c echo.Context) error {

		key := strings.TrimSpace(c.Param("key"))

		// Set up log context
		ctx := c.Request().Context()
		ctx = logctx.Add(ctx, "correlation-id", getCorrelationID(c))
		ctx = logctx.Add(ctx, "key", key)

		if len(key) == 0 {
			slog.Warn("empty key provided", logctx.GetAll(ctx)...)
			return c.String(http.StatusBadRequest, "empty key provided")
		}

		canWait, err := parseOptionalBoolParam(c.QueryParam("canWait"), false)
		if err != nil {
			slog.Warn("failed to parse canWait query parameter", logctx.GetAll(ctx)...)
			return c.String(http.StatusBadRequest, "failed to parse canWait query parameter")
		}

		maxRequests, err := parseOptionalInt32Param(c.QueryParam("maxRequests"), limiter_api.NoChange)
		if err != nil {
			slog.Warn("failed to parse maxRequests query parameter", logctx.GetAll(ctx)...)
			return c.String(http.StatusBadRequest, "failed to parse maxRequests query parameter")
		}

		if maxRequests != limiter_api.NoChange && !cfg.RequestsCanSetRate.Value() {
			slog.Warn("maxRequests query parameter is disabled", logctx.GetAll(ctx)...)
			return c.String(http.StatusForbidden, "maxRequests query parameter is disabled")
		}

		maxRequestsInQueue, err := parseOptionalInt32Param(c.QueryParam("maxRequestsInQueue"), limiter_api.NoChange)
		if err != nil {
			slog.Warn("failed to parse maxRequestsInQueue query parameter", logctx.GetAll(ctx)...)
			return c.String(http.StatusBadRequest, "failed to parse maxRequestsInQueue query parameter")
		}

		if maxRequestsInQueue != limiter_api.NoChange && !cfg.RequestsCanModQueue.Value() {
			slog.Warn("maxRequestsInQueue query parameter is disabled", logctx.GetAll(ctx)...)
			return c.String(http.StatusForbidden, "maxRequestsInQueue query parameter is disabled")
		}

		if maxRequests != limiter_api.NoChange {
			if err := cfg.MaxRequests.CustomValidator(maxRequests); err != nil {
				slog.Warn("maxRequests out of bounds", logctx.GetAll(ctx)...)
				return c.String(http.StatusBadRequest, "maxRequests out of bounds")
			}
		}

		if maxRequestsInQueue != limiter_api.NoChange {
			if err := cfg.MaxRequestsInQueue.CustomValidator(maxRequestsInQueue); err != nil {
				slog.Warn("maxRequestsInQueue out of bounds", logctx.GetAll(ctx)...)
				return c.String(http.StatusBadRequest, "maxRequestsInQueue out of bounds")
			}
		}

		// Check if we are the instance responsible for this key.
		// Otherwise, forward the request to the correct instance.
		err, forwarded := maybeForwardToCorrectInstance(c, cfg, key, ctx)
		if forwarded {
			return err
		}

		result, requestID := limiterManager.AskPermission(ctx, key, canWait, maxRequests, maxRequestsInQueue)

		switch result {
		case limiter_api.Approved:
			return c.String(http.StatusOK, requestID)
		case limiter_api.Denied:
			return c.NoContent(http.StatusTooManyRequests)
		case limiter_api.ClientGaveUp:
			return c.NoContent(499) // will never be returned to the client, so just pick a random status code
		default:
			slog.Error("unexpected response from limiter", append(logctx.GetAll(ctx), slog.String("response", string(result)))...)
			return c.NoContent(http.StatusInternalServerError)
		}

	}
}

func HandleReleaseRequest(
	cfg *config.GlobalCfgValidated,
	limiterManager *limiter_manager.LimiterManagerSet,
) echo.HandlerFunc {

	return func(c echo.Context) error {

		key := strings.TrimSpace(c.Param("key"))
		id := strings.TrimSpace(c.Param("id"))

		// Set up log context
		ctx := c.Request().Context()
		ctx = logctx.Add(ctx, "correlation-id", getCorrelationID(c))
		ctx = logctx.Add(ctx, "key", key)

		if len(key) == 0 {
			slog.Warn("empty key provided", logctx.GetAll(ctx)...)
			return c.String(http.StatusBadRequest, "empty key provided")
		}

		if len(id) == 0 {
			slog.Warn("empty id provided", logctx.GetAll(ctx)...)
			return c.String(http.StatusBadRequest, "empty id provided")
		}

		// Check if we are the instance responsible for this key.
		// Otherwise, forward the request to the correct instance.
		err, forwarded := maybeForwardToCorrectInstance(c, cfg, key, ctx)
		if forwarded {
			return err
		}

		limiterManager.Release(ctx, key, id)

		return c.NoContent(http.StatusOK)
	}
}

func maybeForwardToCorrectInstance(c echo.Context, cfg *config.GlobalCfgValidated, key string, ctx context.Context) (error, bool) {
	if cfg.DistributedMode() && c.QueryParam("ik") != "true" {
		correctInstance := getInstance(cfg, key)
		correctHostname := correctInstance.Hostname()
		requestHostName, _ := splitHostPort(c.Request().Host)
		if correctHostname != requestHostName {
			path := c.Request().URL.Path
			method := c.Request().Method
			slog.Debug(fmt.Sprintf("forwarding %s %s to correct instance %s", method, path, correctInstance.String()), logctx.GetAll(ctx)...)
			// We make a request ourselves to the correct instance
			query := "?ik=true" // avoid loops if we have a routing bug :S
			if len(c.Request().URL.RawQuery) > 0 {
				for k, vs := range c.Request().URL.Query() {
					if k != "ik" {
						for _, v := range vs {
							query += "&" + k + "=" + v
						}
					}
				}
			}
			uri := correctInstance.Scheme + "://" + correctInstance.Host + path + query
			req, err := http.NewRequest(method, uri, nil)
			if err != nil {
				slog.Warn(fmt.Sprintf("failed to forward request to correct instance: %v", err), logctx.GetAll(ctx)...)
				return c.String(http.StatusBadGateway, "failed to forward request to correct instance"), true
			}
			resp, err := http2Client.Do(req)
			if err != nil {
				slog.Warn(fmt.Sprintf("failed to forward request to correct instance: %v", err), logctx.GetAll(ctx)...)
				return c.String(http.StatusBadGateway, "failed to forward request to correct instance"), true
			} else {
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				return c.NoContent(resp.StatusCode), true
			}
		}
	}
	return nil, false
}

func getCorrelationID(c echo.Context) string {
	correlationId := c.Request().Header.Get("X-Correlation-ID")
	if correlationId == "" {
		correlationId = "gcc-" + uuid.New().String()
	}
	return correlationId
}

func parseOptionalBoolParam(raw string, defaultValue bool) (bool, error) {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) > 0 {
		return strconv.ParseBool(trimmed)
	}
	return defaultValue, nil
}

func parseOptionalInt32Param(raw string, defaultValue int) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) > 0 {
		i32, err := strconv.ParseInt(trimmed, 10, 32)
		if err != nil {
			return 0, err
		}
		return int(i32), nil
	}
	return defaultValue, nil
}

// Copy pasta below

// splitHostPort separates host and port. If the port is not valid, it returns
// the entire input as host, and it doesn't check the validity of the host.
// Unlike net.SplitHostPort, but per RFC 3986, it requires ports to be numeric.
func splitHostPort(hostPort string) (host, port string) {
	host = hostPort

	colon := strings.LastIndexByte(host, ':')
	if colon != -1 && validOptionalPort(host[colon:]) {
		host, port = host[:colon], host[colon+1:]
	}

	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}

	return
}

// validOptionalPort reports whether port is either an empty string
// or matches /^:\d*$/
func validOptionalPort(port string) bool {
	if port == "" {
		return true
	}
	if port[0] != ':' {
		return false
	}
	for _, b := range port[1:] {
		if b < '0' || b > '9' {
			return false
		}
	}
	return true
}
