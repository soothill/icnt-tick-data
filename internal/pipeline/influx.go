// Copyright (c) 2025 Darren Soothill (darren [at] soothill [dot] com)

package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

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
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/api/v2/write"
	query := parsed.Query()
	query.Set("org", config.InfluxOrg)
	query.Set("bucket", config.InfluxBucket)
	query.Set("precision", "ns")
	parsed.RawQuery = query.Encode()
	url := parsed.String()
	client := &http.Client{Timeout: config.InfluxTimeout}
	return &InfluxWriter{config: config, client: client, url: url}, nil
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
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("influx write failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
