package services

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	redis "github.com/redis/go-redis/v9"
)

// LocationEntry represents a cached location entry
type LocationEntry struct {
	Latitude  float64  `json:"latitude"`
	Longitude float64  `json:"longitude"`
	Accuracy  float64  `json:"accuracy,omitempty"`
	Bearing   *float64 `json:"bearing,omitempty"`
	Timestamp int64    `json:"timestamp"` // Unix timestamp in milliseconds
}

// LocationCache manages device location caching in memory
type LocationCache struct {
	mu         sync.RWMutex
	locations  map[string][]LocationEntry
	maxEntries int
	client     *redis.Client
	ttl        time.Duration
}

// NewLocationCache creates a new location cache instance
func NewLocationCache() *LocationCache {
	cache := &LocationCache{
		locations:  make(map[string][]LocationEntry),
		maxEntries: 2,
		ttl:        24 * time.Hour,
	}

	cache.initRedisFromEnv()

	return cache
}

func (c *LocationCache) SaveLocation(ctx context.Context, deviceID string, entry LocationEntry) error {
	if c.client != nil {
		return c.saveLocationRedis(ctx, deviceID, entry)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Get existing locations
	entries := c.locations[deviceID]

	// Add new entry at the front (newest first)
	entries = append([]LocationEntry{entry}, entries...)

	// Keep only the latest two
	if len(entries) > c.maxEntries {
		entries = entries[:c.maxEntries]
	}

	// Save back to cache
	c.locations[deviceID] = entries
	log.Printf("location-cache backend=memory action=save device=%s entries=%d", deviceID, len(entries))

	return nil
}

// GetLatestLocations retrieves the most recent location entries for a device
func (c *LocationCache) GetLatestLocations(ctx context.Context, deviceID string) ([]LocationEntry, error) {
	if c.client != nil {
		return c.getLatestLocationsRedis(ctx, deviceID)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	entries, exists := c.locations[deviceID]
	if !exists {
		log.Printf("location-cache backend=memory action=get device=%s hit=false entries=0", deviceID)
		return []LocationEntry{}, nil
	}

	// Return a copy to prevent external modifications
	result := make([]LocationEntry, len(entries))
	copy(result, entries)
	log.Printf("location-cache backend=memory action=get device=%s hit=true entries=%d", deviceID, len(result))

	return result, nil
}

func (c *LocationCache) initRedisFromEnv() {
	addr := strings.TrimSpace(firstNonEmpty(
		os.Getenv("LOCATION_CACHE_REDIS_ADDR"),
		os.Getenv("DEVICE_CACHE_REDIS_ADDR"),
	))
	if addr == "" {
		log.Printf("location-cache backend=memory reason=redis_addr_missing")
		return
	}

	dialTimeout := 2 * time.Second
	if raw := strings.TrimSpace(firstNonEmpty(
		os.Getenv("LOCATION_CACHE_REDIS_DIAL_TIMEOUT_MS"),
		os.Getenv("DEVICE_CACHE_REDIS_DIAL_TIMEOUT_MS"),
	)); raw != "" {
		if ms, err := strconv.Atoi(raw); err == nil && ms > 0 {
			dialTimeout = time.Duration(ms) * time.Millisecond
		}
	}

	opts, err := parseLocationRedisOptions(addr, dialTimeout)
	if err != nil {
		log.Printf("location-cache backend=memory reason=redis_options_error error=%v", err)
		return
	}

	if pwd := firstNonEmpty(
		os.Getenv("LOCATION_CACHE_REDIS_PASSWORD"),
		os.Getenv("DEVICE_CACHE_REDIS_PASSWORD"),
	); pwd != "" && opts.Password == "" {
		opts.Password = pwd
	}

	if raw := strings.TrimSpace(firstNonEmpty(
		os.Getenv("LOCATION_CACHE_REDIS_DB"),
		os.Getenv("DEVICE_CACHE_REDIS_DB"),
	)); raw != "" {
		if db, err := strconv.Atoi(raw); err == nil && db >= 0 {
			opts.DB = db
		}
	}

	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("location-cache backend=memory reason=redis_ping_error error=%v", err)
		return
	}

	c.client = client
	log.Printf("location-cache backend=redis status=connected addr=%s db=%d", opts.Addr, opts.DB)
}

func (c *LocationCache) saveLocationRedis(ctx context.Context, deviceID string, entry LocationEntry) error {
	key := c.redisKey(deviceID)

	payload, err := json.Marshal(entry)
	if err != nil {
		log.Printf("location-cache backend=redis action=marshal_entry device=%s key=%s error=%v", deviceID, key, err)
		return err
	}

	pipe := c.client.TxPipeline()
	pipe.LPush(ctx, key, payload)
	pipe.LTrim(ctx, key, 0, int64(c.maxEntries-1))
	pipe.Expire(ctx, key, c.ttl)

	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("location-cache backend=redis action=push_trim_expire device=%s key=%s max_entries=%d error=%v", deviceID, key, c.maxEntries, err)
		return err
	}

	length, err := c.client.LLen(ctx, key).Result()
	if err != nil {
		log.Printf("location-cache backend=redis action=llen device=%s key=%s error=%v", deviceID, key, err)
		return nil
	}

	log.Printf("location-cache backend=redis action=push_trim_expire device=%s key=%s entries=%d ttl=%s", deviceID, key, length, c.ttl)
	return nil
}

func (c *LocationCache) getLatestLocationsRedis(ctx context.Context, deviceID string) ([]LocationEntry, error) {
	key := c.redisKey(deviceID)

	data, err := c.client.LRange(ctx, key, 0, int64(c.maxEntries-1)).Result()
	if err != nil {
		log.Printf("location-cache backend=redis action=get device=%s key=%s error=%v", deviceID, key, err)
		return nil, err
	}
	if len(data) == 0 {
		log.Printf("location-cache backend=redis action=get device=%s key=%s hit=false entries=0", deviceID, key)
		return []LocationEntry{}, nil
	}

	entries := make([]LocationEntry, 0, len(data))
	for _, raw := range data {
		var entry LocationEntry
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			log.Printf("location-cache backend=redis action=unmarshal_entry device=%s key=%s error=%v", deviceID, key, err)
			return nil, err
		}
		entries = append(entries, entry)
	}

	log.Printf("location-cache backend=redis action=get device=%s key=%s hit=true entries=%d", deviceID, key, len(entries))
	return entries, nil
}

func (c *LocationCache) redisKey(deviceID string) string {
	return "location_history:v2:" + deviceID
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func parseLocationRedisOptions(addr string, dialTimeout time.Duration) (*redis.Options, error) {
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
