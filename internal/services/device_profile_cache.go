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

// DeviceRegistryCache interface - now enabled for Device Registry integration
type DeviceRegistryCache interface {
	// Existing DeviceMapping functionality for backward compatibility
	DeviceMappingCache
	
	// Org-aware Device Entry operations using interface{} to avoid circular imports
	GetDeviceEntry(ctx context.Context, org, deviceID string) (interface{}, error)
	SetDeviceEntry(ctx context.Context, org, deviceID string, device interface{}) error
	DeleteDeviceEntry(ctx context.Context, org, deviceID string) error
	
	// Fast org-specific identifier lookup
	GetDeviceByIdentifier(ctx context.Context, org, identifierType, key, value string) (string, error) // Returns deviceID
	SetIdentifierMapping(ctx context.Context, org, identifierType, key, value, deviceID string) error
	DeleteIdentifierMapping(ctx context.Context, org, identifierType, key, value string) error
	
	// Org-specific connection lookup
	GetDeviceByConnection(ctx context.Context, org, connectionType, value string) (string, error) // Returns deviceID
	SetConnectionMapping(ctx context.Context, org, connectionType, value, deviceID string) error
	DeleteConnectionMapping(ctx context.Context, org, connectionType, value string) error
}

// redisDeviceMappingCache stores device mappings in Redis
type redisDeviceMappingCache struct {
	client *redis.Client
}

// Ensure redisDeviceMappingCache implements DeviceRegistryCache
var _ DeviceRegistryCache = (*redisDeviceMappingCache)(nil)

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

// NewDeviceRegistryCacheFromEnv creates a Device Registry cache from environment variables
func NewDeviceRegistryCacheFromEnv() DeviceRegistryCache {
	// Reuse the same Redis client creation logic
	cache := newDeviceMappingCacheFromEnv()
	if cache == nil {
		return nil
	}
	
	// Type assert to the extended interface (since redisDeviceMappingCache implements DeviceRegistryCache)
	if registryCache, ok := cache.(*redisDeviceMappingCache); ok {
		return registryCache
	}
	
	return nil
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

// Device Registry Cache Implementation with organization isolation

// GetDeviceEntry retrieves a device entry from Redis (org-aware)
func (c *redisDeviceMappingCache) GetDeviceEntry(ctx context.Context, org, deviceID string) (interface{}, error) {
	key := c.deviceEntryKey(org, deviceID)
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrCacheMiss
		}
		return nil, err
	}

	// Return raw JSON map to avoid circular imports
	var device map[string]interface{}
	if err := json.Unmarshal(data, &device); err != nil {
		return nil, err
	}

	return device, nil
}

// SetDeviceEntry stores a device entry in Redis (org-aware)
func (c *redisDeviceMappingCache) SetDeviceEntry(ctx context.Context, org, deviceID string, device interface{}) error {
	key := c.deviceEntryKey(org, deviceID)
	data, err := json.Marshal(device)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, data, 0).Err()
}

// DeleteDeviceEntry removes a device entry from Redis (org-aware)
func (c *redisDeviceMappingCache) DeleteDeviceEntry(ctx context.Context, org, deviceID string) error {
	key := c.deviceEntryKey(org, deviceID)
	return c.client.Del(ctx, key).Err()
}

// GetDeviceByIdentifier implements fast org-specific identifier lookup
func (c *redisDeviceMappingCache) GetDeviceByIdentifier(ctx context.Context, org, identifierType, key, value string) (string, error) {
	indexKey := c.identifierIndexKey(org, identifierType, key, value)
	deviceID, err := c.client.Get(ctx, indexKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrCacheMiss
		}
		return "", err
	}
	return deviceID, nil
}

// SetIdentifierMapping stores an org-specific identifier → device_id mapping
func (c *redisDeviceMappingCache) SetIdentifierMapping(ctx context.Context, org, identifierType, key, value, deviceID string) error {
	indexKey := c.identifierIndexKey(org, identifierType, key, value)
	return c.client.Set(ctx, indexKey, deviceID, 0).Err()
}

// DeleteIdentifierMapping removes an org-specific identifier mapping
func (c *redisDeviceMappingCache) DeleteIdentifierMapping(ctx context.Context, org, identifierType, key, value string) error {
	indexKey := c.identifierIndexKey(org, identifierType, key, value)
	return c.client.Del(ctx, indexKey).Err()
}

// GetDeviceByConnection implements org-specific connection lookup
func (c *redisDeviceMappingCache) GetDeviceByConnection(ctx context.Context, org, connectionType, value string) (string, error) {
	indexKey := c.connectionIndexKey(org, connectionType, value)
	deviceID, err := c.client.Get(ctx, indexKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrCacheMiss
		}
		return "", err
	}
	return deviceID, nil
}

// SetConnectionMapping stores an org-specific connection → device_id mapping
func (c *redisDeviceMappingCache) SetConnectionMapping(ctx context.Context, org, connectionType, value, deviceID string) error {
	indexKey := c.connectionIndexKey(org, connectionType, value)
	return c.client.Set(ctx, indexKey, deviceID, 0).Err()
}

// DeleteConnectionMapping removes an org-specific connection mapping
func (c *redisDeviceMappingCache) DeleteConnectionMapping(ctx context.Context, org, connectionType, value string) error {
	indexKey := c.connectionIndexKey(org, connectionType, value)
	return c.client.Del(ctx, indexKey).Err()
}

// Redis key generators (org-aware, following existing device_cache pattern)
func (c *redisDeviceMappingCache) deviceEntryKey(org, deviceID string) string {
	return "device_registry:" + org + ":entries:" + deviceID
}

func (c *redisDeviceMappingCache) identifierIndexKey(org, identifierType, key, value string) string {
	return "device_registry:" + org + ":" + identifierType + ":" + key + ":" + value
}

func (c *redisDeviceMappingCache) connectionIndexKey(org, connectionType, value string) string {
	return "device_registry:" + org + ":connections:" + connectionType + ":" + value
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
