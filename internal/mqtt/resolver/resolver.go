package resolver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Space-DF/transformer-service/internal/components"
	"github.com/Space-DF/transformer-service/internal/components/registry"
	"github.com/Space-DF/transformer-service/internal/models"
	"github.com/Space-DF/transformer-service/internal/mqtt/helpers"
	"github.com/Space-DF/transformer-service/internal/services"

	// Import component packages to trigger registration
	_ "github.com/Space-DF/transformer-service/internal/components/dut"
	_ "github.com/Space-DF/transformer-service/internal/components/rakwireless"
	_ "github.com/Space-DF/transformer-service/internal/components/seeed"
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
		mapping, mappingErr := r.deviceProfileService.GetDeviceMapping(orgSlug, devEUI)
		if mappingErr != nil {
			r.logTenant(orgSlug, vhost, "⚠️", "Could not get device mapping for %s: %v. Proceeding with location calculation.", devEUI, mappingErr)
		}

		deviceType := components.DeviceTypeUnknown
		if mapping != nil {
			deviceType = r.profileToDeviceType(mapping.Profile)
		}

		requiresCalculation := true
		if components := registry.GetGlobalRegistry().GetComponentsForDevice(deviceType); len(components) > 0 {
			requiresCalculation = !components[0].SupportsGPS(deviceType)
		}

		if requiresCalculation {
			r.logTenant(orgSlug, vhost, "📍", "Calculating location for device %s using trilateration", devEUI)
			// Calculate device location using decoded data if available, otherwise original payload
			deviceLocation, err = r.locationService.CalculateDeviceLocation(locationPayload)
			if err == nil && deviceLocation != nil {
				// Set organization from device mapping if available
				if mapping != nil {
					deviceLocation.Organization = mapping.Organization
					deviceLocation.Manufacture = mapping.Manufacture
				}
			}
		} else {
			// Device has GPS, extract coordinates using device-specific parser
			r.logTenant(orgSlug, vhost, "🛰️", "Device %s has GPS capability, extracting GPS coordinates", devEUI)
			deviceLocation, err = r.extractGPSFromDeviceParser(mapping.Profile, locationPayload, orgSlug)
			if err == nil && deviceLocation != nil && mapping != nil {
				deviceLocation.Manufacture = mapping.Manufacture
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

// extractGPSFromDeviceParser extracts GPS coordinates using component-based parser
func (r *Resolver) extractGPSFromDeviceParser(profile string, payload map[string]interface{}, organization string) (*models.DeviceLocationData, error) {
	// Convert profile to device type
	deviceType := r.profileToDeviceType(profile)
	if deviceType == components.DeviceTypeUnknown {
		return nil, fmt.Errorf("unknown device profile: %s", profile)
	}

	// Create raw payload structure for component system
	rawPayload := &components.RawPayload{
		DeviceEUI: extractDevEUIFromPayload(payload),
		Timestamp: time.Now(),
		Metadata:  payload,
	}

	// Find component that can handle this device type
	component := registry.FindComponent(deviceType, rawPayload)
	if component == nil {
		return nil, fmt.Errorf("no component found for device type: %s", deviceType)
	}

	// Check if device supports GPS
	if !component.SupportsGPS(deviceType) {
		return nil, fmt.Errorf("device type %s does not support GPS", deviceType)
	}

	// Parse using the component
	parsedData, err := component.Parse(r.ctx(), deviceType, rawPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GPS data with component: %w", err)
	}

	// Convert to DeviceLocationData
	if parsedData.Location == nil {
		return nil, fmt.Errorf("no GPS location found in parsed data")
	}

	locationData := &models.DeviceLocationData{
		Latitude:     parsedData.Location.Latitude,
		Longitude:    parsedData.Location.Longitude,
		DevEUI:       parsedData.DeviceEUI,
		Organization: organization,
	}

	return locationData, nil
}

// profileToDeviceType converts a profile string to DeviceType
func (r *Resolver) profileToDeviceType(profile string) components.DeviceType {
	switch profile {
	case "RAK2270":
		return components.DeviceTypeRAK2270
	case "RAK7200":
		return components.DeviceTypeRAK7200
	case "RAK4630":
		return components.DeviceTypeRAK4630
	case "WLBV1":
		return components.DeviceTypeWLBV1
	case "SENSECAP_T1000":
		return components.DeviceTypeSenseCAP_T1000
	default:
		return components.DeviceTypeUnknown
	}
}

// extractDevEUIFromPayload extracts DevEUI from various payload formats
func extractDevEUIFromPayload(payload map[string]interface{}) string {
	// Try multiple locations for device EUI
	if endDeviceIDs, ok := payload["end_device_ids"].(map[string]interface{}); ok {
		if devEUI, ok := endDeviceIDs["dev_eui"].(string); ok {
			return devEUI
		}
	}

	if devEUI, ok := payload["dev_eui"].(string); ok {
		return devEUI
	}

	if devEUI, ok := payload["devEui"].(string); ok {
		return devEUI
	}

	if deviceInfo, ok := payload["deviceInfo"].(map[string]interface{}); ok {
		if devEUI, ok := deviceInfo["devEui"].(string); ok {
			return devEUI
		}
	}

	return ""
}

// ctx returns a background context for component operations
func (r *Resolver) ctx() context.Context {
	return context.Background()
}
