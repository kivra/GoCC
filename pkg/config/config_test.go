package config

import (
	"encoding/json"
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/kivra/gocc/pkg/config/experimental/svc_discovery"
	"github.com/samber/lo"
	"log/slog"
	"os"
	"testing"
)

func TestParseAppConfigBytes(t *testing.T) {

	tempDir := os.TempDir() + "/app_config_test"
	fileName := "test.json"

	appCfg := &CfgFromFile{
		Keys: []CfgFromFileKey{
			{
				KeyPattern:           "key1",
				MaxRequestsPerWindow: 1,
				MaxRequestsInQueue:   2,
				WindowMillis:         3,
			},
			{
				KeyPattern:           "key2",
				KeyPatternIsRegex:    true,
				MaxRequestsPerWindow: 4,
				MaxRequestsInQueue:   5,
				WindowMillis:         6,
			},
		},
	}

	if !DirExists(tempDir) {
		if err := os.MkdirAll(tempDir, 0755); err != nil {
			t.Fatalf("Failed to create temp directory: %v", err)
		}
	}

	err := WriteAppConfigFile(tempDir+"/"+fileName, appCfg)
	if err != nil {
		t.Fatalf("Failed to write to file: %v", err)
	}

	cfg, err := ReadAppConfigFile(tempDir + "/" + fileName)
	if err != nil {
		t.Fatalf("Failed to read app config file: %v", err)
	}

	if cmp.Diff(appCfg, cfg) != "" {
		t.Fatalf("App config read from file does not match the original app config")
	}

	// Print the app config just to see the output
	jsonBytes, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Failed to marshal app config: %v", err)
	}
	fmt.Println(string(jsonBytes))
}

func TestValidateInstanceUrls(t *testing.T) {
	testCases := []struct {
		name         string
		instanceUrls []string
		expectError  bool
	}{
		{
			name: "Valid instance URLs",
			instanceUrls: []string{
				"https://" + svc_discovery.GetOwnHostName() + ":8080",
				"https://" + "other" + ":8081",
			},
			expectError: false,
		},
		{
			name: "Invalid URL",
			instanceUrls: []string{
				"https://" + svc_discovery.GetOwnHostName() + ":8080",
				"invalid-url",
			},
			expectError: true,
		},
		{
			name: "Single valid instance URL",
			instanceUrls: []string{
				"https://" + svc_discovery.GetOwnHostName() + ":8080",
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &GlobalCfg{}
			cfg.InstanceUrls.Default = lo.ToPtr(tc.instanceUrls)

			vCfg, err := cfg.ValidateInstanceUrls()
			if tc.expectError && err == nil {
				t.Fatalf("Expected error, got nil")
			}
			if !tc.expectError && err != nil {
				t.Fatalf("Failed to validate instance urls: %v", err)
			}
			if tc.expectError && err != nil {
				slog.Error(fmt.Sprintf("correctly got error: %v", err))
			}

			if !tc.expectError && vCfg == nil {
				t.Fatalf("Failed to validate instance urls: returned nil")
			}

			if tc.expectError && vCfg != nil {
				t.Fatalf("Expected nil, got %v", vCfg)
			}
		})
	}
}
