// Copyright (c) 2025 Darren Soothill (darren [at] soothill [dot] com)

package main

import (
	"context"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/soothill/icnt-tick-data/internal/pipeline"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			pipeline.LogFailuref("pipeline panic: %v\n%s", r, debug.Stack())
			os.Exit(1)
		}
	}()
	config := pipeline.LoadConfig()
	pipeline.LogInfof("pipeline starting; pair=%s influx_disabled=%t", config.KrakenPair, config.InfluxDisabled)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := pipeline.Run(ctx, config); err != nil {
		pipeline.LogFailuref("pipeline stopped: %v", err)
		os.Exit(1)
	}
}
