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
	results, err := runner.Run(ctx)
	if err != nil {
		log.Fatalf("Speed test failed: %v", err)
	}

	// Log individual results
	log.Printf("Speed test completed successfully with %d measurement(s):", len(results))
	for _, result := range results {
		log.Printf("Measurement %d:", result.MeasurementIndex)
		log.Printf("  Server: %s (%s) - ID: %d", result.Server.Name, result.Server.Country, result.Server.ID)
		log.Printf("  Download: %.2f Mbps", result.DownloadMbps)
		log.Printf("  Upload: %.2f Mbps", result.UploadMbps)
		log.Printf("  Latency: %d ms", result.Latency.Milliseconds())
		log.Printf("  Jitter: %d ms", result.Jitter.Milliseconds())
		log.Printf("  Duration: %v", result.Duration)
	}

	// Calculate and log statistics if multiple measurements
	if len(results) > 1 {
		var totalDownload, totalUpload float64
		minDownload, maxDownload := results[0].DownloadMbps, results[0].DownloadMbps
		minUpload, maxUpload := results[0].UploadMbps, results[0].UploadMbps

		for _, result := range results {
			totalDownload += result.DownloadMbps
			totalUpload += result.UploadMbps

			if result.DownloadMbps < minDownload {
				minDownload = result.DownloadMbps
			}
			if result.DownloadMbps > maxDownload {
				maxDownload = result.DownloadMbps
			}
			if result.UploadMbps < minUpload {
				minUpload = result.UploadMbps
			}
			if result.UploadMbps > maxUpload {
				maxUpload = result.UploadMbps
			}
		}

		avgDownload := totalDownload / float64(len(results))
		avgUpload := totalUpload / float64(len(results))

		log.Printf("Statistics across %d measurements:", len(results))
		log.Printf("  Download - Avg: %.2f Mbps, Min: %.2f Mbps, Max: %.2f Mbps", avgDownload, minDownload, maxDownload)
		log.Printf("  Upload   - Avg: %.2f Mbps, Min: %.2f Mbps, Max: %.2f Mbps", avgUpload, minUpload, maxUpload)
	}

	// Record metrics for each result
	for _, result := range results {
		if err := metrics.RecordSpeedTestMetrics(ctx, result); err != nil {
			log.Printf("Warning: Failed to record metrics for measurement %d: %v", result.MeasurementIndex, err)
		}
	}

	log.Println("Speed test completed, exiting...")
}
