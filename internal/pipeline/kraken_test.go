// Copyright (c) 2025 Darren Soothill (darren [at] soothill [dot] com)

package pipeline

import (
	"strings"
	"testing"
)

func TestParseTimestampNS(t *testing.T) {
	value := "1700000000.123456789"
	parsed, err := parseTimestampNS(value)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed != 1700000000123456789 {
		t.Fatalf("expected 1700000000123456789, got %d", parsed)
	}

	short := "1700000000.12"
	parsed, err = parseTimestampNS(short)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed != 1700000000120000000 {
		t.Fatalf("expected 1700000000120000000, got %d", parsed)
	}
}

func TestParseTradeRow(t *testing.T) {
	row := []interface{}{ "1.23", "4.56", "1700000000.120000000", "b", "l", "" }
	trade, err := parseTradeRow(row, "rest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if trade.TSNS != 1700000000120000000 {
		t.Fatalf("expected ts ns 1700000000120000000, got %d", trade.TSNS)
	}
	if trade.TradeID == "" || !strings.HasPrefix(trade.TradeID, "rest-") {
		t.Fatalf("expected generated trade ID, got %q", trade.TradeID)
	}

	rowWithID := []interface{}{ "1.23", "4.56", "1700000000.120000000", "b", "l", "", "abc123" }
	trade, err = parseTradeRow(rowWithID, "rest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if trade.TradeID != "abc123" {
		t.Fatalf("expected trade ID abc123, got %q", trade.TradeID)
	}
}
