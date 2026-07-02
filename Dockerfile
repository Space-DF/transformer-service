# syntax=docker/dockerfile:1.7
# Build stage
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source
COPY . .

# Build with cache
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o main ./cmd/transformer

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates procps

WORKDIR /root/

COPY --from=builder /app/main .
COPY --from=builder /app/configs ./configs

CMD ["./main", "serve"]