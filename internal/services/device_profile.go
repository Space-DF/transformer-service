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
	"gopkg.in/yaml.v3"
)

// DeviceProfileService handles device profile management
type DeviceProfileService struct {
	httpClient    *http.Client
	baseURL       string
	cacheStore    DeviceMappingCache
	profiles      map[string]*models.DeviceProfile // map[device_type]profile
	profilesByID  map[string]*models.DeviceProfile // map[profile_id]profile
	manufacturers map[string]*models.Manufacturer  // map[manufacturer_id]manufacturer
	profilesMutex sync.RWMutex
}

// NewDeviceProfileService creates a new device profile service
func NewDeviceProfileService() (*DeviceProfileService, error) {
	service := &DeviceProfileService{
		profiles:      make(map[string]*models.DeviceProfile),
		profilesByID:  make(map[string]*models.DeviceProfile),
		manufacturers: make(map[string]*models.Manufacturer),
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

	// Load profiles and manufacturers from YAML
	if err := service.loadProfilesFromYAML(); err != nil {
		log.Printf("Warning: failed to load device profiles from YAML: %v", err)
	}
	if err := service.loadManufacturersFromYAML(); err != nil {
		log.Printf("Warning: failed to load manufacturers from YAML: %v", err)
	}

	return service, nil
}

// loadProfilesFromYAML loads device profiles from YAML configuration
func (dps *DeviceProfileService) loadProfilesFromYAML() error {
	profilePath := filepath.Join("configs", "device_model", "device_profile.yaml")
	profileData, err := os.ReadFile(profilePath) //#nosec #G304
	if err != nil {
		return fmt.Errorf("failed to read device profile config: %w", err)
	}

	var profileConfig models.DeviceProfileConfig
	if err := yaml.Unmarshal(profileData, &profileConfig); err != nil {
		return fmt.Errorf("failed to parse device profile config: %w", err)
	}

	dps.profilesMutex.Lock()
	defer dps.profilesMutex.Unlock()

	for i := range profileConfig.DeviceProfiles {
		profile := &profileConfig.DeviceProfiles[i]
		dps.profiles[profile.DeviceType] = profile
		if profile.ID != "" {
			dps.profilesByID[profile.ID] = profile
		}
	}

	log.Printf("Loaded %d device profiles from YAML", len(dps.profiles))
	return nil
}

// loadManufacturersFromYAML loads manufacturers from YAML configuration
func (dps *DeviceProfileService) loadManufacturersFromYAML() error {
	manufacturerPath := filepath.Join("configs", "device_model", "manufacturers.yaml")
	manufacturerData, err := os.ReadFile(manufacturerPath) //#nosec #G304
	if err != nil {
		return fmt.Errorf("failed to read manufacturers config: %w", err)
	}

	var manufacturerConfig models.ManufacturerConfig
	if err := yaml.Unmarshal(manufacturerData, &manufacturerConfig); err != nil {
		return fmt.Errorf("failed to parse manufacturers config: %w", err)
	}

	dps.profilesMutex.Lock()
	defer dps.profilesMutex.Unlock()

	for i := range manufacturerConfig.Manufacturers {
		manufacturer := &manufacturerConfig.Manufacturers[i]
		dps.manufacturers[manufacturer.ID] = manufacturer
	}

	log.Printf("Loaded %d manufacturers from YAML", len(dps.manufacturers))
	return nil
}

// GetProfileByDeviceType returns a device profile by device type
func (dps *DeviceProfileService) GetProfileByDeviceType(deviceType string) (*models.DeviceProfile, error) {
	dps.profilesMutex.RLock()
	defer dps.profilesMutex.RUnlock()

	profile, ok := dps.profiles[deviceType]
	if !ok {
		return nil, fmt.Errorf("device profile not found for type: %s", deviceType)
	}

	return profile, nil
}

// GetManufacturerByID returns a manufacturer by ID
func (dps *DeviceProfileService) GetManufacturerByID(manufacturerID string) (*models.Manufacturer, error) {
	dps.profilesMutex.RLock()
	defer dps.profilesMutex.RUnlock()

	manufacturer, ok := dps.manufacturers[manufacturerID]
	if !ok {
		return nil, fmt.Errorf("manufacturer not found for ID: %s", manufacturerID)
	}

	return manufacturer, nil
}

// GetProfileByID returns a device profile by its UUID
func (dps *DeviceProfileService) GetProfileByID(profileID string) (*models.DeviceProfile, error) {
	dps.profilesMutex.RLock()
	defer dps.profilesMutex.RUnlock()

	profile, ok := dps.profilesByID[profileID]
	if !ok {
		return nil, fmt.Errorf("device profile not found for ID: %s", profileID)
	}

	return profile, nil
}

// GetAllDeviceModels returns all device models with manufacturer names resolved.
func (dps *DeviceProfileService) GetAllDeviceModels() []models.DeviceModel {
	dps.profilesMutex.RLock()
	defer dps.profilesMutex.RUnlock()

	result := make([]models.DeviceModel, 0, len(dps.profilesByID))
	for _, profile := range dps.profilesByID {
		manufacturerName := profile.ManufacturerID
		if m, ok := dps.manufacturers[profile.ManufacturerID]; ok {
			manufacturerName = m.Name
		}
		result = append(result, models.DeviceModel{
			ID:               profile.ID,
			Name:             profile.Name,
			ManufacturerID:   profile.ManufacturerID,
			ManufacturerName: manufacturerName,
			DeviceType:       profile.DeviceType,
			KeyFeature:       profile.KeyFeature,
		})
	}
	return result
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

	// Look up device profile from YAML using device_model UUID
	deviceModelID := strings.TrimSpace(payload.DeviceModel)
	if deviceModelID == "" {
		return nil, fmt.Errorf("device mapping payload missing device_model for %s", devEUI)
	}

	// Get device profile from YAML by profile ID
	profile, err := dps.GetProfileByID(deviceModelID)
	if err != nil {
		log.Printf("Warning: could not find profile for device_model %s: %v", deviceModelID, err)
	}

	deviceType := "unknown"
	manufacturerName := "unknown"
	deviceName := ""
	description := ""

	if profile != nil {
		deviceType = profile.DeviceType
		if profile.ManufacturerID != "" {
			if manufacturer, err := dps.GetManufacturerByID(profile.ManufacturerID); err == nil {
				manufacturerName = manufacturer.Name
			}
		}
		deviceName = profile.Name
	}

	deviceID := strings.TrimSpace(payload.ID)
	if deviceID == "" {
		deviceID = "unknown-" + devEUI
	}

	if deviceName == "" {
		deviceName = deviceID
	}

	spaceSlug := strings.TrimSpace(payload.SpaceSlug)
	isPublished := payload.IsPublished

	log.Printf("device mapping lookup: dev_eui=%s, device_model=%s, profile=%s, device_id=%s, device_name=%s, manufacture=%s, space_slug=%s, is_published=%v",
		devEUI, deviceModelID, deviceType, deviceID, deviceName, manufacturerName, spaceSlug, isPublished)

	mapping := models.DeviceMapping{
		Profile:      deviceType,
		Organization: orgSlug,
		DeviceID:     deviceID,
		DeviceName:   deviceName,
		Manufacture:  manufacturerName,
		Description:  description,
		SpaceSlug:    spaceSlug,
		IsPublished:  isPublished,
	}

	return &mapping, nil
}

func (dps *DeviceProfileService) getFromCache(key string) (*models.DeviceMapping, bool) {
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

		return cached, true
	}

	return nil, false
}

func (dps *DeviceProfileService) saveToCache(key string, mapping models.DeviceMapping) {
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
