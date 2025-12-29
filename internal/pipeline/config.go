// Copyright (c) 2025 Darren Soothill (darren [at] soothill [dot] com)

package pipeline

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	KrakenPair            string
	KrakenRestPair        string
	KrakenWSURL           string
	KrakenRESTURL         string
	CacheDBPath           string
	InfluxURL             string
	InfluxOrg             string
	InfluxBucket          string
	InfluxToken           string
	InfluxTimeout         time.Duration
	InfluxMeasurementTick string
	InfluxMeasurementMin  string
	FlushInterval         time.Duration
	BackfillInterval      time.Duration
	AggregateInterval     time.Duration
	BatchSize             int
	InfluxDisabled        bool
}

func envString(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	value = strings.ToLower(value)
	return value == "1" || value == "true" || value == "yes"
}

func envDurationSeconds(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return time.Duration(parsed) * time.Second
}

func normalizePair(pair string) string {
	pair = strings.ReplaceAll(pair, "/", "")
	pair = strings.ReplaceAll(pair, "-", "")
	return pair
}

func LoadConfig() Config {
	pair := envString("KRAKEN_PAIR", "ICNT/USD")
	restPair := envString("KRAKEN_REST_PAIR", "")
	if restPair == "" {
		restPair = normalizePair(pair)
	}
	return Config{
		KrakenPair:            pair,
		KrakenRestPair:        restPair,
		KrakenWSURL:           envString("KRAKEN_WS_URL", "wss://ws.kraken.com"),
		KrakenRESTURL:         envString("KRAKEN_REST_URL", "https://api.kraken.com/0/public/Trades"),
		CacheDBPath:           envString("CACHE_DB_PATH", "cache/ticks.sqlite"),
		InfluxURL:             envString("INFLUX_URL", "http://localhost:8086"),
		InfluxOrg:             envString("INFLUX_ORG", ""),
		InfluxBucket:          envString("INFLUX_BUCKET", ""),
		InfluxToken:           envString("INFLUX_TOKEN", ""),
		InfluxTimeout:         envDurationSeconds("INFLUX_TIMEOUT_SEC", 10*time.Second),
		InfluxMeasurementTick: envString("INFLUX_MEASUREMENT_TICKS", "icnt_usd_ticks"),
		InfluxMeasurementMin:  envString("INFLUX_MEASUREMENT_MINUTES", "icnt_usd_minutes"),
		FlushInterval:         envDurationSeconds("FLUSH_INTERVAL_SEC", 5*time.Second),
		BackfillInterval:      envDurationSeconds("BACKFILL_INTERVAL_SEC", 300*time.Second),
		AggregateInterval:     envDurationSeconds("AGGREGATE_INTERVAL_SEC", 30*time.Second),
		BatchSize:             envInt("BATCH_SIZE", 5000),
		InfluxDisabled:        envBool("INFLUX_DISABLED", false),
	}
}
