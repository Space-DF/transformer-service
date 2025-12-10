package rakwireless

import (
	"fmt"

	"github.com/Space-DF/transformer-service/internal/components"
)

// RAK2270Parser handles parsing of RAK2270 device payloads
// RAK2270 doesn't have GPS, requires trilateration calculation
type RAK2270Parser struct{}

// NewRAK2270Parser creates a new RAK2270 parser
func NewRAK2270Parser() *RAK2270Parser {
	return &RAK2270Parser{}
}

// ParsePayload parses RAK2270 device payload
// Since RAK2270 doesn't have GPS, this returns an error to indicate location calculation is needed
func (p *RAK2270Parser) ParsePayload(payload *components.RawPayload) (*components.ParsedData, error) {
	// RAK2270 doesn't have GPS coordinates in payload
	// Location must be calculated using trilateration from gateway RSSI data
	return nil, fmt.Errorf("RAK2270 requires trilateration calculation, no GPS data in payload")
}

// SupportsGPS returns false since RAK2270 doesn't have built-in GPS
func (p *RAK2270Parser) SupportsGPS() bool {
	return false
}

// GetSupportedPorts returns the fPorts typically used by RAK2270
func (p *RAK2270Parser) GetSupportedPorts() []int {
	return []int{1, 2, 3} // Common fPorts for RAK2270
}

// GetSupportedEntityTypes returns entity types supported by RAK2270
func (p *RAK2270Parser) GetSupportedEntityTypes() []string {
	return []string{"location"} // RAK2270 only provides location via trilateration
}

// ParseToEntities creates entities for RAK2270 device
func (p *RAK2270Parser) ParseToEntities(orgSlug, model string, payload *components.RawPayload) ([]components.Entity, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI is required")
	}

	// Generate entity IDs using standard format
	uniqueID := components.GenerateUniqueID(model, devEUI, "location")
	entityID := components.GenerateEntityID(
		components.GetEntityDomain("location"), // "device_tracker"
		orgSlug,
		"rakwireless",
		"rak2270",
		devEUI,
		"location",
	)

	// Create location entity
	// Note: RAK2270 doesn't have GPS, so state will be set by location calculation service
	locationEntity := components.Entity{
		UniqueID:    uniqueID,
		EntityID:    entityID,
		EntityType:  "location",
		DeviceClass: "location",
		Name:        "Location",
		State:       "unknown", // Will be updated by trilateration service
		DisplayType: []string{"map"},
		Attributes: map[string]interface{}{
			"source":               "trilateration",
			"requires_calculation": true,
			"gps_capable":          false,
			"device_model":         "RAK2270",
		},
		Enabled:   true,
		Timestamp: payload.Timestamp,
	}

	return []components.Entity{locationEntity}, nil
}
