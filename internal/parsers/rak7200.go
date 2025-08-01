package parsers

import (
	"fmt"

	"github.com/Space-DF/transformer-service-go/internal/models"
)

// RAK7200Parser handles parsing of RAK7200 device payloads with GPS
type RAK7200Parser struct{}

// NewRAK7200Parser creates a new RAK7200 parser
func NewRAK7200Parser() *RAK7200Parser {
	return &RAK7200Parser{}
}

// ParsePayload parses RAK7200 device payload and extracts GPS coordinates
func (p *RAK7200Parser) ParsePayload(payload map[string]interface{}) (*models.DeviceLocationData, error) {
	// Extract device EUI first
	devEUI := p.extractDevEUI(payload)
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI not found")
	}

	// RAK7200 GPS parsing logic would go here
	// This is a placeholder implementation
	return nil, fmt.Errorf("RAK7200 GPS parsing not yet implemented")
}

// SupportsGPS returns true since RAK7200 has built-in GPS
func (p *RAK7200Parser) SupportsGPS() bool {
	return true
}

// extractDevEUI extracts device EUI from various possible locations in payload
func (p *RAK7200Parser) extractDevEUI(payload map[string]interface{}) string {
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