// Copyright (c) 2025 Darren Soothill (darren [at] soothill [dot] com)

package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

type hostResolver interface {
	LookupHost(ctx context.Context, host string) ([]string, error)
}

func escapeTag(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		" ", "\\ ",
		",", "\\,",
		"=", "\\=",
	)
	return replacer.Replace(value)
}

func escapeFieldString(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"\"", "\\\"",
	)
	return replacer.Replace(value)
}

func tickToLine(measurement, pair string, tick Tick) string {
	tags := fmt.Sprintf("pair=%s,side=%s", escapeTag(pair), escapeTag(tick.Side))
	fields := fmt.Sprintf(
		"price=%v,volume=%v,trade_id=\"%s\"",
		tick.Price,
		tick.Volume,
		escapeFieldString(tick.TradeID),
	)
	return fmt.Sprintf("%s,%s %s %d", measurement, tags, fields, tick.TSNS)
}

func minuteToLine(measurement, pair string, bar MinuteBar) string {
	tags := fmt.Sprintf("pair=%s", escapeTag(pair))
	fields := fmt.Sprintf(
		"open=%v,high=%v,low=%v,close=%v,volume=%v,trade_count=%di",
		bar.Open,
		bar.High,
		bar.Low,
		bar.Close,
		bar.Volume,
		bar.TradeCount,
	)
	return fmt.Sprintf("%s,%s %s %d", measurement, tags, fields, bar.MinuteTS)
}

type InfluxWriter struct {
	config Config
	client *http.Client
	url    string

	host     string
	port     string
	resolver hostResolver

	mu       sync.RWMutex
	cachedIP string
}

func NewInfluxWriter(config Config) (*InfluxWriter, error) {
	if config.InfluxOrg == "" || config.InfluxBucket == "" || config.InfluxToken == "" {
		return nil, fmt.Errorf("INFLUX_ORG, INFLUX_BUCKET, and INFLUX_TOKEN are required")
	}
	baseURL := strings.TrimRight(config.InfluxURL, "/")
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	if host := parsed.Hostname(); host != "" && host != "localhost" && net.ParseIP(host) == nil && !strings.Contains(host, ".") {
		logFailuref("warning: INFLUX_URL host %q looks like a short name; prefer localhost, an IP, or a fully-qualified hostname", host)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/api/v2/write"
	query := parsed.Query()
	query.Set("org", config.InfluxOrg)
	query.Set("bucket", config.InfluxBucket)
	query.Set("precision", "ns")
	parsed.RawQuery = query.Encode()
	url := parsed.String()
	port := parsed.Port()
	if port == "" {
		port = defaultPort(parsed.Scheme)
	}
	if port == "" {
		return nil, fmt.Errorf("influx URL missing port: %s", config.InfluxURL)
	}

	writer := &InfluxWriter{
		config:   config,
		url:      url,
		host:     parsed.Hostname(),
		port:     port,
		resolver: net.DefaultResolver,
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{Timeout: config.InfluxTimeout}
	transport.DialContext = writer.dialContext(dialer)
	writer.client = &http.Client{
		Timeout:   config.InfluxTimeout,
		Transport: transport,
	}
	return writer, nil
}

func (w *InfluxWriter) SendLines(ctx context.Context, lines []string) error {
	if len(lines) == 0 {
		return nil
	}
	payload := bytes.NewBufferString(strings.Join(lines, "\n"))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Token %s", w.config.InfluxToken))
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("influx write failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (w *InfluxWriter) dialContext(dialer *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			host = w.host
			port = w.port
		}
		ip, err := w.resolveIP(ctx, host)
		if err != nil {
			return nil, err
		}
		if port == "" {
			port = w.port
		}
		target := net.JoinHostPort(ip, port)
		return dialer.DialContext(ctx, network, target)
	}
}

func (w *InfluxWriter) resolveIP(ctx context.Context, host string) (string, error) {
	if ip := net.ParseIP(host); ip != nil {
		w.setCachedIP(host)
		return host, nil
	}
	resolver := w.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	ips, err := resolver.LookupHost(ctx, host)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if cached := w.getCachedIP(); cached != "" {
			logFailuref("using cached InfluxDB IP %s for host %s after DNS error: %v", cached, host, err)
			return cached, nil
		}
		return "", fmt.Errorf("dns lookup failed for InfluxDB host %q (set INFLUX_URL to localhost, an IP, or an FQDN): %w", host, err)
	}
	if len(ips) == 0 {
		if cached := w.getCachedIP(); cached != "" {
			logFailuref("using cached InfluxDB IP %s for host %s: DNS returned no results", cached, host)
			return cached, nil
		}
		return "", fmt.Errorf("no IPs resolved for %s", host)
	}
	ip := ips[0]
	w.setCachedIP(ip)
	return ip, nil
}

func (w *InfluxWriter) getCachedIP() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.cachedIP
}

func (w *InfluxWriter) setCachedIP(ip string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cachedIP = ip
}

func defaultPort(scheme string) string {
	switch strings.ToLower(scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}
