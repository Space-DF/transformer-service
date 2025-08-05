package parsers

import (
	"fmt"

	"github.com/Space-DF/transformer-service-go/internal/models"
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
func (p *RAK2270Parser) ParsePayload(payload map[string]interface{}) (*models.DeviceLocationData, error) {
	// RAK2270 doesn't have GPS coordinates in payload
	// Location must be calculated using trilateration from gateway RSSI data
	return nil, fmt.Errorf("RAK2270 requires trilateration calculation, no GPS data in payload")
}

// SupportsGPS returns false since RAK2270 doesn't have built-in GPS
func (p *RAK2270Parser) SupportsGPS() bool {
	return false
}
