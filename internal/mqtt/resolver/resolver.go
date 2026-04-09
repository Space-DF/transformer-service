package resolver

import (
	"context"
	"errors"
	"fmt"
	"time"

	device_profiles "github.com/Space-DF/transformer-service/internal/device_profiles"
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
	"github.com/Space-DF/transformer-service/internal/lns"
	"github.com/Space-DF/transformer-service/internal/models"
	"github.com/Space-DF/transformer-service/internal/mqtt/helpers"
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

func (r *Resolver) Resolve(orgSlug, vhost, devEUI string, payload, locationPayload map[string]interface{}, lnsType ...lns.LNSType) (*models.DeviceLocationData, *models.ProcessingInfo, error) {
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

		deviceType := common.DeviceTypeUnknown
		if mapping != nil {
			deviceType = r.profileToDeviceType(mapping.Profile)
		}

		requiresCalculation := true
		if components := device_profiles.Global(); components != nil {
			requiresCalculation = !components.SupportsGPS(deviceType)
		}

		if requiresCalculation {
			r.logTenant(orgSlug, vhost, "📍", "Calculating location for device %s using trilateration", devEUI)
			// Calculate device location using decoded data if available, otherwise original payload
			// Use LNS-aware extraction
			deviceLocation, err = r.locationService.CalculateDeviceLocationWithLNS(locationPayload, r.getLNSType(lnsType...))
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
			// Pass LNS type if provided
			deviceLocation, err = r.extractGPSFromDeviceParser(mapping.Profile, locationPayload, orgSlug, r.getLNSType(lnsType...))
			if err == nil && deviceLocation != nil && mapping != nil {
				deviceLocation.Manufacture = mapping.Manufacture
			}
		}
	} else {
		// No device profile service or devEUI, fall back to standard calculation
		r.logTenant(orgSlug, vhost, "⚠️", "No device profile service or devEUI, proceeding with location calculation")
		deviceLocation, err = r.locationService.CalculateDeviceLocationWithLNS(locationPayload, r.getLNSType(lnsType...))
	}

	if deviceLocation != nil && deviceLocation.Organization == "" {
		deviceLocation.Organization = orgSlug
	}

	// Set LocationCalculated flag based on successful location calculation
	if deviceLocation != nil && deviceLocation.Latitude != 0 && deviceLocation.Longitude != 0 {
		info.LocationCalculated = true
	}

	if err != nil {
		info.ErrorMessage = err.Error()
		return nil, &info, fmt.Errorf("failed to calculate device location: %w", err)
	}

	return deviceLocation, &info, nil
}

// extractGPSFromDeviceParser extracts GPS coordinates using component-based parser
func (r *Resolver) extractGPSFromDeviceParser(profile string, payload map[string]interface{}, organization string, lnsType lns.LNSType) (*models.DeviceLocationData, error) {
	// Convert profile to device type
	deviceType := r.profileToDeviceType(profile)
	if deviceType == common.DeviceTypeUnknown {
		return nil, fmt.Errorf("unknown device profile: %s", profile)
	}

	deviceEUI := lns.ExtractDevEUI(payload, lnsType)
	rawPayload := &common.RawPayload{
		DeviceEUI: deviceEUI,
		Timestamp: time.Now(),
		Metadata:  payload,
		LNSType:   lnsType,
	}

	components := device_profiles.Global()
	if components == nil || !components.CanHandle(deviceType, rawPayload) {
		return nil, fmt.Errorf("no component found for device type nb: %s", deviceType)
	}

	if !components.SupportsGPS(deviceType) {
		return nil, fmt.Errorf("device type %s does not support GPS", deviceType)
	}

	parsedData, err := components.Parse(r.ctx(), deviceType, rawPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GPS data with component: %w", err)
	}

	locationData := &models.DeviceLocationData{
		DevEUI:       parsedData.DeviceEUI,
		Organization: organization,
	}

	// Only set coordinates if they exist
	if parsedData.Location != nil {
		locationData.Latitude = parsedData.Location.Latitude
		locationData.Longitude = parsedData.Location.Longitude
	}

	return locationData, nil
}

// profileToDeviceType converts a profile string to DeviceType.
func (r *Resolver) profileToDeviceType(profile string) common.DeviceType {
	if profile == "" {
		return common.DeviceTypeUnknown
	}
	dt := common.DeviceType(profile)
	if components := device_profiles.Global(); components != nil && components.CanHandle(dt, nil) {
		return dt
	}

	return common.DeviceTypeUnknown
}

// ctx returns a background context for component operations
func (r *Resolver) ctx() context.Context {
	return context.Background()
}

// getLNSType safely extracts LNS type from variadic args, defaulting to unknown
func (r *Resolver) getLNSType(lnsType ...lns.LNSType) lns.LNSType {
	if len(lnsType) > 0 && lnsType[0].Valid() {
		return lnsType[0]
	}
	return lns.LNSTypeUnknown
}
