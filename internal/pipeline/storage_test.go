// Copyright (c) 2025 Darren Soothill (darren [at] soothill [dot] com)

package pipeline

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestStorage(t *testing.T) *Storage {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ticks.sqlite")
	store, err := NewStorage(path)
	if err != nil {
		t.Fatalf("NewStorage error: %v", err)
	}
	if err := store.Init(); err != nil {
		t.Fatalf("Init error: %v", err)
	}
	return store
}

func TestStorageInsertFetchAndMarkTicks(t *testing.T) {
	store := newTestStorage(t)
	ticks := []Tick{
		{TradeID: "a", TSNS: 10, Price: 1, Volume: 1, Side: "b", OrderType: "l", Misc: "", Source: "ws"},
		{TradeID: "b", TSNS: 20, Price: 2, Volume: 2, Side: "s", OrderType: "l", Misc: "", Source: "ws"},
		{TradeID: "c", TSNS: 15, Price: 3, Volume: 3, Side: "b", OrderType: "l", Misc: "", Source: "ws"},
	}
	if inserted, err := store.InsertTicks(ticks); err != nil || inserted != len(ticks) {
		t.Fatalf("InsertTicks inserted=%d err=%v", inserted, err)
	}
	if inserted, err := store.InsertTicks(ticks); err != nil || inserted != 0 {
		t.Fatalf("duplicate InsertTicks inserted=%d err=%v", inserted, err)
	}
	fetched, err := store.FetchUnsentTicks(10)
	if err != nil {
		t.Fatalf("FetchUnsentTicks error: %v", err)
	}
	if len(fetched) != len(ticks) {
		t.Fatalf("expected %d unsent ticks, got %d", len(ticks), len(fetched))
	}
	if fetched[0].TradeID != "a" || fetched[1].TradeID != "c" || fetched[2].TradeID != "b" {
		t.Fatalf("ticks not ordered by ts_ns: %+v", fetched)
	}
	if err := store.MarkTicksSent([]string{"a", "b", "c"}); err != nil {
		t.Fatalf("MarkTicksSent error: %v", err)
	}
	fetched, err = store.FetchUnsentTicks(10)
	if err != nil {
		t.Fatalf("FetchUnsentTicks after mark error: %v", err)
	}
	if len(fetched) != 0 {
		t.Fatalf("expected 0 unsent ticks after mark, got %d", len(fetched))
	}
}

func TestStorageMinutesAndMark(t *testing.T) {
	store := newTestStorage(t)
	bar := MinuteBar{MinuteTS: 60, Open: 1, High: 2, Low: 1, Close: 2, Volume: 5, TradeCount: 3}
	if err := store.InsertMinuteBar(bar); err != nil {
		t.Fatalf("InsertMinuteBar error: %v", err)
	}
	bar.Volume = 6
	if err := store.InsertMinuteBar(bar); err != nil {
		t.Fatalf("InsertMinuteBar update error: %v", err)
	}
	bars, err := store.FetchUnsentMinutes(10)
	if err != nil {
		t.Fatalf("FetchUnsentMinutes error: %v", err)
	}
	if len(bars) != 1 || bars[0].Volume != 6 {
		t.Fatalf("unexpected bars: %+v", bars)
	}
	if err := store.MarkMinutesSent([]int64{bar.MinuteTS}); err != nil {
		t.Fatalf("MarkMinutesSent error: %v", err)
	}
	bars, err = store.FetchUnsentMinutes(10)
	if err != nil {
		t.Fatalf("FetchUnsentMinutes after mark error: %v", err)
	}
	if len(bars) != 0 {
		t.Fatalf("expected 0 unsent minutes after mark, got %d", len(bars))
	}
}

func TestListMissingMinutesAndAggregate(t *testing.T) {
	store := newTestStorage(t)
	base := time.Now().Add(-3 * time.Minute)
	minute0 := (base.UnixNano() / MinuteNS) * MinuteNS
	minute1 := minute0 + MinuteNS

	ticks := []Tick{
		{TradeID: "m0a", TSNS: minute0 + 1_000, Price: 1, Volume: 1, Side: "b", OrderType: "l", Misc: "", Source: "ws"},
		{TradeID: "m0b", TSNS: minute0 + 2_000, Price: 3, Volume: 2, Side: "b", OrderType: "l", Misc: "", Source: "ws"},
		{TradeID: "m1a", TSNS: minute1 + 1_000, Price: 5, Volume: 4, Side: "s", OrderType: "l", Misc: "", Source: "ws"},
	}
	if inserted, err := store.InsertTicks(ticks); err != nil || inserted != len(ticks) {
		t.Fatalf("InsertTicks inserted=%d err=%v", inserted, err)
	}
	missing, err := store.ListMissingMinutes(minute1+MinuteNS, 10)
	if err != nil {
		t.Fatalf("ListMissingMinutes error: %v", err)
	}
	if len(missing) != 2 || missing[0] != minute0 || missing[1] != minute1 {
		t.Fatalf("unexpected missing minutes: %v", missing)
	}

	count, err := aggregateMissingMinutes(store)
	if err != nil {
		t.Fatalf("aggregateMissingMinutes error: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 aggregated minutes, got %d", count)
	}
	bars, err := store.FetchUnsentMinutes(10)
	if err != nil {
		t.Fatalf("FetchUnsentMinutes error: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("expected 2 aggregated bars, got %d", len(bars))
	}
	if bars[0].MinuteTS != minute0 || bars[0].Open != 1 || bars[0].Close != 3 || bars[0].High != 3 || bars[0].Low != 1 || bars[0].Volume != 3 || bars[0].TradeCount != 2 {
		t.Fatalf("unexpected first bar: %+v", bars[0])
	}
	if bars[1].MinuteTS != minute1 || bars[1].Open != 5 || bars[1].Close != 5 || bars[1].High != 5 || bars[1].Low != 5 || bars[1].Volume != 4 || bars[1].TradeCount != 1 {
		t.Fatalf("unexpected second bar: %+v", bars[1])
	}
}

func TestStorageStateAndStats(t *testing.T) {
	store := newTestStorage(t)
	if err := store.SetState("foo", "bar"); err != nil {
		t.Fatalf("SetState error: %v", err)
	}
	value, err := store.GetState("foo")
	if err != nil || value != "bar" {
		t.Fatalf("GetState value=%s err=%v", value, err)
	}
	_, _ = store.InsertTicks([]Tick{{TradeID: "x", TSNS: 1, Price: 1, Volume: 1, Side: "b", OrderType: "l", Misc: "", Source: "ws"}})
	_ = store.InsertMinuteBar(MinuteBar{MinuteTS: 1, Open: 1, High: 1, Low: 1, Close: 1, Volume: 1, TradeCount: 1})

	stats, err := store.DebugStats()
	if err != nil {
		t.Fatalf("DebugStats error: %v", err)
	}
	if stats != "unsent_ticks=1 unsent_minutes=1" {
		t.Fatalf("unexpected debug stats: %s", stats)
	}
}
