// Copyright (c) 2025 Darren Soothill (darren [at] soothill [dot] com)

package pipeline

import (
	"context"
	"errors"
	"log"
	"time"
)

type Backoff struct {
	base    time.Duration
	current time.Duration
	max     time.Duration
}

const tradeBufferSize = 1000

func NewBackoff(base, max time.Duration) *Backoff {
	return &Backoff{base: base, current: base, max: max}
}

func (b *Backoff) Reset() {
	b.current = b.base
}

func (b *Backoff) Next() time.Duration {
	delay := b.current
	b.current *= 2
	if b.current > b.max {
		b.current = b.max
	}
	return delay
}

func Run(ctx context.Context, config Config) error {
	storage, err := NewStorage(config.CacheDBPath)
	if err != nil {
		return err
	}
	defer storage.Close()

	if err := storage.Init(); err != nil {
		return err
	}

	rest := NewKrakenRestClient(config)
	backfillTrigger := make(chan struct{}, 1)

	var writer *InfluxWriter
	if !config.InfluxDisabled {
		writer, err = NewInfluxWriter(config)
		if err != nil {
			return err
		}
		log.Printf("influx writes enabled")
	} else {
		log.Printf("influx writes disabled; caching only")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	tradeCh := make(chan []Tick, tradeBufferSize)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case trades := <-tradeCh:
				inserted, err := storage.InsertTicks(trades)
				if err != nil {
					log.Printf("failed to store trades: %v", err)
					continue
				}
				if inserted > 0 {
					log.Printf("stored %d trades", inserted)
				}
			}
		}
	}()

	go StreamTrades(ctx, config, func(trades []Tick) {
		select {
		case tradeCh <- trades:
		case <-ctx.Done():
		default:
			log.Printf("dropping %d trades: channel full (%d/%d)", len(trades), len(tradeCh), cap(tradeCh))
		}
	}, func() {
		select {
		case backfillTrigger <- struct{}{}:
		default:
		}
	})

	go backfillLoop(ctx, storage, rest, backfillTrigger, config.BackfillInterval)
	go aggregateLoop(ctx, storage, config.AggregateInterval)
	if writer != nil {
		go flushLoop(ctx, storage, writer, config)
	}
	select {
	case backfillTrigger <- struct{}{}:
	default:
	}

	<-ctx.Done()
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil
	}
	return ctx.Err()
}

func backfillLoop(
	ctx context.Context,
	storage *Storage,
	rest *KrakenRestClient,
	trigger <-chan struct{},
	interval time.Duration,
) {
	backoff := NewBackoff(5*time.Second, 5*time.Minute)
	var nextAllowed time.Time
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-trigger:
		case <-ticker.C:
		}
		if !nextAllowed.IsZero() && time.Now().Before(nextAllowed) {
			continue
		}
		if err := backfillOnce(ctx, storage, rest); err != nil {
			log.Printf("backfill failed: %v", err)
			nextAllowed = time.Now().Add(backoff.Next())
			continue
		}
		backoff.Reset()
		nextAllowed = time.Time{}
	}
}

func backfillOnce(ctx context.Context, storage *Storage, rest *KrakenRestClient) error {
	since, err := storage.GetState("kraken_last")
	if err != nil {
		return err
	}
	current := since
	finalState := since
	const maxPages = 10
	totalInserted := 0
	for page := 0; page < maxPages; page++ {
		previous := current
		trades, last, err := rest.FetchTradesSince(ctx, current)
		if err != nil {
			return err
		}
		if len(trades) > 0 {
			inserted, err := storage.InsertTicks(trades)
			if err != nil {
				return err
			}
			totalInserted += inserted
		}
		if last != "" {
			finalState = last
			current = last
		}
		if last == "" || last == previous || len(trades) == 0 {
			break
		}
	}
	if finalState != "" {
		if err := storage.SetState("kraken_last", finalState); err != nil {
			return err
		}
	}
	if totalInserted > 0 {
		log.Printf("backfilled %d trades", totalInserted)
	}
	return nil
}

func aggregateLoop(ctx context.Context, storage *Storage, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count, err := aggregateMissingMinutes(storage)
			if err != nil {
				log.Printf("aggregation failed: %v", err)
				continue
			}
			if count > 0 {
				log.Printf("aggregated %d minute bars", count)
			}
		}
	}
}

func aggregateMissingMinutes(storage *Storage) (int, error) {
	currentMinute := (time.Now().UnixNano() / MinuteNS) * MinuteNS
	missing, err := storage.ListMissingMinutes(currentMinute, 500)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, minuteTS := range missing {
		ticks, err := storage.FetchTicksForMinute(minuteTS)
		if err != nil {
			return count, err
		}
		if len(ticks) == 0 {
			continue
		}
		openPrice := ticks[0].Price
		closePrice := ticks[len(ticks)-1].Price
		highPrice := openPrice
		lowPrice := openPrice
		volume := 0.0
		for _, tick := range ticks {
			if tick.Price > highPrice {
				highPrice = tick.Price
			}
			if tick.Price < lowPrice {
				lowPrice = tick.Price
			}
			volume += tick.Volume
		}
		bar := MinuteBar{
			MinuteTS:   minuteTS,
			Open:       openPrice,
			High:       highPrice,
			Low:        lowPrice,
			Close:      closePrice,
			Volume:     volume,
			TradeCount: len(ticks),
		}
		if err := storage.InsertMinuteBar(bar); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func flushLoop(ctx context.Context, storage *Storage, writer *InfluxWriter, config Config) {
	tickBackoff := NewBackoff(2*time.Second, 60*time.Second)
	minuteBackoff := NewBackoff(2*time.Second, 60*time.Second)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		wait := config.FlushInterval
		if err := flushTicks(ctx, storage, writer, config); err != nil {
			if delay := tickBackoff.Next(); delay > wait {
				wait = delay
			}
		} else {
			tickBackoff.Reset()
		}
		if err := flushMinutes(ctx, storage, writer, config); err != nil {
			if delay := minuteBackoff.Next(); delay > wait {
				wait = delay
			}
		} else {
			minuteBackoff.Reset()
		}

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func flushTicks(
	ctx context.Context,
	storage *Storage,
	writer *InfluxWriter,
	config Config,
) error {
	ticks, err := storage.FetchUnsentTicks(config.BatchSize)
	if err != nil {
		log.Printf("tick fetch failed: %v", err)
		return err
	}
	if len(ticks) == 0 {
		return nil
	}
	lines := make([]string, 0, len(ticks))
	tradeIDs := make([]string, 0, len(ticks))
	for _, tick := range ticks {
		lines = append(lines, tickToLine(config.InfluxMeasurementTick, config.KrakenPair, tick))
		tradeIDs = append(tradeIDs, tick.TradeID)
	}
	if err := writer.SendLines(ctx, lines); err != nil {
		log.Printf("tick flush failed: %v", err)
		return err
	}
	if err := storage.MarkTicksSent(tradeIDs); err != nil {
		log.Printf("tick mark failed: %v", err)
		return err
	}
	log.Printf("flushed %d ticks", len(ticks))
	return nil
}

func flushMinutes(
	ctx context.Context,
	storage *Storage,
	writer *InfluxWriter,
	config Config,
) error {
	minutes, err := storage.FetchUnsentMinutes(config.BatchSize)
	if err != nil {
		log.Printf("minute fetch failed: %v", err)
		return err
	}
	if len(minutes) == 0 {
		return nil
	}
	lines := make([]string, 0, len(minutes))
	minuteTS := make([]int64, 0, len(minutes))
	for _, bar := range minutes {
		lines = append(lines, minuteToLine(config.InfluxMeasurementMin, config.KrakenPair, bar))
		minuteTS = append(minuteTS, bar.MinuteTS)
	}
	if err := writer.SendLines(ctx, lines); err != nil {
		log.Printf("minute flush failed: %v", err)
		return err
	}
	if err := storage.MarkMinutesSent(minuteTS); err != nil {
		log.Printf("minute mark failed: %v", err)
		return err
	}
	log.Printf("flushed %d minute bars", len(minutes))
	return nil
}
