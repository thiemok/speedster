package metrics

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/thiemok/speedster/pkg/speedtest"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

var (
	meter metric.Meter

	downloadGauge metric.Float64Gauge
	uploadGauge   metric.Float64Gauge
	latencyGauge  metric.Int64Gauge
	jitterGauge   metric.Int64Gauge
)

// InitOTEL initializes OpenTelemetry metrics and tracing
func InitOTEL(ctx context.Context) (func(context.Context) error, error) {
	// Create resource with service information
	res, err := newResource(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Initialize metrics
	metricShutdown, err := initMetrics(ctx, res)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize metrics: %w", err)
	}

	// Initialize tracing
	traceShutdown, err := initTracing(ctx, res)
	if err != nil {
		_ = metricShutdown(ctx)
		return nil, fmt.Errorf("failed to initialize tracing: %w", err)
	}

	// Create meters and instruments
	meter = otel.Meter("speedster")

	downloadGauge, err = meter.Float64Gauge(
		"speedtest_download_mbps",
		metric.WithDescription("Download speed in Mbps"),
		metric.WithUnit("Mbps"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create download gauge: %w", err)
	}

	uploadGauge, err = meter.Float64Gauge(
		"speedtest_upload_mbps",
		metric.WithDescription("Upload speed in Mbps"),
		metric.WithUnit("Mbps"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload gauge: %w", err)
	}

	latencyGauge, err = meter.Int64Gauge(
		"speedtest_latency_ns",
		metric.WithDescription("Latency in nanoseconds"),
		metric.WithUnit("ns"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create latency gauge: %w", err)
	}

	jitterGauge, err = meter.Int64Gauge(
		"speedtest_jitter_ns",
		metric.WithDescription("Jitter in nanoseconds"),
		metric.WithUnit("ns"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create jitter gauge: %w", err)
	}

	// Return combined shutdown function
	return func(ctx context.Context) error {
		var errs []error
		if err := metricShutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("metric shutdown: %w", err))
		}
		if err := traceShutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("trace shutdown: %w", err))
		}
		if len(errs) > 0 {
			return fmt.Errorf("shutdown errors: %v", errs)
		}
		return nil
	}, nil
}

func newResource(ctx context.Context) (*resource.Resource, error) {
	serviceName := getEnv("OTEL_SERVICE_NAME", "speedster")
	serviceNamespace := getEnv("OTEL_SERVICE_NAMESPACE", "")

	attrs := []attribute.KeyValue{
		semconv.ServiceName(serviceName),
	}

	if serviceNamespace != "" {
		attrs = append(attrs, semconv.ServiceNamespace(serviceNamespace))
	}

	return resource.New(ctx,
		resource.WithAttributes(attrs...),
		resource.WithProcessRuntimeName(),
		resource.WithProcessRuntimeVersion(),
		resource.WithProcessRuntimeDescription(),
		resource.WithHost(),
	)
}

func initMetrics(ctx context.Context, res *resource.Resource) (func(context.Context) error, error) {
	exporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create metric exporter: %w", err)
	}

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter,
			sdkmetric.WithInterval(10*time.Second))),
	)

	otel.SetMeterProvider(meterProvider)

	return meterProvider.Shutdown, nil
}

func initTracing(ctx context.Context, res *resource.Resource) (func(context.Context) error, error) {
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tracerProvider)

	return tracerProvider.Shutdown, nil
}

// RecordSpeedTestMetrics records the speed test results as metrics
func RecordSpeedTestMetrics(ctx context.Context, result *speedtest.Result) error {
	attrs := []attribute.KeyValue{
		attribute.String("server_id", result.Server.ID),
		attribute.String("server_name", result.Server.Name),
		attribute.String("server_country", result.Server.Country),
		attribute.Int("measurement_index", result.MeasurementIndex),
	}

	opts := metric.WithAttributes(attrs...)

	downloadGauge.Record(ctx, result.DownloadMbps, opts)
	uploadGauge.Record(ctx, result.UploadMbps, opts)
	latencyGauge.Record(ctx, result.Latency.Nanoseconds(), opts)
	jitterGauge.Record(ctx, result.Jitter.Nanoseconds(), opts)

	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
