package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Space-DF/transformer-service-go/internal/models"
)

// DeviceProfileService handles device profile management
type DeviceProfileService struct {
	profiles *models.DeviceProfiles
}

// NewDeviceProfileService creates a new device profile service
func NewDeviceProfileService(configPath string) (*DeviceProfileService, error) {
	service := &DeviceProfileService{}
	
	if err := service.LoadProfiles(configPath); err != nil {
		return nil, fmt.Errorf("failed to load device profiles: %w", err)
	}
	
	return service, nil
}

// LoadProfiles loads device profiles from a JSON configuration file
func (dps *DeviceProfileService) LoadProfiles(configPath string) error {
	// If configPath is relative, make it absolute from project root
	if !filepath.IsAbs(configPath) {
		pwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
		configPath = filepath.Join(pwd, configPath)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read device profiles config file: %w", err)
	}

	var profiles models.DeviceProfiles
	if err := json.Unmarshal(data, &profiles); err != nil {
		return fmt.Errorf("failed to unmarshal device profiles: %w", err)
	}

	dps.profiles = &profiles
	return nil
}

// GetDeviceProfile returns the device profile for a given devEUI
func (dps *DeviceProfileService) GetDeviceProfile(devEUI string) (*models.DeviceProfile, *models.DeviceMapping, error) {
	if dps.profiles == nil {
		return nil, nil, fmt.Errorf("device profiles not loaded")
	}

	// Get device mapping
	mapping, exists := dps.profiles.DeviceMappings[devEUI]
	if !exists {
		return nil, nil, fmt.Errorf("device with devEUI %s not found in mappings", devEUI)
	}

	// Get device profile
	profile, exists := dps.profiles.DeviceProfiles[mapping.Profile]
	if !exists {
		return nil, nil, fmt.Errorf("device profile %s not found", mapping.Profile)
	}

	return &profile, &mapping, nil
}

// GetAllProfiles returns all available device profiles
func (dps *DeviceProfileService) GetAllProfiles() map[string]models.DeviceProfile {
	if dps.profiles == nil {
		return nil
	}
	return dps.profiles.DeviceProfiles
}

// GetAllMappings returns all device mappings
func (dps *DeviceProfileService) GetAllMappings() map[string]models.DeviceMapping {
	if dps.profiles == nil {
		return nil
	}
	return dps.profiles.DeviceMappings
}

// AddDeviceMapping adds a new device mapping
func (dps *DeviceProfileService) AddDeviceMapping(devEUI string, mapping models.DeviceMapping) error {
	if dps.profiles == nil {
		return fmt.Errorf("device profiles not loaded")
	}

	// Validate that the profile exists
	if _, exists := dps.profiles.DeviceProfiles[mapping.Profile]; !exists {
		return fmt.Errorf("device profile %s does not exist", mapping.Profile)
	}

	dps.profiles.DeviceMappings[devEUI] = mapping
	return nil
}

// HasGPS checks if a device has built-in GPS capability
func (dps *DeviceProfileService) HasGPS(devEUI string) (bool, error) {
	profile, _, err := dps.GetDeviceProfile(devEUI)
	if err != nil {
		return false, err
	}
	return profile.HasGPS, nil
}

// RequiresLocationCalculation checks if a device requires location calculation
func (dps *DeviceProfileService) RequiresLocationCalculation(devEUI string) (bool, error) {
	profile, _, err := dps.GetDeviceProfile(devEUI)
	if err != nil {
		return true, err // Default to requiring calculation if profile not found
	}
	return profile.LocationCalculationRequired, nil
}

// GetParserType returns the parser type for a device
func (dps *DeviceProfileService) GetParserType(devEUI string) (string, error) {
	profile, _, err := dps.GetDeviceProfile(devEUI)
	if err != nil {
		return "", err
	}
	return profile.ParserType, nil
}

// ShouldSkipDevice checks if a device should be skipped from processing
func (dps *DeviceProfileService) ShouldSkipDevice(devEUI string) (bool, error) {
	_, mapping, err := dps.GetDeviceProfile(devEUI)
	if err != nil {
		return false, err
	}
	return mapping.Skip, nil
}