package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Space-DF/transformer-service/internal/models"
	redis "github.com/redis/go-redis/v9"
)

type cacheEntry struct {
	mapping models.DeviceMapping
}

// DeviceProfileService handles device profile management
type DeviceProfileService struct {
	profiles map[string]models.DeviceProfile

	httpClient  *http.Client
	baseURL     string
	cache       map[string]cacheEntry
	cacheLocker sync.RWMutex
	redisClient *redis.Client
}

// NewDeviceProfileService creates a new device profile service
func NewDeviceProfileService(configPath string) (*DeviceProfileService, error) {
	service := &DeviceProfileService{
		cache: make(map[string]cacheEntry),
	}

	if err := service.LoadProfiles(configPath); err != nil {
		return nil, fmt.Errorf("failed to load device profiles: %w", err)
	}

	service.baseURL = strings.TrimSpace(os.Getenv("DEVICE_SERVICE_BASE_URL"))

	timeout := 5 * time.Second
	if raw := strings.TrimSpace(os.Getenv("DEVICE_SERVICE_TIMEOUT_SECONDS")); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			timeout = time.Duration(seconds) * time.Second
		}
	}

	service.httpClient = &http.Client{Timeout: timeout}

	if client := newRedisClientFromEnv(); client != nil {
		service.redisClient = client
	}

	return service, nil
}

// LoadProfiles loads device profiles from a JSON configuration file
func (dps *DeviceProfileService) LoadProfiles(configPath string) error {
	pwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// If configPath is relative, make it absolute from project root
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(pwd, configPath)
	}

	// Clean and validate the path to prevent directory traversal
	configPath = filepath.Clean(configPath)

	// Validate that the path stays within the project directory
	allowedDir := filepath.Clean(pwd)
	if !strings.HasPrefix(configPath, allowedDir+string(filepath.Separator)) && configPath != allowedDir {
		return fmt.Errorf("config file path is outside allowed directory")
	}
	
	// Validate file extension
	if filepath.Ext(configPath) != ".json" {
		return fmt.Errorf("config file must have .json extension")
	}

	data, err := os.ReadFile(configPath) // #nosec G304 - path is validated above
	if err != nil {
		return fmt.Errorf("failed to read device profiles config file: %w", err)
	}

	var cfg models.DeviceProfiles
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to unmarshal device profiles: %w", err)
	}

	dps.profiles = cfg.DeviceProfiles
	return nil
}

// GetDeviceProfile returns the device profile and mapping for a given organization + DevEUI.
func (dps *DeviceProfileService) GetDeviceProfile(orgSlug, devEUI string) (*models.DeviceProfile, *models.DeviceMapping, error) {
	if dps.profiles == nil {
		return nil, nil, fmt.Errorf("device profiles not loaded")
	}

	if devEUI == "" {
		return nil, nil, fmt.Errorf("dev_eui is required")
	}

	mapping, err := dps.getMapping(orgSlug, devEUI)
	if err != nil {
		return nil, nil, err
	}

	profile, ok := dps.profiles[mapping.Profile]
	if !ok {
		return nil, nil, fmt.Errorf("device profile %s not found", mapping.Profile)
	}

	return &profile, mapping, nil
}

// Get mapping device
func (dps *DeviceProfileService) getMapping(orgSlug, devEUI string) (*models.DeviceMapping, error) {
	cacheKey := orgSlug + ":" + devEUI

	// Get mapping device from Redis
	// If there's no data in Redis, it calls an API to get the mapping device
	if mapping, ok := dps.getFromCache(cacheKey); ok {
		return mapping, nil
	}

	if dps.baseURL == "" {
		return nil, fmt.Errorf("device mapping for %s not found", devEUI)
	}

	// Call API to get mapping device with 2 params: orgSlug and devEUI
	mapping, err := dps.lookupViaDeviceService(orgSlug, devEUI)
	if err != nil {
		return nil, err
	}

	// Get response from API calling and save to Redis
	dps.saveToCache(cacheKey, *mapping)

	return mapping, nil
}

// API calling to look up device
func (dps *DeviceProfileService) lookupViaDeviceService(orgSlug, devEUI string) (*models.DeviceMapping, error) {
	endpoint := fmt.Sprintf("%s/devices/%s/internal",
		strings.TrimRight(dps.baseURL, "/"),
		url.QueryEscape(devEUI),
	)

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Organization", orgSlug)

	resp, err := dps.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	log.Printf("device service response: status=%s, headers=%v", resp.Status, resp.Header)
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("device mapping for %s not found", devEUI)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("device service error: %s - %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	profile := valueToString(payload["device_profile"])
	
	if profile == "" {
		return nil, fmt.Errorf("device mapping payload missing profile for %s", devEUI)
	}

	deviceID := valueToString(payload["device_id"])
	if deviceID == "" {
		deviceID = valueToString(payload["id"])
	}

	deviceName := valueToString(payload["device_name"])
	if deviceName == "" {
		deviceName = deviceID
	}

	description := valueToString(payload["description"])

	skip := false
	if rawSkip, ok := payload["skip"]; ok {
		switch v := rawSkip.(type) {
		case bool:
			skip = v
		case string:
			skip = strings.EqualFold(v, "true")
		case float64:
			skip = v != 0
		}
	}
	log.Printf("device mapping lookup: dev_eui=%s, profile=%s, device_id=%s, device_name=%s, description=%s, skip=%v",
		devEUI, profile, deviceID, deviceName, description, skip)
	mapping := models.DeviceMapping{
		Profile:      profile,
		Organization: orgSlug,
		DeviceID:     deviceID,
		DeviceName:   deviceName,
		Description:  description,
		Skip:         skip,
	}

	return &mapping, nil
}

func (dps *DeviceProfileService) getFromCache(key string) (*models.DeviceMapping, bool) {
	dps.cacheLocker.RLock()
	entry, ok := dps.cache[key]
	dps.cacheLocker.RUnlock()
	if !ok {
		if dps.redisClient != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			data, err := dps.redisClient.Get(ctx, key).Bytes()
			if err != nil {
				if !errors.Is(err, redis.Nil) {
					log.Printf("device-profile redis get error: %v", err)
				}
				return nil, false
			}

			var cached models.DeviceMapping
			if err := json.Unmarshal(data, &cached); err != nil {
				log.Printf("device-profile redis unmarshal error: %v", err)
				return nil, false
			}

			dps.cacheLocker.Lock()
			dps.cache[key] = cacheEntry{mapping: cached}
			dps.cacheLocker.Unlock()
			return &cached, true
		}
		return nil, false
	}

	mappingCopy := entry.mapping
	return &mappingCopy, true
}

func (dps *DeviceProfileService) saveToCache(key string, mapping models.DeviceMapping) {
	entry := cacheEntry{mapping: mapping}

	dps.cacheLocker.Lock()
	dps.cache[key] = entry
	dps.cacheLocker.Unlock()

	if dps.redisClient != nil {
		payload, err := json.Marshal(mapping)
		if err != nil {
			log.Printf("device-profile redis marshal error: %v", err)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := dps.redisClient.Set(ctx, key, payload, 0).Err(); err != nil {
			log.Printf("device-profile redis set error: %v", err)
		}
	}
}

// GetAllProfiles returns all available device profiles.
func (dps *DeviceProfileService) GetAllProfiles() map[string]models.DeviceProfile {
	return dps.profiles
}

// HasGPS checks if a device has built-in GPS capability.
func (dps *DeviceProfileService) HasGPS(orgSlug, devEUI string) (bool, error) {
	profile, _, err := dps.GetDeviceProfile(orgSlug, devEUI)
	if err != nil {
		return false, err
	}
	return profile.HasGPS, nil
}

// RequiresLocationCalculation checks if a device requires location calculation
func (dps *DeviceProfileService) RequiresLocationCalculation(orgSlug, devEUI string) (bool, error) {
	profile, _, err := dps.GetDeviceProfile(orgSlug, devEUI)
	if err != nil {
		return true, err // Default to requiring calculation if profile not found
	}
	return profile.LocationCalculationRequired, nil
}

// GetParserType returns the parser type for a device
func (dps *DeviceProfileService) GetParserType(orgSlug, devEUI string) (string, error) {
	profile, _, err := dps.GetDeviceProfile(orgSlug, devEUI)
	if err != nil {
		return "", err
	}
	return profile.ParserType, nil
}

// ShouldSkipDevice checks if a device should be skipped from processing.
func (dps *DeviceProfileService) ShouldSkipDevice(orgSlug, devEUI string) (bool, error) {
	_, mapping, err := dps.GetDeviceProfile(orgSlug, devEUI)
	if err != nil {
		return false, err
	}
	return mapping.Skip, nil
}

func newRedisClientFromEnv() *redis.Client {
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
	return client
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

func valueToString(value interface{}) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
		return s
	}
}
