package svc_discovery

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
)

func IsIpV4(ip string) bool {
	parsedIP := net.ParseIP(ip)
	return parsedIP != nil && parsedIP.To4() != nil
}

func IsIpV6(ip string) bool {
	parsedIP := net.ParseIP(ip)
	return parsedIP != nil && parsedIP.To4() == nil
}

func GetOwnIPs() ([]string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("error getting own hostname: %v", err)
	}

	return MakeDnsLookup(hostname)
}

func FilterAwayOwnIPs(ips []string) ([]string, error) {
	ownIPs, err := GetOwnIPs()
	if err != nil {
		return nil, fmt.Errorf("error getting own IPs: %v", err)
	}

	var filteredIPs []string
	for _, ip := range ips {
		if !slices.Contains(ownIPs, ip) {
			filteredIPs = append(filteredIPs, ip)
		}
	}

	return filteredIPs, nil
}

func MakeDnsLookup(hostname string) ([]string, error) {

	ips, err := net.LookupIP(hostname)
	if err != nil {
		return nil, fmt.Errorf("error looking up IP for hostname: %v", err)
	}

	var ipStrings []string
	for _, ip := range ips {
		ipStrings = append(ipStrings, ip.String())
	}

	return ipStrings, nil
}

func GetOwnHostName() string {
	hostname, err := os.Hostname()
	if err != nil {
		panic(fmt.Sprintf("Error getting own hostname: %v", err))
	}
	return hostname
}

func InferHeadlessSvcName(hostname string) (string, error) {

	dotParts := strings.Split(hostname, ".")
	if len(dotParts) == 0 {
		return "", fmt.Errorf("invalid hostname: %s", hostname)
	}

	dashParts := strings.Split(dotParts[0], "-")
	if len(dashParts) == 0 {
		return "", fmt.Errorf("invalid hostname: %s", hostname)
	}

	// if last part is shorter than 4 and a number, it's probably a stateful set pod index
	lastPart := dashParts[len(dashParts)-1]
	if len(lastPart) < 4 {
		_, err := strconv.Atoi(lastPart)
		if err == nil {
			slog.Info("last part of hostname is a number, so it's probably a stateful set pod index, returning stateful set name", slog.String("hostname", hostname))
			return strings.Join(dashParts[0:len(dashParts)-1], "-"), nil
		} else {
			slog.Info("last part of hostname is not a number, so it's probably not a stateful set pod index", slog.String("hostname", hostname))
			// not a number, so not a stateful set pod index
		}
	}

	if len(dashParts) < 3 {
		slog.Warn("hostname has less than 3 parts, so it's not a deployment or stateful set pod, returning original hostname", slog.String("hostname", hostname))
		return hostname, nil
	}

	slog.Info("hostname is probably a deployment pod, returning deployment name", slog.String("hostname", hostname))
	return strings.Join(dashParts[0:len(dashParts)-2], "-"), nil
}

func GetInstanceList() ([]string, error) {
	hostname := GetOwnHostName()
	svcName, err := InferHeadlessSvcName(hostname)
	if err != nil {
		return nil, fmt.Errorf("failed to infer headless service name: %w", err)
	}

	slog.Info("Inferred headless service name", slog.String("svcName", svcName))

	ips, err := MakeDnsLookup(svcName)
	if err != nil {
		return nil, fmt.Errorf("failed to make DNS lookup: %w", err)
	}

	slog.Info("Headless service IPs", slog.String("ips", strings.Join(ips, ",")))

	otherIps, err := FilterAwayOwnIPs(ips)
	if err != nil {
		return nil, fmt.Errorf("failed to filter away own IPs: %w", err)
	}

	slog.Info("Other headless service IPs", slog.String("ips", strings.Join(otherIps, ",")))

	if len(otherIps) == 0 {
		return nil, fmt.Errorf("no other instances found")
	}

	slog.Info("Returning headless service IPs", slog.String("ips", strings.Join(otherIps, ",")))
	return ips, nil
}
