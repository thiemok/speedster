package speedtest

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var tracer = otel.Tracer("speedster")

// MeasurementStrategy defines how measurements are distributed across servers
type MeasurementStrategy string

const (
	// MeasurementStrategySingleServer runs all measurements on the same server
	MeasurementStrategySingleServer MeasurementStrategy = "single-server"

	// MeasurementStrategyMultiServer runs each measurement on a different server
	MeasurementStrategyMultiServer MeasurementStrategy = "multi-server"
)

// Valid checks if the strategy is valid
func (s MeasurementStrategy) Valid() bool {
	switch s {
	case MeasurementStrategySingleServer, MeasurementStrategyMultiServer:
		return true
	default:
		return false
	}
}

// Config holds the speed test configuration
type Config struct {
	ServerIDs           []string
	Timeout             time.Duration
	ConcurrentStreams   int
	TestDuration        time.Duration
	SkipDownload        bool
	SkipUpload          bool
	MeasurementCount    int
	MeasurementStrategy MeasurementStrategy
}

// Result holds the speed test results
type Result struct {
	Server           ServerInfo
	DownloadMbps     float64
	UploadMbps       float64
	Duration         time.Duration
	Latency          time.Duration
	Jitter           time.Duration
	MeasurementIndex int
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
	measurementCount := getEnvInt("SPEEDTEST_MEASUREMENT_COUNT", 1)
	if measurementCount < 1 {
		measurementCount = 1
	}

	strategy := MeasurementStrategy(getEnv("SPEEDTEST_MEASUREMENT_STRATEGY", string(MeasurementStrategySingleServer)))
	if !strategy.Valid() {
		fmt.Fprintf(os.Stderr, "Warning: Invalid measurement strategy '%s', defaulting to '%s'\n", strategy, MeasurementStrategySingleServer)
		strategy = MeasurementStrategySingleServer
	}

	// Parse server IDs from comma-separated list
	serverIDs := parseServerIDs(getEnv("SPEEDTEST_SERVER_ID", ""))

	// Validate server ID count
	if err := validateServerIDs(serverIDs, strategy, measurementCount); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	return Config{
		ServerIDs:           serverIDs,
		Timeout:             getEnvDuration("SPEEDTEST_TIMEOUT", 30*time.Second),
		ConcurrentStreams:   getEnvInt("SPEEDTEST_CONCURRENT_STREAMS", 0),
		TestDuration:        getEnvDuration("SPEEDTEST_TEST_DURATION", 0),
		SkipDownload:        getEnvBool("SPEEDTEST_SKIP_DOWNLOAD", false),
		SkipUpload:          getEnvBool("SPEEDTEST_SKIP_UPLOAD", false),
		MeasurementCount:    measurementCount,
		MeasurementStrategy: strategy,
	}
}

// parseServerIDs parses a comma-separated list of server IDs
func parseServerIDs(serverIDStr string) []string {
	if serverIDStr == "" {
		return []string{}
	}

	// Split by comma and trim whitespace
	parts := strings.Split(serverIDStr, ",")
	serverIDs := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			serverIDs = append(serverIDs, trimmed)
		}
	}

	return serverIDs
}

// validateServerIDs validates that the number of server IDs matches the strategy requirements
func validateServerIDs(serverIDs []string, strategy MeasurementStrategy, measurementCount int) error {
	idCount := len(serverIDs)

	switch strategy {
	case MeasurementStrategySingleServer:
		// Single-server mode: 0 or 1 server IDs allowed
		if idCount > 1 {
			return fmt.Errorf("in single-server mode, you can only specify 0 or 1 server ID (found %d)", idCount)
		}

	case MeasurementStrategyMultiServer:
		// Multi-server mode: 0 or exactly measurementCount server IDs allowed
		if idCount != 0 && idCount != measurementCount {
			return fmt.Errorf("in multi-server mode with %d measurements, you must provide exactly %d server IDs or none (found %d)",
				measurementCount, measurementCount, idCount)
		}
	}

	return nil
}

// NewRunner creates a new speed test runner
func NewRunner(config Config) *Runner {
	return &Runner{config: config}
}

// Run executes the speed test with tracing and returns all measurement results
func (r *Runner) Run(ctx context.Context) ([]*Result, error) {
	ctx, span := tracer.Start(ctx, "speedtest.execution")
	defer span.End()

	// Select servers based on strategy
	servers, err := r.selectServers(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "server selection failed")
		return nil, fmt.Errorf("server selection failed: %w", err)
	}

	span.SetAttributes(
		attribute.Int("measurement_count", r.config.MeasurementCount),
		attribute.String("measurement_strategy", string(r.config.MeasurementStrategy)),
	)

	results := make([]*Result, 0, r.config.MeasurementCount)

	// Run measurements
	for i := 0; i < r.config.MeasurementCount; i++ {
		measurementCtx, measurementSpan := tracer.Start(ctx, fmt.Sprintf("speedtest.measurement_%d", i+1))

		// Select server for this measurement
		var server *speedtest.Server
		if r.config.MeasurementStrategy == MeasurementStrategySingleServer {
			// Reuse the same server for all measurements
			server = servers[0]
		} else {
			// Use a different server for each measurement
			if i < len(servers) {
				server = servers[i]
			} else {
				// Fallback if we run out of servers
				server = servers[i%len(servers)]
			}
		}

		measurementSpan.SetAttributes(
			attribute.Int("measurement_index", i+1),
			attribute.String("speedtest.server.id", server.ID),
			attribute.String("speedtest.server.name", server.Name),
			attribute.String("speedtest.server.country", server.Country),
			attribute.Float64("speedtest.server.distance", server.Distance),
		)

		startTime := time.Now()

		result := &Result{
			Server: ServerInfo{
				ID:       server.ID,
				Name:     server.Name,
				Country:  server.Country,
				Distance: server.Distance,
			},
			MeasurementIndex: i + 1,
		}

		// Run download test
		if !r.config.SkipDownload {
			downloadMbps, err := r.runDownloadTest(measurementCtx, server)
			if err != nil {
				measurementSpan.RecordError(err)
				measurementSpan.SetStatus(codes.Error, "download test failed")
				measurementSpan.End()
				return nil, fmt.Errorf("download test failed for measurement %d: %w", i+1, err)
			}
			result.DownloadMbps = downloadMbps
			measurementSpan.SetAttributes(attribute.Float64("speedtest.download.mbps", downloadMbps))
		}

		// Run upload test
		if !r.config.SkipUpload {
			uploadMbps, err := r.runUploadTest(measurementCtx, server)
			if err != nil {
				measurementSpan.RecordError(err)
				measurementSpan.SetStatus(codes.Error, "upload test failed")
				measurementSpan.End()
				return nil, fmt.Errorf("upload test failed for measurement %d: %w", i+1, err)
			}
			result.UploadMbps = uploadMbps
			measurementSpan.SetAttributes(attribute.Float64("speedtest.upload.mbps", uploadMbps))
		}

		result.Duration = time.Since(startTime)
		result.Latency = server.Latency
		result.Jitter = server.Jitter

		measurementSpan.SetStatus(codes.Ok, "measurement completed successfully")
		measurementSpan.End()

		results = append(results, result)
	}

	span.SetStatus(codes.Ok, "speed test completed successfully")

	return results, nil
}

func (r *Runner) selectServers(ctx context.Context) ([]*speedtest.Server, error) {
	ctx, span := tracer.Start(ctx, "speedtest.server_selection")
	defer span.End()

	user := speedtest.New()

	// Fetch server list
	serverList, err := user.FetchServers()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch servers: %w", err)
	}

	var targets []*speedtest.Server

	// If specific server IDs are provided, use them
	if len(r.config.ServerIDs) > 0 {
		preferredServers := make([]int, 0, len(r.config.ServerIDs))
		for _, idStr := range r.config.ServerIDs {
			if id, err := strconv.Atoi(idStr); err == nil {
				preferredServers = append(preferredServers, id)
			} else {
				return nil, fmt.Errorf("invalid server ID '%s': %w", idStr, err)
			}
		}
		targets, err = serverList.FindServer(preferredServers)
		if err != nil {
			return nil, fmt.Errorf("failed to find servers with IDs %v: %w", r.config.ServerIDs, err)
		}
	} else {
		// No specific server IDs, use serverList directly
		// serverList is already the list of all available servers
		targets = serverList

		// Sort servers by latency (lowest first) for multi-server mode
		if r.config.MeasurementStrategy == MeasurementStrategyMultiServer {
			sort.Slice(targets, func(i, j int) bool {
				return targets[i].Latency < targets[j].Latency
			})
		}
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("no servers found")
	}

	// Select servers based on strategy
	var selectedServers []*speedtest.Server

	switch r.config.MeasurementStrategy {
	case MeasurementStrategySingleServer:
		// Use the same server for all measurements (best latency)
		selectedServers = []*speedtest.Server{targets[0]}

	case MeasurementStrategyMultiServer:
		// Use different servers for each measurement
		// If specific server IDs were provided, use all of them
		if len(r.config.ServerIDs) > 0 {
			selectedServers = targets
		} else {
			// Otherwise, select the N servers with lowest latency
			count := r.config.MeasurementCount
			if count > len(targets) {
				count = len(targets)
				fmt.Fprintf(os.Stderr, "Warning: Requested %d measurements but only %d servers available\n", r.config.MeasurementCount, len(targets))
			}
			selectedServers = targets[:count]
		}

	default:
		selectedServers = []*speedtest.Server{targets[0]}
	}

	span.SetAttributes(
		attribute.Int("server_count", len(selectedServers)),
		attribute.String("strategy", string(r.config.MeasurementStrategy)),
	)

	return selectedServers, nil
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
