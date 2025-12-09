# Build stage
FROM golang:1.24.4-alpine AS builder

# Install dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/transformer

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests and procps for health check
RUN apk --no-cache add ca-certificates procps

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/main .
COPY --from=builder /app/configs ./configs

# No port exposure needed for MQTT consumer

# Health check - verify main process is running
# HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
#   CMD ps aux | grep -q '[m]ain serve' || exit 1

# Command to run
CMD ["./main", "serve"]