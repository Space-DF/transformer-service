package services

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Space-DF/transformer-service/internal/models"
	redis "github.com/redis/go-redis/v9"
)

// ErrCacheMiss indicates that the requested key is not cached
var ErrCacheMiss = errors.New("cache miss")

// DeviceMappingCache defines the behaviour required for a cache backend
type DeviceMappingCache interface {
	Get(ctx context.Context, key string) (*models.DeviceMapping, error)
	Set(ctx context.Context, key string, mapping models.DeviceMapping) error
}

// redisDeviceMappingCache stores device mappings in Redis
type redisDeviceMappingCache struct {
	client *redis.Client
}

// newDeviceMappingCacheFromEnv returns a Redis-backed cache if configured via env vars
func newDeviceMappingCacheFromEnv() DeviceMappingCache {
	addr := strings.TrimSpace(os.Getenv("DEVICE_CACHE_REDIS_ADDR"))
	if addr == "" {
		return nil
	}

	dialTimeout := 2 * time.Second
	if raw := strings.TrimSpace(os.Getenv("DEVICE_CACHE_REDIS_DIAL_TIMEOUT_MS")); raw != "" {
		if ms, err := strconv.Atoi(raw); err == nil && ms > 0 {
			dialTimeout = time.Duration(ms) * time.Millisecond
		}
	}

	opts, err := parseRedisOptions(addr, dialTimeout)
	if err != nil {
		log.Printf("device-profile redis options error: %v", err)
		return nil
	}

	if pwd := os.Getenv("DEVICE_CACHE_REDIS_PASSWORD"); pwd != "" && opts.Password == "" {
		opts.Password = pwd
	}

	if raw := os.Getenv("DEVICE_CACHE_REDIS_DB"); raw != "" {
		if db, err := strconv.Atoi(raw); err == nil && db >= 0 {
			opts.DB = db
		}
	}

	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("device-profile redis ping error: %v", err)
		return nil
	}

	return &redisDeviceMappingCache{client: client}
}

// Get fetches a device mapping from Redis
func (c *redisDeviceMappingCache) Get(ctx context.Context, key string) (*models.DeviceMapping, error) {
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrCacheMiss
		}
		return nil, err
	}

	var cached models.DeviceMapping
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, err
	}

	m := cached
	return &m, nil
}

// Set stores a device mapping in Redis
func (c *redisDeviceMappingCache) Set(ctx context.Context, key string, mapping models.DeviceMapping) error {
	payload, err := json.Marshal(mapping)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, payload, 0).Err()
}

func parseRedisOptions(addr string, dialTimeout time.Duration) (*redis.Options, error) {
	if strings.HasPrefix(strings.ToLower(addr), "redis://") {
		opts, err := redis.ParseURL(addr)
		if err != nil {
			return nil, err
		}
		opts.DialTimeout = dialTimeout
		return opts, nil
	}
	return &redis.Options{
		Addr:        addr,
		DialTimeout: dialTimeout,
	}, nil
}
