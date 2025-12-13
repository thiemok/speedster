package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thiemok/speedster/pkg/metrics"
	"github.com/thiemok/speedster/pkg/speedtest"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Received shutdown signal, cleaning up...")
		cancel()
	}()

	// Initialize OTEL
	log.Println("Initializing OpenTelemetry...")
	shutdown, err := metrics.InitOTEL(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize OTEL: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdown(ctx); err != nil {
			log.Printf("Error during OTEL shutdown: %v", err)
		}
	}()

	// Load configuration
	config := speedtest.LoadConfig()
	log.Printf("Starting speed test with config: %+v", config)

	// Run speed test with tracing
	runner := speedtest.NewRunner(config)
	result, err := runner.Run(ctx)
	if err != nil {
		log.Fatalf("Speed test failed: %v", err)
	}

	// Log results
	log.Printf("Speed test completed successfully:")
	log.Printf("  Server: %s (%s)", result.Server.Name, result.Server.Country)
	log.Printf("  Download: %.2f Mbps", result.DownloadMbps)
	log.Printf("  Upload: %.2f Mbps", result.UploadMbps)
	log.Printf("  Latency: %d ms", result.Latency.Milliseconds())
	log.Printf("  Jitter: %.d ms", result.Jitter.Milliseconds())

	// Record metrics
	if err := metrics.RecordSpeedTestMetrics(ctx, result); err != nil {
		log.Printf("Warning: Failed to record metrics: %v", err)
	}

	log.Println("Speed test completed, exiting...")
}
