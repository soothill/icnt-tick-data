// Copyright (c) 2025 Darren Soothill (darren [at] soothill [dot] com)

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/soothill/icnt-tick-data/internal/pipeline"
)

func main() {
	config := pipeline.LoadConfig()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := pipeline.Run(ctx, config); err != nil {
		log.Fatalf("pipeline stopped: %v", err)
	}
}
