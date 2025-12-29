// Copyright (c) 2025 Darren Soothill (darren [at] soothill [dot] com)

package pipeline

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

const MinuteNS = int64(60_000_000_000)

type Tick struct {
	TradeID   string
	TSNS      int64
	Price     float64
	Volume    float64
	Side      string
	OrderType string
	Misc      string
	Source    string
}

type MinuteBar struct {
	MinuteTS   int64
	Open       float64
	High       float64
	Low        float64
	Close      float64
	Volume     float64
	TradeCount int
}

type Storage struct {
	db *sql.DB
	mu sync.Mutex
}

func NewStorage(dbPath string) (*Storage, error) {
	if dir := filepath.Dir(dbPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	return &Storage{db: conn}, nil
}

func (s *Storage) Init() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return err
	}
	if _, err := s.db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		return err
	}
	if _, err := s.db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS ticks (
			trade_id TEXT PRIMARY KEY,
			ts_ns INTEGER NOT NULL,
			price REAL NOT NULL,
			volume REAL NOT NULL,
			side TEXT NOT NULL,
			order_type TEXT NOT NULL,
			misc TEXT NOT NULL,
			source TEXT NOT NULL,
			sent INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_ticks_sent_ts ON ticks(sent, ts_ns);

		CREATE TABLE IF NOT EXISTS minute_bars (
			minute_ts INTEGER PRIMARY KEY,
			open REAL NOT NULL,
			high REAL NOT NULL,
			low REAL NOT NULL,
			close REAL NOT NULL,
			volume REAL NOT NULL,
			trade_count INTEGER NOT NULL,
			sent INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_minutes_sent_ts ON minute_bars(sent, minute_ts);

		CREATE TABLE IF NOT EXISTS state (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`)
	return err
}

func (s *Storage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}

func (s *Storage) InsertTicks(ticks []Tick) (int, error) {
	if len(ticks) == 0 {
		return 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO ticks (
			trade_id, ts_ns, price, volume, side, order_type, misc, source
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	defer stmt.Close()
	inserted := 0
	for _, tick := range ticks {
		result, execErr := stmt.Exec(
			tick.TradeID,
			tick.TSNS,
			tick.Price,
			tick.Volume,
			tick.Side,
			tick.OrderType,
			tick.Misc,
			tick.Source,
		)
		if execErr != nil {
			_ = tx.Rollback()
			return inserted, execErr
		}
		if rows, _ := result.RowsAffected(); rows > 0 {
			inserted += int(rows)
		}
	}
	if err := tx.Commit(); err != nil {
		return inserted, err
	}
	return inserted, nil
}

func (s *Storage) FetchUnsentTicks(limit int) ([]Tick, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`
		SELECT trade_id, ts_ns, price, volume, side, order_type, misc, source
		FROM ticks
		WHERE sent = 0
		ORDER BY ts_ns ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ticks []Tick
	for rows.Next() {
		var tick Tick
		if err := rows.Scan(
			&tick.TradeID,
			&tick.TSNS,
			&tick.Price,
			&tick.Volume,
			&tick.Side,
			&tick.OrderType,
			&tick.Misc,
			&tick.Source,
		); err != nil {
			return nil, err
		}
		ticks = append(ticks, tick)
	}
	return ticks, rows.Err()
}

func (s *Storage) MarkTicksSent(tradeIDs []string) error {
	if len(tradeIDs) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stmt, err := s.db.Prepare("UPDATE ticks SET sent = 1 WHERE trade_id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, tradeID := range tradeIDs {
		if _, err := stmt.Exec(tradeID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Storage) ListMissingMinutes(currentMinuteStart int64, limit int) ([]int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`
		SELECT m.minute_ts
		FROM (
			SELECT DISTINCT CAST(ts_ns / ? AS INTEGER) * ? AS minute_ts
			FROM ticks
			WHERE ts_ns < ?
		) m
		LEFT JOIN minute_bars b ON b.minute_ts = m.minute_ts
		WHERE b.minute_ts IS NULL
		ORDER BY m.minute_ts ASC
		LIMIT ?
	`, MinuteNS, MinuteNS, currentMinuteStart, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var minutes []int64
	for rows.Next() {
		var minuteTS int64
		if err := rows.Scan(&minuteTS); err != nil {
			return nil, err
		}
		minutes = append(minutes, minuteTS)
	}
	return minutes, rows.Err()
}

func (s *Storage) FetchTicksForMinute(minuteTS int64) ([]Tick, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`
		SELECT price, volume, ts_ns
		FROM ticks
		WHERE ts_ns >= ? AND ts_ns < ?
		ORDER BY ts_ns ASC
	`, minuteTS, minuteTS+MinuteNS)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ticks []Tick
	for rows.Next() {
		var tick Tick
		if err := rows.Scan(&tick.Price, &tick.Volume, &tick.TSNS); err != nil {
			return nil, err
		}
		ticks = append(ticks, tick)
	}
	return ticks, rows.Err()
}

func (s *Storage) InsertMinuteBar(bar MinuteBar) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO minute_bars (
			minute_ts, open, high, low, close, volume, trade_count
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, bar.MinuteTS, bar.Open, bar.High, bar.Low, bar.Close, bar.Volume, bar.TradeCount)
	return err
}

func (s *Storage) FetchUnsentMinutes(limit int) ([]MinuteBar, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`
		SELECT minute_ts, open, high, low, close, volume, trade_count
		FROM minute_bars
		WHERE sent = 0
		ORDER BY minute_ts ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bars []MinuteBar
	for rows.Next() {
		var bar MinuteBar
		if err := rows.Scan(
			&bar.MinuteTS,
			&bar.Open,
			&bar.High,
			&bar.Low,
			&bar.Close,
			&bar.Volume,
			&bar.TradeCount,
		); err != nil {
			return nil, err
		}
		bars = append(bars, bar)
	}
	return bars, rows.Err()
}

func (s *Storage) MarkMinutesSent(minuteTS []int64) error {
	if len(minuteTS) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stmt, err := s.db.Prepare("UPDATE minute_bars SET sent = 1 WHERE minute_ts = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, minute := range minuteTS {
		if _, err := stmt.Exec(minute); err != nil {
			return err
		}
	}
	return nil
}

func (s *Storage) GetState(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.db.QueryRow("SELECT value FROM state WHERE key = ?", key)
	var value string
	switch err := row.Scan(&value); err {
	case nil:
		return value, nil
	case sql.ErrNoRows:
		return "", nil
	default:
		return "", err
	}
}

func (s *Storage) SetState(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		"INSERT INTO state (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key,
		value,
	)
	return err
}

func (s *Storage) CountUnsentTicks() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.db.QueryRow("SELECT COUNT(*) FROM ticks WHERE sent = 0")
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Storage) CountUnsentMinutes() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.db.QueryRow("SELECT COUNT(*) FROM minute_bars WHERE sent = 0")
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Storage) DebugStats() (string, error) {
	ticks, err := s.CountUnsentTicks()
	if err != nil {
		return "", err
	}
	minutes, err := s.CountUnsentMinutes()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("unsent_ticks=%d unsent_minutes=%d", ticks, minutes), nil
}
