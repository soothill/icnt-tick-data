// Copyright (c) 2025 Darren Soothill (darren [at] soothill [dot] com)

package pipeline

import (
	"testing"
	"time"
)

func TestLoadConfigDefaultsAndOverrides(t *testing.T) {
	t.Setenv("KRAKEN_PAIR", "ICNT/USD")
	t.Setenv("KRAKEN_REST_PAIR", "")
	t.Setenv("FLUSH_INTERVAL_SEC", "7")
	t.Setenv("BATCH_SIZE", "42")

	config := LoadConfig()
	if config.KrakenRestPair != "ICNTUSD" {
		t.Fatalf("expected rest pair ICNTUSD, got %q", config.KrakenRestPair)
	}
	if config.FlushInterval != 7*time.Second {
		t.Fatalf("expected flush interval 7s, got %v", config.FlushInterval)
	}
	if config.BatchSize != 42 {
		t.Fatalf("expected batch size 42, got %d", config.BatchSize)
	}
}
