package rakwireless

import (
	"fmt"

	"github.com/Space-DF/transformer-service/internal/components"
)

// RAK7200Parser handles parsing of RAK7200 device payloads with GPS
type RAK7200Parser struct{}

// NewRAK7200Parser creates a new RAK7200 parser
func NewRAK7200Parser() *RAK7200Parser {
	return &RAK7200Parser{}
}

// ParsePayload parses RAK7200 device payload and extracts GPS coordinates
func (p *RAK7200Parser) ParsePayload(payload *components.RawPayload) (*components.ParsedData, error) {
	if payload.DeviceEUI == "" {
		return nil, fmt.Errorf("device EUI not found")
	}

	// RAK7200 GPS parsing logic would go here
	// This is a placeholder implementation
	parsedData := &components.ParsedData{
		DeviceEUI:    payload.DeviceEUI,
		DeviceType:   components.DeviceTypeRAK7200,
		Timestamp:    payload.Timestamp,
		SensorData:   make(map[string]interface{}),
		RawData:      payload.Data,
	}

	// TODO: Implement actual GPS parsing from base64 data
	// For now, return without location data
	return parsedData, fmt.Errorf("RAK7200 GPS parsing not yet implemented")
}

// SupportsGPS returns true since RAK7200 has built-in GPS
func (p *RAK7200Parser) SupportsGPS() bool {
	return true
}

// GetSupportedPorts returns the fPorts typically used by RAK7200
func (p *RAK7200Parser) GetSupportedPorts() []int {
	return []int{2, 3, 4, 5} // Common fPorts for RAK7200
}