package rakwireless

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"

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
		DeviceEUI:  payload.DeviceEUI,
		DeviceType: components.DeviceTypeRAK7200,
		Timestamp:  payload.Timestamp,
		SensorData: make(map[string]interface{}),
		RawData:    payload.Data,
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

// GetSupportedEntityTypes returns entity types supported by RAK7200
func (p *RAK7200Parser) GetSupportedEntityTypes() []string {
	return []string{"location", "battery", "temperature"} // RAK7200 provides GPS, battery, and temperature
}

// ParseToEntities creates entities for RAK7200 device
func (p *RAK7200Parser) ParseToEntities(orgSlug, model string, payload *components.RawPayload) ([]components.Entity, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI is required")
	}

	// Parse RAK7200 payload (simplified example)
	parsedValues, err := p.parseRAK7200Payload(payload.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RAK7200 payload: %v", err)
	}

	var entities []components.Entity
	timestamp := payload.Timestamp

	// 1. Location Entity (GPS-based)
	if parsedValues.HasGPS {
		locationEntity := components.Entity{
			UniqueID: components.GenerateUniqueID(orgSlug, devEUI, "location"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("location"),
				orgSlug, "rakwireless", "rak7200", devEUI, "location",
			),
			EntityType:  "location",
			DeviceClass: "location",
			Name:        "Location",
			State:       "home",
			Attributes: map[string]interface{}{
				"latitude":     parsedValues.Latitude,
				"longitude":    parsedValues.Longitude,
				"source":       "gps",
				"accuracy":     5.0, // GPS typically 5m accuracy
				"gps_capable":  true,
				"device_model": "RAK7200",
			},
			Enabled:   true,
			Timestamp: timestamp,
		}
		entities = append(entities, locationEntity)
	}

	// 2. Battery Entity
	if parsedValues.BatteryLevel > 0 {
		batteryEntity := components.Entity{
			UniqueID: components.GenerateUniqueID(orgSlug, devEUI, "battery"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("battery"),
				orgSlug, "rakwireless", "rak7200", devEUI, "battery",
			),
			EntityType:  "battery",
			DeviceClass: "battery",
			Name:        "Battery Level",
			State:       parsedValues.BatteryLevel,
			UnitOfMeas:  "%",
			Icon:        "mdi:battery",
			Enabled:     true,
			Timestamp:   timestamp,
		}
		entities = append(entities, batteryEntity)
	}

	// 3. Temperature Entity
	if parsedValues.Temperature != 0 {
		tempEntity := components.Entity{
			UniqueID: components.GenerateUniqueID(orgSlug, devEUI, "temperature"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("temperature"),
				orgSlug, "rakwireless", "rak7200", devEUI, "temperature",
			),
			EntityType:  "temperature",
			DeviceClass: "temperature",
			Name:        "Temperature",
			State:       parsedValues.Temperature,
			UnitOfMeas:  "°C",
			Icon:        "mdi:thermometer",
			Enabled:     true,
			Timestamp:   timestamp,
		}
		entities = append(entities, tempEntity)
	}

	return entities, nil
}

// RAK7200ParsedValues holds the parsed sensor values
type RAK7200ParsedValues struct {
	HasGPS       bool
	Latitude     float64
	Longitude    float64
	BatteryLevel float64
	Temperature  float64
}

// parseRAK7200Payload parses the base64 encoded payload
func (p *RAK7200Parser) parseRAK7200Payload(data string) (*RAK7200ParsedValues, error) {
	// Decode base64 payload
	decodedData, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 payload: %v", err)
	}

	if len(decodedData) < 11 { // Minimum payload size for GPS + battery
		return nil, fmt.Errorf("payload too short: %d bytes", len(decodedData))
	}

	result := &RAK7200ParsedValues{}

	// Example RAK7200 payload format (simplified):
	// Bytes 0-3: Latitude (int32)
	// Bytes 4-7: Longitude (int32)
	// Byte 8: Battery level (uint8)
	// Bytes 9-10: Temperature (int16)

	// Parse GPS coordinates
	latRawU := binary.LittleEndian.Uint32(decodedData[0:4])
	lonRawU := binary.LittleEndian.Uint32(decodedData[4:8])

	if latRawU > math.MaxInt32 || lonRawU > math.MaxInt32 {
		return nil, fmt.Errorf("gps coordinate out of int32 range")
	}

	latRaw := int32(latRawU)
	lonRaw := int32(lonRawU)

	if latRaw != 0 && lonRaw != 0 {
		result.HasGPS = true
		result.Latitude = float64(latRaw) / 1000000.0  // Convert from microdegrees
		result.Longitude = float64(lonRaw) / 1000000.0 // Convert from microdegrees
	}

	// Parse battery level
	if len(decodedData) > 8 {
		result.BatteryLevel = float64(decodedData[8])
	}

	// Parse temperature
	if len(decodedData) > 10 {
		tempRawU := binary.LittleEndian.Uint16(decodedData[9:11])
		if tempRawU > math.MaxInt16 {
			return nil, fmt.Errorf("temperature out of int16 range")
		}
		tempRaw := int16(tempRawU)
		result.Temperature = float64(tempRaw) / 100.0 // Convert from centidegrees
	}

	return result, nil
}
