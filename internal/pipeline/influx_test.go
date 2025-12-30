// Copyright (c) 2025 Darren Soothill (darren [at] soothill [dot] com)

package pipeline

import (
	"context"
	"fmt"
	"testing"
	"time"
)

type lookupResponse struct {
	addrs []string
	err   error
}

type stubHostResolver struct {
	responses []lookupResponse
}

func (r *stubHostResolver) LookupHost(_ context.Context, _ string) ([]string, error) {
	if len(r.responses) == 0 {
		return nil, fmt.Errorf("no lookup responses configured")
	}
	res := r.responses[0]
	r.responses = r.responses[1:]
	return res.addrs, res.err
}

func TestResolveIPUsesCacheWhenLookupFails(t *testing.T) {
	writer := newTestInfluxWriter(t)
	writer.resolver = &stubHostResolver{
		responses: []lookupResponse{
			{addrs: []string{"1.2.3.4"}, err: nil},
			{addrs: nil, err: fmt.Errorf("dns down")},
		},
	}

	first, err := writer.resolveIP(context.Background(), "influxdb")
	if err != nil {
		t.Fatalf("first resolve failed: %v", err)
	}
	if first != "1.2.3.4" {
		t.Fatalf("unexpected first IP %q", first)
	}

	second, err := writer.resolveIP(context.Background(), "influxdb")
	if err != nil {
		t.Fatalf("expected cached IP on DNS failure, got error: %v", err)
	}
	if second != first {
		t.Fatalf("expected cached IP %q, got %q", first, second)
	}
}

func TestResolveIPReturnsErrorWithoutCache(t *testing.T) {
	writer := newTestInfluxWriter(t)
	writer.resolver = &stubHostResolver{
		responses: []lookupResponse{
			{addrs: nil, err: fmt.Errorf("dns unreachable")},
		},
	}

	if _, err := writer.resolveIP(context.Background(), "influxdb"); err == nil {
		t.Fatalf("expected error without cached IP when DNS fails")
	}
}

func newTestInfluxWriter(t *testing.T) *InfluxWriter {
	t.Helper()
	writer, err := NewInfluxWriter(Config{
		InfluxURL:     "http://influxdb:8086",
		InfluxOrg:     "org",
		InfluxBucket:  "bucket",
		InfluxToken:   "token",
		InfluxTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	return writer
}
