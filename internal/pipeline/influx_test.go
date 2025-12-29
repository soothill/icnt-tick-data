// Copyright (c) 2025 Darren Soothill (darren [at] soothill [dot] com)

package pipeline

import "testing"

func TestTickToLine(t *testing.T) {
	tick := Tick{
		TradeID: "trade\"1",
		TSNS:    1700000000000000000,
		Price:   1.23,
		Volume:  4.56,
		Side:    "b",
	}
	line := tickToLine("ticks", "ICNT/USD", tick)
	expected := "ticks,pair=ICNT/USD,side=b price=1.23,volume=4.56,trade_id=\"trade\\\"1\" 1700000000000000000"
	if line != expected {
		t.Fatalf("expected %q, got %q", expected, line)
	}
}

func TestMinuteToLine(t *testing.T) {
	bar := MinuteBar{
		MinuteTS:   1700000000000000000,
		Open:       1.0,
		High:       2.0,
		Low:        0.5,
		Close:      1.5,
		Volume:     10.0,
		TradeCount: 3,
	}
	line := minuteToLine("minutes", "ICNT/USD", bar)
	expected := "minutes,pair=ICNT/USD open=1,high=2,low=0.5,close=1.5,volume=10,trade_count=3i 1700000000000000000"
	if line != expected {
		t.Fatalf("expected %q, got %q", expected, line)
	}
}
