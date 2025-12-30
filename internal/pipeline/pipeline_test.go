// Copyright (c) 2025 Darren Soothill (darren [at] soothill [dot] com)

package pipeline

import (
	"context"
	"fmt"
	"testing"
	"time"
)

type stubWriter struct {
	lines [][]string
	fail  bool
}

func (w *stubWriter) SendLines(_ context.Context, lines []string) error {
	if w.fail {
		return fmt.Errorf("boom")
	}
	copyLines := append([]string(nil), lines...)
	w.lines = append(w.lines, copyLines)
	return nil
}

func TestAggregateMissingMinutesRefreshesExistingBars(t *testing.T) {
	storage, err := NewStorage(t.TempDir() + "/ticks.sqlite")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.Close()
	})
	if err := storage.Init(); err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	minuteStart := (time.Now().Add(-2*time.Minute).UnixNano() / MinuteNS) * MinuteNS
	initial := []Tick{
		{TradeID: "t1", TSNS: minuteStart, Price: 1.0, Volume: 1.0, Side: "b", OrderType: "l"},
		{TradeID: "t2", TSNS: minuteStart + 1_000, Price: 1.1, Volume: 1.0, Side: "b", OrderType: "l"},
	}
	if _, err := storage.InsertTicks(initial); err != nil {
		t.Fatalf("failed to insert initial ticks: %v", err)
	}
	initialBar := MinuteBar{
		MinuteTS:   minuteStart,
		Open:       1.0,
		High:       1.1,
		Low:        1.0,
		Close:      1.1,
		Volume:     2.0,
		TradeCount: 2,
	}
	if err := storage.InsertMinuteBar(initialBar); err != nil {
		t.Fatalf("failed to insert initial bar: %v", err)
	}
	if err := storage.MarkMinutesSent([]int64{minuteStart}); err != nil {
		t.Fatalf("failed to mark bar sent: %v", err)
	}

	// New tick arrives for same minute; aggregate should refresh the bar and mark it unsent.
	extra := Tick{TradeID: "t3", TSNS: minuteStart + 2_000, Price: 1.3, Volume: 0.5, Side: "s", OrderType: "l"}
	if _, err := storage.InsertTicks([]Tick{extra}); err != nil {
		t.Fatalf("failed to insert extra tick: %v", err)
	}

	count, err := aggregateMissingMinutes(storage)
	if err != nil {
		t.Fatalf("aggregation failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 bar refreshed, got %d", count)
	}

	bars, err := storage.FetchUnsentMinutes(10)
	if err != nil {
		t.Fatalf("failed to fetch unsent bars: %v", err)
	}
	if len(bars) != 1 {
		t.Fatalf("expected 1 unsent bar after refresh, got %d", len(bars))
	}
	if bars[0].TradeCount != 3 || bars[0].High != 1.3 || bars[0].Close != 1.3 {
		t.Fatalf("unexpected refreshed bar: %+v", bars[0])
	}
}

func TestFlushTicksAndMinutes(t *testing.T) {
	storage, err := NewStorage(t.TempDir() + "/ticks.sqlite")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.Close()
	})
	if err := storage.Init(); err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	ticks := []Tick{
		{TradeID: "t1", TSNS: 1, Price: 1.0, Volume: 1.0, Side: "b", OrderType: "l"},
		{TradeID: "t2", TSNS: 2, Price: 1.1, Volume: 2.0, Side: "s", OrderType: "l"},
	}
	if _, err := storage.InsertTicks(ticks); err != nil {
		t.Fatalf("failed to insert ticks: %v", err)
	}
	bar := MinuteBar{MinuteTS: 0, Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 3, TradeCount: 2}
	if err := storage.InsertMinuteBar(bar); err != nil {
		t.Fatalf("failed to insert bar: %v", err)
	}

	writer := &stubWriter{}
	cfg := Config{
		KrakenPair:            "ICNT/USD",
		InfluxMeasurementTick: "ticks",
		InfluxMeasurementMin:  "minutes",
		BatchSize:             10,
	}

	if err := flushTicks(context.Background(), storage, writer, cfg); err != nil {
		t.Fatalf("flush ticks failed: %v", err)
	}
	if len(writer.lines) != 1 || len(writer.lines[0]) != 2 {
		t.Fatalf("expected 2 tick lines, got %+v", writer.lines)
	}
	unsentTicks, err := storage.FetchUnsentTicks(10)
	if err != nil {
		t.Fatalf("failed to fetch ticks after flush: %v", err)
	}
	if len(unsentTicks) != 0 {
		t.Fatalf("expected all ticks marked sent, got %d", len(unsentTicks))
	}

	if err := flushMinutes(context.Background(), storage, writer, cfg); err != nil {
		t.Fatalf("flush minutes failed: %v", err)
	}
	if len(writer.lines) != 2 || len(writer.lines[1]) != 1 {
		t.Fatalf("expected 1 minute line, got %+v", writer.lines)
	}
	unsentBars, err := storage.FetchUnsentMinutes(10)
	if err != nil {
		t.Fatalf("failed to fetch bars after flush: %v", err)
	}
	if len(unsentBars) != 0 {
		t.Fatalf("expected all bars marked sent, got %d", len(unsentBars))
	}
}
