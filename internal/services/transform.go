package services

import (
	"fmt"
	"time"

	"github.com/Space-DF/transformer-service/internal/models"
)

// TransformService handles data transformation
type TransformService struct{
	deviceProfileService *DeviceProfileService
}

// NewTransformService creates a new transform service
func NewTransformService(deviceProfileService *DeviceProfileService) *TransformService {
	return &TransformService{
		deviceProfileService: deviceProfileService,
	}
}

// TransformDeviceData transforms device location data to the standardized output format
func (ts *TransformService) TransformDeviceData(deviceLocation *models.DeviceLocationData, originalPayload map[string]interface{}) (*models.TransformedDeviceData, error) {
	if deviceLocation == nil {
		return nil, fmt.Errorf("device location data is nil")
	}

	// Determine location accuracy based on calculation method
	accuracy := ts.determineLocationAccuracy(originalPayload)

	// Extract additional metadata from original payload
	metadata := ts.extractMetadata(originalPayload)

	// Extract device ID from payload or device mappings
	deviceID := ts.extractDeviceID(originalPayload, deviceLocation.Organization, deviceLocation.DevEUI)

	// Create transformed data structure
	transformedData := &models.TransformedDeviceData{
		DeviceEUI: deviceLocation.DevEUI,
		DeviceID:  deviceID,
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
func (ts *TransformService) determineLocationAccuracy(payload map[string]interface{}) string {
	// Try to extract uplink message, if not found, use payload directly
	var uplinkMessage map[string]interface{}
	if msg, ok := payload["uplink_message"].(map[string]interface{}); ok {
		uplinkMessage = msg
	} else {
		uplinkMessage = payload
	}

	// Try to find gateway metadata in multiple possible locations
	var rxMetadata []interface{}
	var ok bool

	if rxMetadata, ok = uplinkMessage["rx_metadata"].([]interface{}); !ok {
		if rxMetadata, ok = uplinkMessage["gateways"].([]interface{}); !ok {
			if rxMetadata, ok = uplinkMessage["gateway_info"].([]interface{}); !ok {
				if rxMetadata, ok = uplinkMessage["rxInfo"].([]interface{}); !ok {
					return "unknown"
				}
			}
		}
	}

	// Count gateways with valid location data
	gatewayCount := 0
	for _, gw := range rxMetadata {
		gateway, ok := gw.(map[string]interface{})
		if !ok {
			continue
		}

		if location, exists := gateway["location"]; exists && location != nil {
			gatewayCount++
		}
	}

	// Determine accuracy based on number of gateways
	switch gatewayCount {
	case 0:
		return "no-location"
	case 1:
		return "single-gateway"
	case 2:
		return "dual-gateway"
	case 3:
		return "triangulated"
	default:
		return "multi-gateway"
	}
}

// extractMetadata extracts useful metadata from the original payload
func (ts *TransformService) extractMetadata(payload map[string]interface{}) map[string]interface{} {
	metadata := make(map[string]interface{})

	// Add received timestamp if available
	if receivedAt, exists := payload["received_at"]; exists {
		metadata["received_at"] = receivedAt
	}

	// Extract uplink message metadata - handle both formats
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

		// Add gateway information - try multiple possible locations
		var rxMetadata []interface{}
		var metadataOk bool

		if rxMetadata, metadataOk = uplinkMessage["rx_metadata"].([]interface{}); !metadataOk {
			if rxMetadata, metadataOk = uplinkMessage["gateways"].([]interface{}); !metadataOk {
				if rxMetadata, metadataOk = uplinkMessage["gateway_info"].([]interface{}); !metadataOk {
					// Try LoRaWAN rxInfo format
					rxMetadata, metadataOk = uplinkMessage["rxInfo"].([]interface{})
				}
			}
		}

		if metadataOk {
			var gateways []map[string]interface{}
			for _, gw := range rxMetadata {
				if gateway, ok := gw.(map[string]interface{}); ok {
					gatewayInfo := make(map[string]interface{})

					// Add gateway ID if available
					if gatewayID, exists := gateway["gateway_ids"]; exists {
						gatewayInfo["gateway_id"] = gatewayID
					} else if gatewayID, exists := gateway["gatewayId"]; exists {
						gatewayInfo["gateway_id"] = gatewayID
					}

					// Add RSSI
					if rssi, exists := gateway["rssi"]; exists {
						gatewayInfo["rssi"] = rssi
					}

					// Add SNR if available
					if snr, exists := gateway["snr"]; exists {
						gatewayInfo["snr"] = snr
					}

					// Add location if available
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

		// Add frame counter if available
		if fCnt, exists := uplinkMessage["f_cnt"]; exists {
			metadata["frame_counter"] = fCnt
		}

		// Add port information if available
		if fPort, exists := uplinkMessage["f_port"]; exists {
			metadata["port"] = fPort
		}
	}

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

// extractDeviceID extracts device ID from device mappings first, then payload as fallback
func (ts *TransformService) extractDeviceID(payload map[string]interface{}, organization, devEUI string) string {
	// Priority 1: Look up device mapping by DevEUI for hardcoded device name
	if _, mapping, err := ts.deviceProfileService.GetDeviceProfile(organization, devEUI); err == nil {
		if mapping.DeviceID != "" {
			return mapping.DeviceID
		}
		if mapping.DeviceName != "" {
			return mapping.DeviceName
		}
	}
	
	// Priority 2: Try direct device_id field in payload as fallback
	if deviceID, exists := payload["device_id"]; exists {
		if strVal, ok := deviceID.(string); ok {
			return strVal
		}
	}
	
	// If no device_id found, return "unknown"
	return "unknown"
}
