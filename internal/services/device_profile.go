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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Space-DF/transformer-service/internal/models"
)

type cacheEntry struct {
	mapping models.DeviceMapping
}

// DeviceProfileService handles device profile management
type DeviceProfileService struct {
	httpClient  *http.Client
	baseURL     string
	cache       map[string]cacheEntry
	cacheLocker sync.RWMutex
	cacheStore  DeviceMappingCache
}

// NewDeviceProfileService creates a new device profile service
func NewDeviceProfileService() (*DeviceProfileService, error) {
	service := &DeviceProfileService{
		cache: make(map[string]cacheEntry),
	}

	service.baseURL = strings.TrimSpace(os.Getenv("DEVICE_SERVICE_BASE_URL"))

	timeout := 5 * time.Second
	if raw := strings.TrimSpace(os.Getenv("DEVICE_SERVICE_TIMEOUT_SECONDS")); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			timeout = time.Duration(seconds) * time.Second
		}
	}

	service.httpClient = &http.Client{Timeout: timeout}

	service.cacheStore = newDeviceMappingCacheFromEnv()

	return service, nil
}

func (dps *DeviceProfileService) GetDeviceMapping(orgSlug, devEUI string) (*models.DeviceMapping, error) {
	if devEUI == "" {
		return nil, fmt.Errorf("dev_eui is required")
	}

	return dps.getMapping(orgSlug, devEUI)
}

// Get mapping device
func (dps *DeviceProfileService) getMapping(orgSlug, devEUI string) (*models.DeviceMapping, error) {
	version := 1
	cacheKey := fmt.Sprintf(":%d:%s:lorawan:%s", version, orgSlug, devEUI)

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

	var payload models.DeviceLookupResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	profile := ""
	if rawProfile, ok := payload["device_profile"].(map[string]interface{}); ok {
		if deviceType, ok := rawProfile["device_type"].(string); ok {
			profile = strings.TrimSpace(deviceType)
		}
	}
	if profile == "" {
		return nil, fmt.Errorf("device mapping payload missing profile for %s", devEUI)
	}

	deviceID := strings.TrimSpace(payload.ID)
	if deviceID == "" {
		deviceID = "unknown-" + devEUI
	}

	deviceName := strings.TrimSpace(payload.DeviceProfile.Name)
	if deviceName == "" {
		deviceName = deviceID
	}

	manufacture := strings.TrimSpace(payload.DeviceProfile.Manufacture)
	if manufacture == "" {
		manufacture = "unknown"
	}

	description := strings.TrimSpace(payload.DeviceProfile.Description)

	spaceSlug := strings.TrimSpace(payload.SpaceSlug)
	isPublished := payload.IsPublished
	skip := payload.Skip

	log.Printf("device mapping lookup: dev_eui=%s, profile=%s, device_id=%s, device_name=%s, manufacture=%s, description=%s, space_slug=%s, is_published=%v,  skip=%v",
		devEUI, profile, deviceID, deviceName, manufacture, description, spaceSlug, isPublished, skip)
	mapping := models.DeviceMapping{
		Profile:      profile,
		Organization: orgSlug,
		DeviceID:     deviceID,
		DeviceName:   deviceName,
		Manufacture:  manufacture,
		Description:  description,
		SpaceSlug:    spaceSlug,
		IsPublished:  isPublished,
		Skip:         skip,
	}

	return &mapping, nil
}

func (dps *DeviceProfileService) getFromCache(key string) (*models.DeviceMapping, bool) {
	dps.cacheLocker.RLock()
	entry, ok := dps.cache[key]
	dps.cacheLocker.RUnlock()
	if !ok {
		if dps.cacheStore != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			cached, err := dps.cacheStore.Get(ctx, key)
			if err != nil {
				if !errors.Is(err, ErrCacheMiss) {
					log.Printf("device-profile redis get error: %v", err)
				}
				return nil, false
			}

			dps.cacheLocker.Lock()
			dps.cache[key] = cacheEntry{mapping: *cached}
			dps.cacheLocker.Unlock()
			return cached, true
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

	if dps.cacheStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := dps.cacheStore.Set(ctx, key, mapping); err != nil {
			log.Printf("device-profile redis set error: %v", err)
		}
	}
}

// ShouldSkipDevice checks if a device should be skipped from processing.
func (dps *DeviceProfileService) ShouldSkipDevice(orgSlug, devEUI string) (bool, error) {
	mapping, err := dps.GetDeviceMapping(orgSlug, devEUI)
	if err != nil {
		return false, err
	}
	return mapping.Skip, nil
}
