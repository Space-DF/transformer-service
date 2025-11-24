package resolver

import (
	"errors"
	"fmt"

	"github.com/Space-DF/transformer-service/internal/models"
	"github.com/Space-DF/transformer-service/internal/mqtt/helpers"
	"github.com/Space-DF/transformer-service/internal/parsers"
	"github.com/Space-DF/transformer-service/internal/services"
)

var ErrDeviceSkipped = errors.New("device skipped")

type Resolver struct {
	locationService      *services.LocationService
	deviceProfileService *services.DeviceProfileService
	logTenant            TenantLogger
}

type TenantLogger func(orgSlug, vhost, emoji, format string, args ...interface{})

func New(locationService *services.LocationService, deviceProfileService *services.DeviceProfileService, log TenantLogger) *Resolver {
	return &Resolver{
		locationService:      locationService,
		deviceProfileService: deviceProfileService,
		logTenant:            log,
	}
}

func (r *Resolver) Resolve(orgSlug, vhost, devEUI string, payload, locationPayload map[string]interface{}) (*models.DeviceLocationData, *models.ProcessingInfo, error) {
	info := models.ProcessingInfo{
		HasLocationData: helpers.HasLocationData(locationPayload),
		GatewayCount:    helpers.CountGateways(locationPayload),
	}

	if devEUI != "" && r.deviceProfileService != nil {
		shouldSkip, skipErr := r.deviceProfileService.ShouldSkipDevice(orgSlug, devEUI)
		if skipErr != nil {
			r.logTenant(orgSlug, vhost, "⚠️", "Could not check skip status for device %s: %v", devEUI, skipErr)
		} else if shouldSkip {
			r.logTenant(orgSlug, vhost, "⏭️", "Skipping device %s as per configuration", devEUI)
			return nil, &info, ErrDeviceSkipped
		}
	}

	var deviceLocation *models.DeviceLocationData
	var err error

	if devEUI != "" && r.deviceProfileService != nil {
		requiresCalculation, profileErr := r.deviceProfileService.RequiresLocationCalculation(orgSlug, devEUI)
		if profileErr != nil {
			r.logTenant(orgSlug, vhost, "⚠️", "Could not get device profile for %s: %v. Proceeding with location calculation.", devEUI, profileErr)
			requiresCalculation = true // Default to requiring calculation
		}

		if requiresCalculation {
			r.logTenant(orgSlug, vhost, "📍", "Calculating location for device %s using trilateration", devEUI)
			// Calculate device location using decoded data if available, otherwise original payload
			deviceLocation, err = r.locationService.CalculateDeviceLocation(locationPayload)
			if err == nil && deviceLocation != nil {
				// Set organization from device mapping if available
				if _, mapping, mappingErr := r.deviceProfileService.GetDeviceProfile(orgSlug, devEUI); mappingErr == nil {
					deviceLocation.Organization = mapping.Organization
				}
			}
		} else {
			// Device has GPS, extract coordinates using device-specific parser
			r.logTenant(orgSlug, vhost, "🛰️", "Device %s has GPS capability, extracting GPS coordinates", devEUI)
			if _, mapping, profileErr := r.deviceProfileService.GetDeviceProfile(orgSlug, devEUI); profileErr == nil {
				deviceLocation, err = r.extractGPSFromDeviceParser(mapping.Profile, locationPayload, mapping.Organization)
			} else {
				r.logTenant(orgSlug, vhost, "⚠️", "Could not get device profile for %s: %v. Falling back to location calculation.", devEUI, profileErr)
				deviceLocation, err = r.locationService.CalculateDeviceLocation(locationPayload)
			}
		}
	} else {
		// No device profile service or devEUI, fall back to standard calculation
		r.logTenant(orgSlug, vhost, "⚠️", "No device profile service or devEUI, proceeding with location calculation")
		deviceLocation, err = r.locationService.CalculateDeviceLocation(locationPayload)
	}

	if deviceLocation != nil && deviceLocation.Organization == "" {
		deviceLocation.Organization = orgSlug
	}

	if err != nil {
		info.ErrorMessage = err.Error()
		return nil, &info, fmt.Errorf("failed to calculate device location: %w", err)
	}

	return deviceLocation, &info, nil
}

// extractGPSFromDeviceParser extracts GPS coordinates using device-specific parser
func (r *Resolver) extractGPSFromDeviceParser(profile string, payload map[string]interface{}, organization string) (*models.DeviceLocationData, error) {
	switch profile {
	case "RAK4630":
		return r.extractGPSFromRAK4630(payload, organization)
	default:
		return nil, fmt.Errorf("GPS extraction not implemented for device profile: %s", profile)
	}
}

// extractGPSFromRAK4630 extracts GPS coordinates from RAK4630 device using CBOR parsing
func (r *Resolver) extractGPSFromRAK4630(payload map[string]interface{}, organization string) (*models.DeviceLocationData, error) {
	// Create RAK4630 parser
	rak4630Parser := parsers.NewRAK4630Parser()

	// Use RAK4630 parser to extract GPS data from payload
	locationData, err := rak4630Parser.ParsePayload(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RAK4630 GPS data: %w", err)
	}

	// Set organization
	locationData.Organization = organization

	return locationData, nil
}
