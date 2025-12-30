// Copyright (c) 2025 Darren Soothill (darren [at] soothill [dot] com)

package pipeline

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBackoffNextAndReset(t *testing.T) {
	backoff := NewBackoff(time.Second, 8*time.Second)
	if got := backoff.Next(); got != time.Second {
		t.Fatalf("expected first delay 1s, got %v", got)
	}
	if got := backoff.Next(); got != 2*time.Second {
		t.Fatalf("expected second delay 2s, got %v", got)
	}
	if got := backoff.Next(); got != 4*time.Second {
		t.Fatalf("expected third delay 4s, got %v", got)
	}
	if got := backoff.Next(); got != 8*time.Second {
		t.Fatalf("expected capped delay 8s, got %v", got)
	}
	backoff.Reset()
	if got := backoff.Next(); got != time.Second {
		t.Fatalf("expected reset to base delay, got %v", got)
	}
}

func TestNormalizePair(t *testing.T) {
	got := normalizePair("ICNT/USD-foo")
	if got != "ICNTUSDfoo" {
		t.Fatalf("unexpected normalized pair: %s", got)
	}
}

func TestLoadConfigAdditionalParsing(t *testing.T) {
	t.Setenv("KRAKEN_PAIR", "")
	t.Setenv("KRAKEN_WS_URL", "")
	t.Setenv("KRAKEN_REST_URL", "")
	t.Setenv("CACHE_DB_PATH", "")
	t.Setenv("INFLUX_URL", "")
	t.Setenv("INFLUX_TIMEOUT_SEC", "")
	t.Setenv("BACKFILL_INTERVAL_SEC", "")
	t.Setenv("AGGREGATE_INTERVAL_SEC", "")
	t.Setenv("FLUSH_INTERVAL_SEC", "")
	t.Setenv("BATCH_SIZE", "")
	t.Setenv("INFLUX_DISABLED", "")

	cfg := LoadConfig()
	if cfg.KrakenPair != "ICNT/USD" {
		t.Fatalf("expected default pair ICNT/USD, got %s", cfg.KrakenPair)
	}
	if cfg.InfluxTimeout != 10*time.Second {
		t.Fatalf("expected default influx timeout 10s, got %v", cfg.InfluxTimeout)
	}

	t.Setenv("KRAKEN_PAIR", "FOO/BAR")
	t.Setenv("KRAKEN_REST_PAIR", "FOOBAR")
	t.Setenv("INFLUX_TIMEOUT_SEC", "3")
	t.Setenv("BACKFILL_INTERVAL_SEC", "15")
	t.Setenv("AGGREGATE_INTERVAL_SEC", "9")
	t.Setenv("FLUSH_INTERVAL_SEC", "2")
	t.Setenv("BATCH_SIZE", "123")
	t.Setenv("INFLUX_DISABLED", "true")

	cfg = LoadConfig()
	if cfg.KrakenPair != "FOO/BAR" || cfg.KrakenRestPair != "FOOBAR" {
		t.Fatalf("unexpected pair parsing: %+v", cfg)
	}
	if cfg.InfluxTimeout != 3*time.Second {
		t.Fatalf("unexpected influx timeout: %v", cfg.InfluxTimeout)
	}
	if cfg.BackfillInterval != 15*time.Second || cfg.AggregateInterval != 9*time.Second || cfg.FlushInterval != 2*time.Second {
		t.Fatalf("unexpected interval parsing: %+v", cfg)
	}
	if cfg.BatchSize != 123 {
		t.Fatalf("unexpected batch size: %d", cfg.BatchSize)
	}
	if !cfg.InfluxDisabled {
		t.Fatalf("expected influx disabled to parse as true")
	}
}

func TestEscapeAndLineBuilders(t *testing.T) {
	tag := escapeTag(`ICNT/USD foo,bar=1\2`)
	if tag != `ICNT/USD\ foo\,bar\=1\\2` {
		t.Fatalf("unexpected escaped tag: %s", tag)
	}
	field := escapeFieldString(`abc "quoted"`)
	if field != `abc \"quoted\"` {
		t.Fatalf("unexpected escaped field: %s", field)
	}

	tick := Tick{
		TradeID: "t123",
		TSNS:    123,
		Price:   1.23,
		Volume:  0.5,
		Side:    "buy",
	}
	line := tickToLine("meas", "PAIR", tick)
	expectedTick := `meas,pair=PAIR,side=buy price=1.23,volume=0.5,trade_id="t123" 123`
	if line != expectedTick {
		t.Fatalf("unexpected tick line: %s", line)
	}

	bar := MinuteBar{
		MinuteTS:   456,
		Open:       1,
		High:       3,
		Low:        1,
		Close:      2,
		Volume:     10,
		TradeCount: 5,
	}
	line = minuteToLine("mins", "PAIR", bar)
	expectedMin := `mins,pair=PAIR open=1,high=3,low=1,close=2,volume=10,trade_count=5i 456`
	if line != expectedMin {
		t.Fatalf("unexpected minute line: %s", line)
	}
}

func TestDefaultPort(t *testing.T) {
	if got := defaultPort("http"); got != "80" {
		t.Fatalf("expected port 80, got %s", got)
	}
	if got := defaultPort("https"); got != "443" {
		t.Fatalf("expected port 443, got %s", got)
	}
	if got := defaultPort("tcp"); got != "" {
		t.Fatalf("expected empty port, got %s", got)
	}
}

func TestParseHelpers(t *testing.T) {
	if v, err := parseString(1.25); err != nil || v != "1.25" {
		t.Fatalf("unexpected parseString result: %s %v", v, err)
	}
	if v, err := parseFloat("2.5"); err != nil || v != 2.5 {
		t.Fatalf("unexpected parseFloat result: %v %v", v, err)
	}
	if v, err := parseFloat(json.Number("3.5")); err != nil || v != 3.5 {
		t.Fatalf("unexpected parseFloat(json.Number) result: %v %v", v, err)
	}

	ts, err := parseTimestampNS("1700000000.123456789")
	if err != nil {
		t.Fatalf("parseTimestampNS error: %v", err)
	}
	if ts != 1700000000123456789 {
		t.Fatalf("unexpected timestamp: %d", ts)
	}

	ts, err = parseTimestampNS("1700000000.123456789999")
	if err != nil {
		t.Fatalf("parseTimestampNS error: %v", err)
	}
	if ts != 1700000000123456789 {
		t.Fatalf("unexpected truncated timestamp: %d", ts)
	}
}

func TestParseTradeRowAdditional(t *testing.T) {
	row := []interface{}{"1.0", "2.0", "1700000000.5", "b", "l", "m", "abc123"}
	trade, err := parseTradeRow(row, "ws")
	if err != nil {
		t.Fatalf("parseTradeRow error: %v", err)
	}
	if trade.TradeID != "abc123" || trade.Source != "ws" {
		t.Fatalf("unexpected trade: %+v", trade)
	}

	rowNoID := []interface{}{"1.0", "2.0", "1700000000.5", "b", "l", "m"}
	trade2, err := parseTradeRow(rowNoID, "rest")
	if err != nil {
		t.Fatalf("parseTradeRow without id error: %v", err)
	}
	if trade2.TradeID == "" || trade2.Source != "rest" {
		t.Fatalf("unexpected trade without id: %+v", trade2)
	}
	if trade2.TradeID[:5] != "rest-" {
		t.Fatalf("expected rest- prefix, got %s", trade2.TradeID)
	}
	if trade2.TradeID != hashTradeID(trade2.TSNS, trade2.Price, trade2.Volume, trade2.Side, trade2.OrderType, trade2.Misc) {
		t.Fatalf("hashTradeID not deterministic")
	}
}

func TestLogHelpersDontPanicWithoutSyslog(t *testing.T) {
	// Ensure logging helpers are safe even if syslog is unavailable.
	LogInfof("test info log")
	LogFailuref("test failure log")
}
