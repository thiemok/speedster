# Speedster

A lightweight internet speed monitoring application that measures download and upload speeds, then reports the results as OpenTelemetry metrics and traces.

## Features

- 🚀 Fast and lightweight Go application
- 📊 OpenTelemetry metrics (download, upload, latency, jitter)
- 🔍 Distributed tracing for detailed execution visibility
- 🐳 Small Docker image (~10-15MB)
- ☸️ Kubernetes CronJob deployment via Helm
- 🔧 Highly configurable
- 🔐 Support for authenticated OTEL endpoints

## Metrics

The following metrics are exported:

| Metric Name | Type | Description | Unit | Labels |
|-------------|------|-------------|------|--------|
| `speedtest_download_mbps` | Gauge | Download speed | Mbps | server_id, server_name, server_location, server_country |
| `speedtest_upload_mbps` | Gauge | Upload speed | Mbps | server_id, server_name, server_location, server_country |
| `speedtest_latency_ms` | Gauge | Latency | ms | server_id, server_name, server_location, server_country |
| `speedtest_jitter_ms` | Gauge | Jitter | ms | server_id, server_name, server_location, server_country |

## Traces

The application emits detailed spans:

```
speedtest.execution (root span)
├── speedtest.server_selection
├── speedtest.download_test
├── speedtest.upload_test
└── speedtest.metrics_export
```

Each span includes attributes like server information, test results, and timing data.

## Installation

### Docker Images

Docker images are available from GitHub Container Registry with multi-architecture support:
- **Platforms**: `linux/amd64`, `linux/arm64`
- **Registry**: `ghcr.io/thiemok/speedster`

### Helm Charts

Helm charts are distributed as OCI artifacts via GitHub Container Registry: `oci://ghcr.io/thiemok/charts/speedster`

## Quick Start

### Prerequisites

- Go 1.21+ (for building)
- Docker (for containerization)
- Kubernetes cluster (for deployment)
- Helm 3+ (for deployment)

### Building Locally

```bash
# Download dependencies
go mod download

# Build the binary
go build -o speedster ./cmd/speedster

# Run locally (requires OTEL endpoint)
export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4318"
./speedster
```

### Using Docker Image

```bash
# Pull from GitHub Container Registry
docker pull ghcr.io/thiemok/speedster:latest

# Or pull a specific version
docker pull ghcr.io/thiemok/speedster:1.0.0

# Run the container
docker run --rm \
  -e OTEL_EXPORTER_OTLP_ENDPOINT="http://host.docker.internal:4318" \
  ghcr.io/thiemok/speedster:latest
```

### Building Docker Image Locally

```bash
# Build the image
docker build -t speedster:latest .

# Run the container
docker run --rm \
  -e OTEL_EXPORTER_OTLP_ENDPOINT="http://host.docker.internal:4318" \
  speedster:latest
```

## Kubernetes Deployment

### Installing with Helm

```bash
# Install latest version from GitHub Container Registry
helm install speedster oci://ghcr.io/thiemok/charts/speedster \
  --set otel.endpoint="http://otel-collector:4318"

# Install specific version
helm install speedster oci://ghcr.io/thiemok/charts/speedster \
  --version 1.0.0 \
  --set otel.endpoint="http://otel-collector:4318"

# Install with custom values
helm install speedster oci://ghcr.io/thiemok/charts/speedster \
  --set otel.endpoint="http://otel-collector:4318" \
  --set otel.serviceName="speedster" \
  --set otel.serviceNamespace="monitoring" \
  --set cronjob.schedule="0 * * * *"

# Install with OTEL authentication
helm install speedster oci://ghcr.io/thiemok/charts/speedster \
  --set otel.endpoint="http://otel-collector:4318" \
  --set-string otel.headers.authorization="Bearer YOUR_TOKEN"

# Upgrade to latest version
helm upgrade speedster oci://ghcr.io/thiemok/charts/speedster

# Uninstall
helm uninstall speedster
```

### Installing with Helm (from local chart)

```bash
# Install from local chart directory
helm install speedster ./helm/speedster \
  --set otel.endpoint="http://otel-collector:4318"
```

### Example values.yaml

```yaml
image:
  repository: your-registry/speedster
  tag: 1.0.0
  pullPolicy: IfNotPresent

cronjob:
  schedule: "0 * * * *"  # Every hour

otel:
  endpoint: "http://otel-collector:4318"
  serviceName: "speedster"
  serviceNamespace: "monitoring"
  headers:
    authorization: "Bearer your-token-here"

speedtest:
  timeout: 30

resources:
  limits:
    cpu: 500m
    memory: 128Mi
  requests:
    cpu: 100m
    memory: 64Mi
```

## Configuration

### Environment Variables

#### OpenTelemetry Configuration

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP endpoint URL | - | Yes |
| `OTEL_EXPORTER_OTLP_HEADERS` | Authentication headers | - | No |
| `OTEL_SERVICE_NAME` | Service name | `speedster` | No |
| `OTEL_SERVICE_NAMESPACE` | Service namespace | - | No |

#### Speedtest Configuration

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `SPEEDTEST_SERVER_ID` | Pin to specific server | - | No |
| `SPEEDTEST_TIMEOUT` | Test timeout (seconds) | `30` | No |
| `SPEEDTEST_CONCURRENT_STREAMS` | Concurrent streams | `0` (library default) | No |
| `SPEEDTEST_TEST_DURATION` | Test duration (seconds) | `0` (library default) | No |
| `SPEEDTEST_SKIP_DOWNLOAD` | Skip download test | `false` | No |
| `SPEEDTEST_SKIP_UPLOAD` | Skip upload test | `false` | No |

#### Application Configuration

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `LOG_LEVEL` | Log level (debug, info, warn, error) | `info` | No |

### Helm Values

See [helm/speedster/values.yaml](helm/speedster/values.yaml) for all available configuration options.

## Development

### Running Tests

```bash
go test ./...
```

### Building for Different Platforms

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o speedster-linux ./cmd/speedster

# macOS
GOOS=darwin GOARCH=amd64 go build -o speedster-macos ./cmd/speedster

# Windows
GOOS=windows GOARCH=amd64 go build -o speedster.exe ./cmd/speedster
```

## License

MIT

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
