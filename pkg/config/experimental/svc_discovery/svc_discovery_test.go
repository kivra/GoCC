package svc_discovery

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"log/slog"
	"testing"
)

func TestGetOwnHostName(t *testing.T) {
	hostname := GetOwnHostName()
	if hostname == "" {
		t.Fatalf("GetOwnHostName() = %v; want non-empty string", hostname)
	}

	slog.Info("Own hostname", slog.String("hostname", hostname))
}

func TestInferHeadlessSvcName(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		want     string
		wantErr  bool
	}{
		{
			name:     "stateful set",
			hostname: "my-stateful-set-0.svc.cluster.local",
			want:     "my-stateful-set",
			wantErr:  false,
		},
		{
			name:     "deployment",
			hostname: "my-deployment-asda98h7f9-3912823",
			want:     "my-deployment",
			wantErr:  false,
		},
		{
			name:     "deployment",
			hostname: "bob",
			want:     "bob",
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := InferHeadlessSvcName(tt.hostname)
			if (err != nil) != tt.wantErr {
				t.Fatalf("InferHeadlessSvcName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Fatalf("InferHeadlessSvcName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMakeDnsLookup(t *testing.T) {
	hostname := GetOwnHostName()
	ips, err := MakeDnsLookup(hostname)
	if err != nil {
		t.Fatalf("MakeDnsLookup() error = %v; want nil", err)
	}
	for _, ip := range ips {
		slog.Info("IP address", slog.String("ip", ip))
	}

	googleIps, err := MakeDnsLookup("google.com")
	if err != nil {
		t.Fatalf("MakeDnsLookup() error = %v; want nil", err)
	}
	for _, ip := range googleIps {
		slog.Info("Google IP address", slog.String("ip", ip))
	}
}

func TestIsIpV4(t *testing.T) {
	validAddresses := []string{
		"127.0.0.1",
		"172.18.9.24",
	}

	invalidAddresses := []string{
		"::1",
		"2a00:1450:400f:801::200e",
		"banana",
	}

	for _, addr := range validAddresses {
		if !IsIpV4(addr) {
			t.Fatalf("IsIpV4(%s) = false; want true", addr)
		}
		if IsIpV6(addr) {
			t.Fatalf("IsIpV6(%s) = true; want false", addr)
		}
	}

	for _, addr := range invalidAddresses {
		if IsIpV4(addr) {
			t.Fatalf("IsIpV4(%s) = true; want false", addr)
		}
	}
}

func TestIsIpV6(t *testing.T) {
	validAddresses := []string{
		"::1",
		"2a00:1450:400f:801::200e",
	}

	invalidAddresses := []string{
		"127.0.0.1",
		"172.18.9.24",
		"banana",
	}

	for _, addr := range validAddresses {
		if !IsIpV6(addr) {
			t.Fatalf("IsIpV6(%s) = false; want true", addr)
		}
		if IsIpV4(addr) {
			t.Fatalf("IsIpV4(%s) = true; want false", addr)
		}
	}

	for _, addr := range invalidAddresses {
		if IsIpV6(addr) {
			t.Fatalf("IsIpV6(%s) = true; want false", addr)
		}
	}
}

func TestGetOwnIPs(t *testing.T) {
	ips, err := GetOwnIPs()
	if err != nil {
		t.Fatalf("GetOwnIPs() error = %v; want nil", err)
	}
	for _, ip := range ips {
		slog.Info("IP address", slog.String("ip", ip))
	}
}

func TestFilterAwayOwnIPs(t *testing.T) {
	ownIps, err := GetOwnIPs()
	if err != nil {
		t.Fatalf("GetOwnIPs() error = %v; want nil", err)
	}

	ips := []string{
		"121.121.121.123",
		ownIps[0],
		"121.121.121.121",
	}

	ips = append(ips, ownIps...)

	filteredIPs, err := FilterAwayOwnIPs(ips)
	if err != nil {
		t.Fatalf("FilterAwayOwnIPs() error = %v; want nil", err)
	}

	expIPs := []string{
		"121.121.121.123",
		"121.121.121.121",
	}

	if diff := cmp.Diff(expIPs, filteredIPs); diff != "" {
		t.Fatalf("FilterAwayOwnIPs() mismatch (-want +got):\n%s", diff)
	}
}

func TestGetInstanceList(t *testing.T) {
	_, err := GetInstanceList()
	if err != nil {
		// Can't really run this test automated. Needs a kubernetes enviornment
		slog.Error(fmt.Sprintf("Error getting instance list: %v", err))
	}

	//for _, instance := range instances {
	//	slog.Info("Instance", slog.String("instance", instance))
	//}
}
