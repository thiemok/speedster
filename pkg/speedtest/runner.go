package speedtest

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var tracer = otel.Tracer("speedster")

// Config holds the speed test configuration
type Config struct {
	ServerID          string
	Timeout           time.Duration
	ConcurrentStreams int
	TestDuration      time.Duration
	SkipDownload      bool
	SkipUpload        bool
}

// Result holds the speed test results
type Result struct {
	Server       ServerInfo
	DownloadMbps float64
	UploadMbps   float64
	Duration     time.Duration
	Latency      time.Duration
	Jitter       time.Duration
}

// ServerInfo contains information about the test server
type ServerInfo struct {
	ID       string
	Name     string
	Country  string
	Distance float64
}

// Runner executes speed tests
type Runner struct {
	config Config
}

// LoadConfig loads configuration from environment variables
func LoadConfig() Config {
	return Config{
		ServerID:          getEnv("SPEEDTEST_SERVER_ID", ""),
		Timeout:           getEnvDuration("SPEEDTEST_TIMEOUT", 30*time.Second),
		ConcurrentStreams: getEnvInt("SPEEDTEST_CONCURRENT_STREAMS", 0),
		TestDuration:      getEnvDuration("SPEEDTEST_TEST_DURATION", 0),
		SkipDownload:      getEnvBool("SPEEDTEST_SKIP_DOWNLOAD", false),
		SkipUpload:        getEnvBool("SPEEDTEST_SKIP_UPLOAD", false),
	}
}

// NewRunner creates a new speed test runner
func NewRunner(config Config) *Runner {
	return &Runner{config: config}
}

// Run executes the speed test with tracing
func (r *Runner) Run(ctx context.Context) (*Result, error) {
	ctx, span := tracer.Start(ctx, "speedtest.execution")
	defer span.End()

	startTime := time.Now()

	// Select server
	server, err := r.selectServer(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "server selection failed")
		return nil, fmt.Errorf("server selection failed: %w", err)
	}

	span.SetAttributes(
		attribute.String("speedtest.server.id", server.ID),
		attribute.String("speedtest.server.name", server.Name),
		attribute.String("speedtest.server.country", server.Country),
		attribute.Float64("speedtest.server.distance", server.Distance),
	)

	result := &Result{
		Server: ServerInfo{
			ID:       server.ID,
			Name:     server.Name,
			Country:  server.Country,
			Distance: server.Distance,
		},
	}

	// Run download test
	if !r.config.SkipDownload {
		downloadMbps, err := r.runDownloadTest(ctx, &server)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "download test failed")
			return nil, fmt.Errorf("download test failed: %w", err)
		}
		result.DownloadMbps = downloadMbps
		span.SetAttributes(attribute.Float64("speedtest.download.mbps", downloadMbps))
	}

	// Run upload test
	if !r.config.SkipUpload {
		uploadMbps, err := r.runUploadTest(ctx, &server)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "upload test failed")
			return nil, fmt.Errorf("upload test failed: %w", err)
		}
		result.UploadMbps = uploadMbps
		span.SetAttributes(attribute.Float64("speedtest.upload.mbps", uploadMbps))
	}

	result.Duration = time.Since(startTime)
	result.Latency = server.Latency
	result.Jitter = server.Jitter

	span.SetStatus(codes.Ok, "speed test completed successfully")

	return result, nil
}

func (r *Runner) selectServer(ctx context.Context) (speedtest.Server, error) {
	ctx, span := tracer.Start(ctx, "speedtest.server_selection")
	defer span.End()

	user := speedtest.New()

	// Fetch server list
	serverList, err := user.FetchServers()
	if err != nil {
		return speedtest.Server{}, fmt.Errorf("failed to fetch servers: %w", err)
	}

	var preferredServers []int
	if id, err := strconv.Atoi(r.config.ServerID); err != nil {
		preferredServers = []int{id}
	}

	// Filter to requested number of servers
	targets, err := serverList.FindServer(preferredServers)
	if err != nil {
		return speedtest.Server{}, fmt.Errorf("failed to find servers: %w", err)
	}

	if len(targets) == 0 {
		return speedtest.Server{}, fmt.Errorf("no servers found")
	}

	return *targets[0], nil
}

func (r *Runner) runDownloadTest(ctx context.Context, server *speedtest.Server) (float64, error) {
	ctx, span := tracer.Start(ctx, "speedtest.download_test")
	defer span.End()

	if server == nil {
		return 0, fmt.Errorf("server missing")
	}

	span.SetAttributes(
		attribute.String("server.id", server.ID),
		attribute.String("server.name", server.Name),
	)

	err := server.DownloadTest()
	if err != nil {
		return 0, fmt.Errorf("download test failed: %w", err)
	}

	mbps := server.DLSpeed.Mbps()
	span.SetAttributes(attribute.Float64("download.mbps", mbps))

	latency := server.Latency.Nanoseconds()
	span.SetAttributes(attribute.Int64("download.latency_nanos", latency))

	jitter := server.Jitter.Nanoseconds()
	span.SetAttributes(attribute.Int64("download.jitter_nanos", jitter))

	return mbps, nil
}

func (r *Runner) runUploadTest(ctx context.Context, server *speedtest.Server) (float64, error) {
	ctx, span := tracer.Start(ctx, "speedtest.upload_test")
	defer span.End()

	if server == nil {
		return 0, fmt.Errorf("server missing")
	}

	span.SetAttributes(
		attribute.String("server.id", server.ID),
		attribute.String("server.name", server.Name),
	)

	err := server.UploadTest()
	if err != nil {
		return 0, fmt.Errorf("upload test failed: %w", err)
	}

	mbps := server.ULSpeed.Mbps()
	span.SetAttributes(attribute.Float64("upload.mbps", mbps))

	latency := server.Latency.Nanoseconds()
	span.SetAttributes(attribute.Int64("upload.latency_nanos", latency))

	jitter := server.Jitter.Nanoseconds()
	span.SetAttributes(attribute.Int64("upload.jitter_nanos", jitter))

	return mbps, nil
}

// Helper functions for environment variables
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
		// Try parsing as seconds
		if i, err := strconv.Atoi(value); err == nil {
			return time.Duration(i) * time.Second
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}
