package services

import (
	"fmt"
	"time"

	"github.com/Space-DF/transformer-service/internal/components"
	"github.com/Space-DF/transformer-service/internal/models"
)

// EntityTransformService handles transformation from ParseResult to telemetry output
type EntityTransformService struct {
	deviceProfileService *DeviceProfileService
	entityCacheService   *EntityCacheService
}

// NewEntityTransformService creates a new entity transform service
func NewEntityTransformService(deviceProfileService *DeviceProfileService, entityCacheService *EntityCacheService) *EntityTransformService {
	return &EntityTransformService{
		deviceProfileService: deviceProfileService,
		entityCacheService:   entityCacheService,
	}
}

// TransformToTelemetry converts ParseResult to TelemetryPayload for telemetry service
func (ts *EntityTransformService) TransformToTelemetry(parseResult *components.ParseResult, orgSlug string, originalPayload map[string]interface{}) (*models.TelemetryPayload, error) {
	if parseResult == nil {
		return nil, fmt.Errorf("parse result is nil")
	}

	// Extract device identifiers from mappings
	deviceID, spaceSlug := ts.extractDeviceIdentifiers(originalPayload, orgSlug, parseResult.DeviceEUI)

	// Convert device info
	deviceInfo := models.TelemetryDeviceInfo{
		Identifiers:  parseResult.DeviceInfo.Identifiers,
		Name:         parseResult.DeviceInfo.Name,
		Manufacturer: parseResult.DeviceInfo.Manufacturer,
		Model:        parseResult.DeviceInfo.Model,
		ModelID:      parseResult.DeviceInfo.ModelID,
	}

	// Convert entities
	var telemetryEntities []models.TelemetryEntity
	for _, entity := range parseResult.Entities {
		telemetryEntity := models.TelemetryEntity{
			UniqueID:    entity.UniqueID,
			EntityID:    entity.EntityID,
			EntityType:  entity.EntityType,
			DeviceClass: entity.DeviceClass,
			Name:        entity.Name,
			State:       entity.State,
			Attributes:  entity.Attributes,
			UnitOfMeas:  entity.UnitOfMeas,
			Icon:        entity.Icon,
			Timestamp:   entity.Timestamp.Format(time.RFC3339),
		}
		telemetryEntities = append(telemetryEntities, telemetryEntity)
	}

	// Extract additional metadata from original payload
	metadata := ts.extractMetadata(originalPayload)

	// Create telemetry payload
	telemetryPayload := &models.TelemetryPayload{
		Organization: orgSlug,
		DeviceEUI:    parseResult.DeviceEUI,
		DeviceID:     deviceID,
		SpaceSlug:    spaceSlug,
		DeviceInfo:   deviceInfo,
		Entities:     telemetryEntities,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Source:       "transformer-service",
		Metadata:     metadata,
	}

	return telemetryPayload, nil
}

// UpdateEntityLocation updates a location entity with calculated coordinates
func (ts *EntityTransformService) UpdateEntityLocation(orgSlug, deviceEUI string, latitude, longitude, accuracy float64, calculationMethod string) error {
	if ts.entityCacheService == nil {
		return fmt.Errorf("entity cache service not available")
	}

	// Find location entity for device
	uniqueID := components.GenerateUniqueID(orgSlug, deviceEUI, "location")

	// Update entity state and attributes
	state := "home" // Could be calculated based on geofences
	attributes := map[string]interface{}{
		"latitude":           latitude,
		"longitude":          longitude,
		"accuracy":           accuracy,
		"calculation_method": calculationMethod,
		"last_updated":       time.Now().UTC().Format(time.RFC3339),
	}

	return ts.entityCacheService.UpdateEntityState(nil, orgSlug, uniqueID, state, attributes)
}

// TransformLocationData converts legacy location data to entity-based format (for backward compatibility)
func (ts *EntityTransformService) TransformLocationData(deviceLocation *models.DeviceLocationData, gatewayCount int, originalPayload map[string]interface{}) (*models.TelemetryPayload, error) {
	if deviceLocation == nil {
		return nil, fmt.Errorf("device location data is nil")
	}

	// Determine accuracy based on gateway count
	accuracy := ts.determineLocationAccuracy(gatewayCount)

	// Extract device identifiers
	deviceID, spaceSlug := ts.extractDeviceIdentifiers(originalPayload, deviceLocation.Organization, deviceLocation.DevEUI)

	// Create simplified device info (we don't have full device info from legacy data)
	deviceInfo := models.TelemetryDeviceInfo{
		Identifiers: []string{deviceLocation.DevEUI},
		Name:        fmt.Sprintf("Device %s", deviceLocation.DevEUI[12:]),
		// Manufacturer and Model would need to be determined from device profiles
	}

	// Create location entity
	locationEntity := models.TelemetryEntity{
		UniqueID:    components.GenerateUniqueID(deviceLocation.Organization, deviceLocation.DevEUI, "location"),
		EntityID:    components.GenerateEntityID("device_tracker", deviceLocation.Organization, "unknown", "unknown", deviceLocation.DevEUI, "location"),
		EntityType:  "location",
		DeviceClass: "location",
		Name:        "Location",
		State:       "home",
		Attributes: map[string]interface{}{
			"latitude":           deviceLocation.Latitude,
			"longitude":          deviceLocation.Longitude,
			"accuracy":           accuracy,
			"gateway_count":      gatewayCount,
			"calculation_method": ts.getCalculationMethod(gatewayCount),
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	// Extract metadata
	metadata := ts.extractMetadata(originalPayload)

	return &models.TelemetryPayload{
		Organization: deviceLocation.Organization,
		DeviceEUI:    deviceLocation.DevEUI,
		DeviceID:     deviceID,
		SpaceSlug:    spaceSlug,
		DeviceInfo:   deviceInfo,
		Entities:     []models.TelemetryEntity{locationEntity},
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Source:       "transformer-service",
		Metadata:     metadata,
	}, nil
}

// Helper methods (reused from original transform service)

func (ts *EntityTransformService) determineLocationAccuracy(gatewayCount int) float64 {
	switch gatewayCount {
	case 0:
		return 0 // GPS accuracy
	case 1:
		return 300 // ~300 m error
	case 2:
		return 100 // ~100 m error
	case 3:
		return 40 // ~40 m error
	default: // 4+ gateways
		return 20 // ~20 m error
	}
}

func (ts *EntityTransformService) getCalculationMethod(gatewayCount int) string {
	if gatewayCount == 0 {
		return "gps"
	}
	return "trilateration"
}

func (ts *EntityTransformService) extractMetadata(payload map[string]interface{}) map[string]interface{} {
	metadata := make(map[string]interface{})

	// Add received timestamp if available
	if receivedAt, exists := payload["received_at"]; exists {
		metadata["received_at"] = receivedAt
	}

	// Extract uplink message metadata
	var uplinkMessage map[string]interface{}
	if msg, ok := payload["uplink_message"].(map[string]interface{}); ok {
		uplinkMessage = msg
	} else {
		uplinkMessage = payload
	}

	if len(uplinkMessage) > 0 {
		// Add frequency information
		if settings, ok := uplinkMessage["settings"].(map[string]interface{}); ok {
			if frequency, exists := settings["frequency"]; exists {
				metadata["frequency"] = frequency
			}
		}

		// Add gateway information
		var rxMetadata []interface{}
		var metadataOk bool

		if rxMetadata, metadataOk = uplinkMessage["rx_metadata"].([]interface{}); !metadataOk {
			if rxMetadata, metadataOk = uplinkMessage["gateways"].([]interface{}); !metadataOk {
				if rxMetadata, metadataOk = uplinkMessage["gateway_info"].([]interface{}); !metadataOk {
					rxMetadata, metadataOk = uplinkMessage["rxInfo"].([]interface{})
				}
			}
		}

		if metadataOk {
			var gateways []map[string]interface{}
			for _, gw := range rxMetadata {
				if gateway, ok := gw.(map[string]interface{}); ok {
					gatewayInfo := make(map[string]interface{})

					// Add gateway ID
					if gatewayID, exists := gateway["gateway_ids"]; exists {
						gatewayInfo["gateway_id"] = gatewayID
					} else if gatewayID, exists := gateway["gatewayId"]; exists {
						gatewayInfo["gateway_id"] = gatewayID
					}

					// Add RSSI and SNR
					if rssi, exists := gateway["rssi"]; exists {
						gatewayInfo["rssi"] = rssi
					}
					if snr, exists := gateway["snr"]; exists {
						gatewayInfo["snr"] = snr
					}
					if location, exists := gateway["location"]; exists {
						gatewayInfo["location"] = location
					}

					if len(gatewayInfo) > 0 {
						gateways = append(gateways, gatewayInfo)
					}
				}
			}
			if len(gateways) > 0 {
				metadata["gateways"] = gateways
			}
		}

		// Add frame counter and port
		if fCnt, exists := uplinkMessage["f_cnt"]; exists {
			metadata["frame_counter"] = fCnt
		}
		if fPort, exists := uplinkMessage["f_port"]; exists {
			metadata["port"] = fPort
		}
	}

	// Add correlation IDs and application info
	if correlationIDs, exists := payload["correlation_ids"]; exists {
		metadata["correlation_ids"] = correlationIDs
	}

	if endDeviceIDs, ok := payload["end_device_ids"].(map[string]interface{}); ok {
		if applicationIDs, exists := endDeviceIDs["application_ids"]; exists {
			metadata["application"] = applicationIDs
		}
	}

	return metadata
}

func (ts *EntityTransformService) extractDeviceIdentifiers(payload map[string]interface{}, organization, devEUI string) (string, string) {
	deviceID := "unknown"
	spaceSlug := ""

	// Try to get from device profile service
	if ts.deviceProfileService != nil && organization != "" && devEUI != "" {
		if _, mapping, err := ts.deviceProfileService.GetDeviceProfile(organization, devEUI); err == nil && mapping != nil {
			if mapping.DeviceID != "" {
				deviceID = mapping.DeviceID
			}
			if mapping.SpaceSlug != "" {
				spaceSlug = mapping.SpaceSlug
			}
		}
	}

	// Fallback to payload data
	if deviceID == "unknown" {
		if rawDeviceID, exists := payload["device_id"]; exists {
			if strVal, ok := rawDeviceID.(string); ok && strVal != "" {
				deviceID = strVal
			}
		}
	}

	if rawSpaceSlug, exists := payload["space_slug"]; exists {
		if strVal, ok := rawSpaceSlug.(string); ok && strVal != "" {
			spaceSlug = strVal
		}
	}

	return deviceID, spaceSlug
}
