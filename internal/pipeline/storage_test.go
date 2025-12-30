// Copyright (c) 2025 Darren Soothill (darren [at] soothill [dot] com)

package pipeline

import (
	"path/filepath"
	"testing"
)

func TestStorageLifecycle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ticks.sqlite")
	storage, err := NewStorage(path)
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
		{
			TradeID:   "t1",
			TSNS:      1700000000123456789,
			Price:     1.1,
			Volume:    2.2,
			Side:      "b",
			OrderType: "l",
			Misc:      "",
			Source:    "ws",
		},
		{
			TradeID:   "t2",
			TSNS:      1700000000450000000,
			Price:     1.2,
			Volume:    3.3,
			Side:      "s",
			OrderType: "l",
			Misc:      "",
			Source:    "ws",
		},
	}
	inserted, err := storage.InsertTicks(ticks)
	if err != nil {
		t.Fatalf("failed to insert ticks: %v", err)
	}
	if inserted != 2 {
		t.Fatalf("expected 2 inserts, got %d", inserted)
	}
	inserted, err = storage.InsertTicks([]Tick{ticks[0]})
	if err != nil {
		t.Fatalf("failed to insert duplicate: %v", err)
	}
	if inserted != 0 {
		t.Fatalf("expected 0 inserts for duplicate, got %d", inserted)
	}

	unsent, err := storage.FetchUnsentTicks(10)
	if err != nil {
		t.Fatalf("failed to fetch ticks: %v", err)
	}
	if len(unsent) != 2 {
		t.Fatalf("expected 2 unsent ticks, got %d", len(unsent))
	}

	if err := storage.MarkTicksSent([]string{"t1", "t2"}); err != nil {
		t.Fatalf("failed to mark sent: %v", err)
	}
	unsent, err = storage.FetchUnsentTicks(10)
	if err != nil {
		t.Fatalf("failed to fetch ticks after mark: %v", err)
	}
	if len(unsent) != 0 {
		t.Fatalf("expected 0 unsent ticks, got %d", len(unsent))
	}

	minuteStart := (ticks[0].TSNS / MinuteNS) * MinuteNS
	missing, err := storage.ListMissingMinutes(minuteStart+MinuteNS, 10)
	if err != nil {
		t.Fatalf("failed to list missing minutes: %v", err)
	}
	if len(missing) != 1 || missing[0] != minuteStart {
		t.Fatalf("expected missing minute %d, got %v", minuteStart, missing)
	}
	minuteTicks, err := storage.FetchTicksForMinute(minuteStart)
	if err != nil {
		t.Fatalf("failed to fetch ticks for minute: %v", err)
	}
	if len(minuteTicks) != 2 {
		t.Fatalf("expected 2 ticks in minute, got %d", len(minuteTicks))
	}

	bar := MinuteBar{
		MinuteTS:   minuteStart,
		Open:       1.1,
		High:       1.2,
		Low:        1.1,
		Close:      1.2,
		Volume:     5.5,
		TradeCount: 2,
	}
	if err := storage.InsertMinuteBar(bar); err != nil {
		t.Fatalf("failed to insert minute bar: %v", err)
	}
	bars, err := storage.FetchUnsentMinutes(10)
	if err != nil {
		t.Fatalf("failed to fetch minute bars: %v", err)
	}
	if len(bars) != 1 {
		t.Fatalf("expected 1 unsent minute bar, got %d", len(bars))
	}
	if err := storage.MarkMinutesSent([]int64{minuteStart}); err != nil {
		t.Fatalf("failed to mark minute sent: %v", err)
	}
	bars, err = storage.FetchUnsentMinutes(10)
	if err != nil {
		t.Fatalf("failed to fetch minute bars after mark: %v", err)
	}
	if len(bars) != 0 {
		t.Fatalf("expected 0 unsent minute bars, got %d", len(bars))
	}

	// New tick arrives for an already aggregated minute; bar should be re-aggregated and marked unsent.
	extraTick := Tick{
		TradeID:   "t3",
		TSNS:      minuteStart + 10,
		Price:     1.3,
		Volume:    1.1,
		Side:      "b",
		OrderType: "l",
		Misc:      "",
		Source:    "ws",
	}
	if _, err := storage.InsertTicks([]Tick{extraTick}); err != nil {
		t.Fatalf("failed to insert extra tick: %v", err)
	}
	missing, err = storage.ListMissingMinutes(minuteStart+MinuteNS, 10)
	if err != nil {
		t.Fatalf("failed to list missing minutes after new tick: %v", err)
	}
	if len(missing) != 1 || missing[0] != minuteStart {
		t.Fatalf("expected missing minute %d after new tick, got %v", minuteStart, missing)
	}
	updatedBar := MinuteBar{
		MinuteTS:   minuteStart,
		Open:       1.1,
		High:       1.3,
		Low:        1.1,
		Close:      1.3,
		Volume:     6.6,
		TradeCount: 3,
	}
	if err := storage.InsertMinuteBar(updatedBar); err != nil {
		t.Fatalf("failed to upsert minute bar: %v", err)
	}
	bars, err = storage.FetchUnsentMinutes(10)
	if err != nil {
		t.Fatalf("failed to fetch updated minute bars: %v", err)
	}
	if len(bars) != 1 || bars[0].TradeCount != 3 || bars[0].High != 1.3 {
		t.Fatalf("expected refreshed bar with 3 trades, got %+v", bars)
	}

	if err := storage.SetState("kraken_last", "123"); err != nil {
		t.Fatalf("failed to set state: %v", err)
	}
	state, err := storage.GetState("kraken_last")
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}
	if state != "123" {
		t.Fatalf("expected state 123, got %q", state)
	}
}
