package services

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	PluginFetchTimeout = 30 * time.Second
	DNSLookupTimeout   = 5 * time.Second
)

var allowedProtocols = map[string]bool{
	"http":  true,
	"https": true,
}

func isPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() || ip.IsUnspecified() {
		return true
	}

	if ip4 := ip.To4(); ip4 != nil {
		return ip4.IsPrivate() || ip4.IsLoopback() || ip4.IsLinkLocalUnicast() || ip4.IsUnspecified()
	}

	if len(ip) == net.IPv6len && (ip[0]&0xfe) == 0xfc {
		return true
	}

	return false
}

type ValidatedFetchTarget struct {
	ParsedURL       *url.URL
	ResolvedAddress string
	HostHeader      string
	TLSServerName   string
	UseTLS          bool
}

func ValidateAndResolveFetchURL(ctx context.Context, urlString string) (*ValidatedFetchTarget, error) {
	parsed, err := url.Parse(urlString)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %s", urlString)
	}

	if !allowedProtocols[parsed.Scheme] {
		return nil, fmt.Errorf("disallowed protocol %q - only http and https are permitted", parsed.Scheme)
	}

	host := parsed.Hostname()
	hostHeader := parsed.Host

	resolver := &net.Resolver{}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, DNSLookupTimeout)
	defer cancel()

	ips, err := resolver.LookupIPAddr(ctxWithTimeout, host)
	if err != nil {
		return nil, fmt.Errorf("DNS resolution failed for %s: %w", host, err)
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("DNS resolution returned no results for %s", host)
	}

	var resolvedIP string
	for _, ip := range ips {
		if !isPrivateIP(ip.String()) {
			resolvedIP = ip.String()
			break
		}
	}

	if resolvedIP == "" {
		return nil, fmt.Errorf("all resolved IPs for %s are in private/reserved ranges", host)
	}

	tlsServerName := ""
	if parsed.Scheme == "https" && net.ParseIP(host) == nil {
		tlsServerName = host
	}

	return &ValidatedFetchTarget{
		ParsedURL:       parsed,
		ResolvedAddress: resolvedIP,
		HostHeader:      hostHeader,
		TLSServerName:   tlsServerName,
		UseTLS:          parsed.Scheme == "https",
	}, nil
}

func BuildSafeHTTPClient(target *ValidatedFetchTarget) *http.Client {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	port := target.ParsedURL.Port()
	if port == "" {
		if target.UseTLS {
			port = "443"
		} else {
			port = "80"
		}
	}

	addr := net.JoinHostPort(target.ResolvedAddress, port)

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   PluginFetchTimeout,
	}
}

func SafeFetch(ctx context.Context, method, urlStr string, headers map[string]string) (*http.Response, error) {
	target, err := ValidateAndResolveFetchURL(ctx, urlStr)
	if err != nil {
		return nil, err
	}

	client := BuildSafeHTTPClient(target)

	reqURL := *target.ParsedURL

	req, err := http.NewRequestWithContext(ctx, method, reqURL.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Host = target.HostHeader

	for k, v := range headers {
		if strings.ToLower(k) == "host" {
			continue
		}
		req.Header.Set(k, v)
	}

	return client.Do(req)
}
