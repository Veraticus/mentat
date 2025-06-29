// Package main provides the entry point for the Mentat personal assistant bot.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	os.Exit(runMain())
}

func runMain() int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down gracefully...")
		cancel()
	}()

	if err := run(ctx); err != nil {
		log.Printf("Error: %v", err)
		return 1
	}
	return 0
}

func run(ctx context.Context) error {
	log.Println("Mentat starting...")
	
	<-ctx.Done()
	
	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	
	return shutdown(shutdownCtx)
}

func shutdown(_ context.Context) error {
	log.Println("Shutdown complete")
	return nil
}