# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o speedster ./cmd/speedster

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /build/speedster .

# Run as non-root user
RUN addgroup -g 1000 speedster && \
    adduser -D -u 1000 -G speedster speedster && \
    chown -R speedster:speedster /app

USER speedster

ENTRYPOINT ["/app/speedster"]
