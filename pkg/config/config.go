package config

import (
	"cmp"
	"encoding/json"
	"fmt"
	"github.com/GiGurra/boa/pkg/boa"
	"github.com/samber/lo"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type ServerType string

const (
	ServerTypeEcho      ServerType = "echo"
	ServerTypeEchohttp2 ServerType = "echo-http2"
	ServerTypeFast      ServerType = "fast"
)

type GlobalCfg struct {
	MaxRequests         boa.Required[int]      `default:"100"        env:"MAX_REQUESTS"           descr:"Default max requests per window per key"`
	MaxRequestsInQueue  boa.Required[int]      `default:"400"        env:"MAX_REQUESTS_IN_QUEUE"  descr:"Default max requests in queue per key"`
	WindowMillis        boa.Required[int]      `default:"1000"       env:"WINDOW_MILLIS"          descr:"Default size in milliseconds per window"`
	RequestsCanSetRate  boa.Required[bool]     `default:"true"       env:"REQUESTS_CAN_SET_RATE"  descr:"Allow clients to set their own rate"`
	RequestsCanModQueue boa.Required[bool]     `default:"true"       env:"REQUESTS_CAN_MOD_QUEUE" descr:"Allow clients to set their own queue size"`
	ConfigFile          boa.Required[string]   `default:""           env:"CONFIG_FILE"            descr:"Path to a JSON file with key-specific rate limits"`
	Port                boa.Required[int]      `default:"8080"       env:"PORT"                   descr:"Port to listen on"`
	LogFormat           boa.Required[string]   `default:"json"       env:"LOG_FORMAT"             descr:"json,text,system-default"`
	LogLevel            boa.Required[string]   `default:"INFO"       env:"LOG_LEVEL"              descr:"DEBUG,INFO,WARN,ERROR"`
	LogIncludesSource   boa.Required[bool]     `default:"true"       env:"LOG_INCLUDES_SOURCE"    descr:"if true, log messages include the source code location"`
	Log2xx              boa.Required[bool]     `default:"false"      env:"LOG_2XX"                descr:"if true, log 2xx responses"`
	Log4xx              boa.Required[bool]     `default:"false"      env:"LOG_4XX"                descr:"if true, log 4xx responses. Includes rate limit exceeded responses"`
	Log5xx              boa.Required[bool]     `default:"true"       env:"LOG_5XX"                descr:"if true, log 5xx responses"`
	ServerType          boa.Required[string]   `default:"echo-http2" env:"SERVER_TYPE"            descr:"echo,echo-http2,fast. 'fast' is a fasthttp server, not fully implemented yet"`
	InstanceUrls        boa.Required[[]string] `default:"[]"         env:"INSTANCE_URLS"          descr:"For distributed mode, a list of instance urls to use (incl this instance)"`
}

type GlobalCfgValidated struct {
	*GlobalCfg
	Instances []*url.URL
}

func (c *GlobalCfg) ValidateInstanceUrls() (*GlobalCfgValidated, error) {

	result := &GlobalCfgValidated{GlobalCfg: c}

	instanceUrlStrings := c.InstanceUrls.Value()

	if len(lo.Uniq(instanceUrlStrings)) != len(instanceUrlStrings) {
		return nil, fmt.Errorf("duplicate instance urls provided")
	}

	if len(instanceUrlStrings) == 1 {
		return nil, fmt.Errorf("only one instance url provided, distributed mode requires at least 2. Omit this setting to run in single instance mode")
	}

	if len(instanceUrlStrings) != 0 {
		slog.Info(fmt.Sprintf("%d instance urls provided, preparing for distributed mode", len(instanceUrlStrings)))
		// we must ensure these are valid, and that our own hostname corresponds to one of these.
		// Otherwise, we know the configuration is wrong.
		instances := make([]*url.URL, 0, len(c.InstanceUrls.Value()))
		for _, instanceUrl := range c.InstanceUrls.Value() {
			instanceUrl = strings.TrimSpace(instanceUrl)
			slog.Info(fmt.Sprintf("Instance url: %v", instanceUrl))
			if len(strings.TrimSpace(instanceUrl)) == 0 {
				return nil, fmt.Errorf("instance url '%s' is empty", instanceUrl)
			}
			parsedUrl, err := url.Parse(instanceUrl)
			if err != nil {
				return nil, fmt.Errorf("invalid instance url: '%v'", instanceUrl)
			}
			if !parsedUrl.IsAbs() {
				return nil, fmt.Errorf("instance url '%s' is not absolute. Only absolute urls are supported", instanceUrl)
			}
			instances = append(instances, parsedUrl)
		}

		result.Instances = instances

		return result, nil

	} else {
		return result, nil
	}
}

func (c *GlobalCfgValidated) DistributedMode() bool {
	return len(c.Instances) > 0
}

func NewGlobalCfg() *GlobalCfg {
	cfg := &GlobalCfg{}
	cfg.MaxRequests.CustomValidator = minMax(1, 1_000_000_000)
	cfg.MaxRequestsInQueue.CustomValidator = minMax(0, 1_000_000_000)
	cfg.WindowMillis.CustomValidator = minMax(10, 3600*1000)
	cfg.Port.CustomValidator = minMax(0, 65_535) // 0 = ephemeral port
	cfg.LogFormat.CustomValidator = oneOf("json", "text", "system-default")
	cfg.LogLevel.CustomValidator = oneOf("DEBUG", "INFO", "WARN", "ERROR")
	cfg.ServerType.CustomValidator = oneOf("echo", "echo-http2", "fast")
	return cfg
}

type CfgFromFile struct {
	Keys []CfgFromFileKey `json:"keys"`
}

// MonitorConfigFromFile reads the config file and sets up a monitor for changes.
func MonitorConfigFromFile(path string) (*CfgFromFile, *JsonFileChangeMonitor[*CfgFromFile]) {
	initAppConfigFromFile := &CfgFromFile{}
	var configFileChangeMonitor *JsonFileChangeMonitor[*CfgFromFile] = nil
	if path != "" {
		configDir, configFile := filepath.Split(path)
		if configDir == "" {
			slog.Warn("No directory provided in config file path, using current directory", slog.String("configFile", path))
			configDir = "."
		}
		if configFile == "" {
			panic("config file path is empty")
		}

		// Read the config file
		var err error
		initAppConfigFromFile, err = ReadAppConfigFile(path)
		if err != nil {
			panic(fmt.Sprintf("Failed to read config file: %v", err))
		}

		slog.Info("Config file read successfully", slog.String("configFile", path))
		slog.Info("Config values:")
		for _, key := range initAppConfigFromFile.Keys {
			slog.Info(fmt.Sprintf(" - %s", key.ToJson()))
		}
		configFileChangeMonitor = MonitorJsonFileUpdates[*CfgFromFile](configDir, configFile, StringTransformer)
	} else {
		slog.Warn("No config file provided, no key specific rate limits will be used (unless clients set them)", slog.String("configFile", path))
	}
	return initAppConfigFromFile, configFileChangeMonitor
}

type CfgFromFileKey struct {
	KeyPattern           string `json:"key_pattern"`
	KeyPatternIsRegex    bool   `json:"key_pattern_is_regex"`
	MaxRequestsPerWindow int    `json:"max_requests_per_window"`
	MaxRequestsInQueue   int    `json:"max_requests_in_queue"`
	WindowMillis         int    `json:"window_millis"`
}

func (c *CfgFromFileKey) ToJson() string {
	jsBytes, err := json.Marshal(c)
	if err != nil {
		panic(fmt.Sprintf("BUG: failed to marshal CfgFromFileKey to json. This should not be possible: %v", err))
	}
	return string(jsBytes)
}

func (c *CfgFromFileKey) MatchesKey(str string) bool {
	if c.KeyPatternIsRegex {
		compiledRegex, err := regexp.Compile(c.KeyPattern)
		if err != nil {
			slog.Error(fmt.Sprintf("Failed to compile regex: '%s', due to: %v", str, err))
			return false
		}
		return compiledRegex.MatchString(str)
	} else {
		return c.KeyPattern == str
	}
}

func ParseAppConfigBytes(data []byte) (*CfgFromFile, error) {
	var cfg CfgFromFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal App Config json: %w", err)
	}
	return &cfg, nil
}

func ParseAppConfigString(str string) (*CfgFromFile, error) {
	return ParseAppConfigBytes([]byte(str))
}

func ReadAppConfigFile(filePath string) (*CfgFromFile, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %v: %w", filePath, err)
	}

	cfg, err := ParseAppConfigBytes(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse App Config from file %v: %w", filePath, err)
	}

	return cfg, nil
}

func StringTransformer(in string) (*CfgFromFile, bool) {
	config, err := ParseAppConfigString(in)
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to parse config file: %v", err))
		return nil, false
	}
	return config, true
}

func WriteAppConfigFile(path string, cfg *CfgFromFile) error {
	// write to file
	jsBytes, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal json: %w", err)
	}

	if err := os.WriteFile(path, jsBytes, 0644); err != nil {
		return fmt.Errorf("failed to write to file: %w", err)
	}

	return nil
}

func minMax[T cmp.Ordered](min T, max T) func(t T) error {
	return func(t T) error {
		if t < min {
			return fmt.Errorf("value must be at least %v", min)
		}
		if t > max {
			return fmt.Errorf("value must be at most %v", max)
		}
		return nil
	}
}

func oneOf(validValues ...string) func(t string) error {
	return func(t string) error {
		for _, v := range validValues {
			if t == v {
				return nil
			}
		}
		return fmt.Errorf("value must be one of %v", validValues)
	}
}
