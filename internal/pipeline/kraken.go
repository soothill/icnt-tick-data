// Copyright (c) 2025 Darren Soothill (darren [at] soothill [dot] com)

package pipeline

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type KrakenRestClient struct {
	config Config
	client *http.Client
}

func NewKrakenRestClient(config Config) *KrakenRestClient {
	return &KrakenRestClient{
		config: config,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *KrakenRestClient) FetchTradesSince(ctx context.Context, since string) ([]Tick, string, error) {
	params := url.Values{}
	params.Set("pair", c.config.KrakenRestPair)
	if since != "" {
		params.Set("since", since)
	}
	endpoint := fmt.Sprintf("%s?%s", c.config.KrakenRESTURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, "", fmt.Errorf("kraken rest status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	var parsed struct {
		Error  []string                   `json:"error"`
		Result map[string]json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return nil, "", err
	}
	if len(parsed.Error) > 0 {
		return nil, "", fmt.Errorf("kraken rest error: %s", strings.Join(parsed.Error, ", "))
	}
	var last string
	if raw, ok := parsed.Result["last"]; ok {
		_ = json.Unmarshal(raw, &last)
	}
	var tradesRaw json.RawMessage
	for key, raw := range parsed.Result {
		if key != "last" {
			tradesRaw = raw
			break
		}
	}
	if len(tradesRaw) == 0 {
		return nil, last, nil
	}
	var rows [][]interface{}
	if err := json.Unmarshal(tradesRaw, &rows); err != nil {
		return nil, last, err
	}
	trades := make([]Tick, 0, len(rows))
	for _, row := range rows {
		trade, err := parseTradeRow(row, "rest")
		if err != nil {
			continue
		}
		trades = append(trades, trade)
	}
	return trades, last, nil
}

func StreamTrades(ctx context.Context, config Config, onTrades func([]Tick), onDisconnect func()) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		err := streamOnce(ctx, config, onTrades)
		if err != nil && onDisconnect != nil {
			onDisconnect()
		}
		if err != nil {
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			backoff *= 2
			if backoff > 60*time.Second {
				backoff = 60 * time.Second
			}
			continue
		}
		backoff = time.Second
	}
}

func streamOnce(ctx context.Context, config Config, onTrades func([]Tick)) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.DialContext(ctx, config.KrakenWSURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	pongWait := 60 * time.Second
	pingPeriod := 20 * time.Second
	writeWait := 10 * time.Second
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				deadline := time.Now().Add(writeWait)
				if err := conn.WriteControl(websocket.PingMessage, []byte("ping"), deadline); err != nil {
					_ = conn.Close()
					return
				}
			}
		}
	}()

	subscribe := map[string]interface{}{
		"event": "subscribe",
		"pair":  []string{config.KrakenPair},
		"subscription": map[string]string{
			"name": "trade",
		},
	}
	if err := conn.WriteJSON(subscribe); err != nil {
		return err
	}

	for {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		_, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var payload interface{}
		if err := json.Unmarshal(message, &payload); err != nil {
			continue
		}
		arr, ok := payload.([]interface{})
		if !ok || len(arr) < 4 {
			continue
		}
		channel, ok := arr[2].(string)
		if !ok || channel != "trade" {
			continue
		}
		rows, ok := arr[1].([]interface{})
		if !ok {
			continue
		}
		trades := make([]Tick, 0, len(rows))
		for _, row := range rows {
			rowValues, ok := row.([]interface{})
			if !ok {
				continue
			}
			trade, err := parseTradeRow(rowValues, "ws")
			if err != nil {
				continue
			}
			trades = append(trades, trade)
		}
		if len(trades) > 0 {
			onTrades(trades)
		}
	}
}

func parseTradeRow(row []interface{}, source string) (Tick, error) {
	if len(row) < 6 {
		return Tick{}, fmt.Errorf("invalid trade row")
	}
	price, err := parseFloat(row[0])
	if err != nil {
		return Tick{}, err
	}
	volume, err := parseFloat(row[1])
	if err != nil {
		return Tick{}, err
	}
	tsNS, err := parseTimestampNS(row[2])
	if err != nil {
		return Tick{}, err
	}
	side, err := parseString(row[3])
	if err != nil {
		return Tick{}, err
	}
	orderType, err := parseString(row[4])
	if err != nil {
		return Tick{}, err
	}
	misc, err := parseString(row[5])
	if err != nil {
		return Tick{}, err
	}
	tradeID := ""
	if len(row) >= 7 {
		tradeID, _ = parseString(row[6])
	}
	if tradeID == "" {
		tradeID = hashTradeID(tsNS, price, volume, side, orderType, misc)
	}
	return Tick{
		TradeID:   tradeID,
		TSNS:      tsNS,
		Price:     price,
		Volume:    volume,
		Side:      side,
		OrderType: orderType,
		Misc:      misc,
		Source:    source,
	}, nil
}

func parseString(value interface{}) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	default:
		return "", fmt.Errorf("invalid string type")
	}
}

func parseFloat(value interface{}) (float64, error) {
	switch v := value.(type) {
	case string:
		return strconv.ParseFloat(v, 64)
	case float64:
		return v, nil
	case json.Number:
		return v.Float64()
	default:
		return 0, fmt.Errorf("invalid float type")
	}
}

func parseTimestampNS(value interface{}) (int64, error) {
	switch v := value.(type) {
	case string:
		parts := strings.SplitN(v, ".", 2)
		secs, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return 0, err
		}
		nanos := int64(0)
		if len(parts) == 2 {
			frac := parts[1]
			if len(frac) > 9 {
				frac = frac[:9]
			}
			for len(frac) < 9 {
				frac += "0"
			}
			parsed, err := strconv.ParseInt(frac, 10, 64)
			if err != nil {
				return 0, err
			}
			nanos = parsed
		}
		return secs*1_000_000_000 + nanos, nil
	case float64:
		return int64(v * 1_000_000_000), nil
	case json.Number:
		parsed, err := v.Float64()
		if err != nil {
			return 0, err
		}
		return int64(parsed * 1_000_000_000), nil
	default:
		return 0, fmt.Errorf("invalid timestamp type")
	}
}

func hashTradeID(tsNS int64, price, volume float64, side, orderType, misc string) string {
	payload := fmt.Sprintf("%d-%f-%f-%s-%s-%s", tsNS, price, volume, side, orderType, misc)
	digest := sha1.Sum([]byte(payload))
	return "rest-" + hex.EncodeToString(digest[:10])
}
