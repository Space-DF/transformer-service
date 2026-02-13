package services

import (
	"fmt"
	"time"

	"github.com/Space-DF/transformer-service/internal/models"
)

// TransformService handles data transformation
type TransformService struct {
	deviceProfileService *DeviceProfileService
}

// NewTransformService creates a new transform service
func NewTransformService(deviceProfileService *DeviceProfileService) *TransformService {
	return &TransformService{
		deviceProfileService: deviceProfileService,
	}
}

// TransformDeviceData transforms device location data to the standardized output format
func (ts *TransformService) TransformDeviceData(deviceLocation *models.DeviceLocationData, gatewayCount int, originalPayload map[string]interface{}) (*models.TransformedDeviceData, error) {
	if deviceLocation == nil {
		return nil, fmt.Errorf("device location data is nil")
	}

	// Determine location accuracy based on calculation method
	accuracy := ts.determineLocationAccuracy(gatewayCount)

	// Extract additional metadata from original payload
	metadata := ts.extractMetadata(originalPayload)

	// Extract device identifiers (device + space) from payload or device mappings
	deviceID, spaceSlug, isPublished := ts.extractDeviceIdentifiers(originalPayload, deviceLocation.Organization, deviceLocation.DevEUI)

	// Create transformed data structure
	transformedData := &models.TransformedDeviceData{
		DeviceEUI:   deviceLocation.DevEUI,
		DeviceID:    deviceID,
		SpaceSlug:   spaceSlug,
		IsPublished: isPublished,
		Location: models.LocationCoordinates{
			Latitude:  deviceLocation.Latitude,
			Longitude: deviceLocation.Longitude,
			Accuracy:  accuracy,
		},
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Organization: deviceLocation.Organization,
		Metadata:     metadata,
		Source:       "transformer-service",
	}

	return transformedData, nil
}

// determineLocationAccuracy analyzes the original payload to determine location accuracy
func (ts *TransformService) determineLocationAccuracy(gatewayCount int) float64 {
	// Determine accuracy based on number of gateways
	switch gatewayCount {
	case 0:
		return 0 // from GPS, no estimate
	case 1:
		return 300 // ~300 m error
	case 2:
		return 100 // ~100 m error
	case 3:
		return 40 // ~40 m error
	default: // multiple gateways (4+)
		return 20 // ~20 m (or better) error
	}
}

// extractMetadata extracts useful metadata from the original payload
func (ts *TransformService) extractMetadata(payload map[string]interface{}) map[string]interface{} {
	metadata := make(map[string]interface{})

	// Add received timestamp if available
	if receivedAt, exists := payload["received_at"]; exists {
		metadata["received_at"] = receivedAt
	}

	// Extract uplink message metadata - prioritize ChirpStack format
	var uplinkMessage map[string]interface{}

	// Check for ChirpStack decoded_raw_data first
	if decoded, ok := payload["decoded_raw_data"].(map[string]interface{}); ok {
		uplinkMessage = decoded
	} else if msg, ok := payload["uplink_message"].(map[string]interface{}); ok {
		uplinkMessage = msg
	} else {
		uplinkMessage = payload
	}

	// Extract basic LoRaWAN metadata
	ts.extractLoRaWANMetadata(uplinkMessage, metadata)

	// Add correlation IDs if available
	if correlationIDs, exists := payload["correlation_ids"]; exists {
		metadata["correlation_ids"] = correlationIDs
	}

	// Add application information if available
	if endDeviceIDs, ok := payload["end_device_ids"].(map[string]interface{}); ok {
		if applicationIDs, exists := endDeviceIDs["application_ids"]; exists {
			metadata["application"] = applicationIDs
		}
	}

	return metadata
}

// extractLoRaWANMetadata extracts LoRaWAN-specific metadata from uplinkEvent
func (ts *TransformService) extractLoRaWANMetadata(uplinkMessage map[string]interface{}, metadata map[string]interface{}) {
	// For your data format, LoRaWAN data is consistently in uplinkEvent
	uplinkEvent, ok := uplinkMessage["uplinkEvent"].(map[string]interface{})
	if !ok {
		return
	}

	// Add frame counter
	if fCnt, exists := uplinkEvent["fCnt"]; exists {
		metadata["frame_counter"] = fCnt
	}

	// Add port information
	if fPort, exists := uplinkEvent["fPort"]; exists {
		metadata["port"] = fPort
	}

	// Add frequency information
	if txInfo, ok := uplinkEvent["txInfo"].(map[string]interface{}); ok {
		if frequency, exists := txInfo["frequency"]; exists {
			metadata["frequency"] = frequency
		}
	}

	// Add gateway information from rxInfo format
	gateways := ts.extractGatewayInfo(uplinkEvent)
	if len(gateways) > 0 {
		metadata["gateways"] = gateways
	}
}

// extractGatewayInfo extracts gateway information from rxInfo format
func (ts *TransformService) extractGatewayInfo(uplinkMessage map[string]interface{}) []map[string]interface{} {
	var gateways []map[string]interface{}

	// Look for rxInfo in the uplinkMessage
	if rxMetadata, ok := uplinkMessage["rxInfo"].([]interface{}); ok {
		for _, gw := range rxMetadata {
			if gateway, ok := gw.(map[string]interface{}); ok {
				gatewayInfo := make(map[string]interface{})

				// Extract gateway fields
				if gatewayID, exists := gateway["gatewayId"]; exists {
					gatewayInfo["gateway_id"] = gatewayID
				}
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
	}

	return gateways
}

// extractDeviceIdentifiers extracts device and space identifiers from device profile service
func (ts *TransformService) extractDeviceIdentifiers(_ map[string]interface{}, organization, devEUI string) (string, string, bool) {
	deviceID := "unknown"
	spaceSlug := ""
	isPublished := false

	// Get device identifiers from device profile service (authoritative source)
	if ts.deviceProfileService != nil && organization != "" && devEUI != "" {
		if mapping, err := ts.deviceProfileService.GetDeviceMapping(organization, devEUI); err == nil && mapping != nil {
			deviceID = mapping.DeviceID
			spaceSlug = mapping.SpaceSlug
			isPublished = mapping.IsPublished
		}
	}

	return deviceID, spaceSlug, isPublished
}
