package rakwireless

import (
	"encoding/base64"
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
func (p *RAK2270Parser) ParsePayload(payload *components.RawPayload) (*components.ParsedData, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = components.ExtractDevEUI(payload.Metadata)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI not found")
	}

	sensorData, err := decodeRAK2270Payload(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode sensor readings: %w", err)
	}

	return &components.ParsedData{
		DeviceEUI:  devEUI,
		DeviceType: components.DeviceTypeRAK2270,
		Timestamp:  payload.Timestamp,
		SensorData: sensorData,
		RawData:    payload.Data,
	}, nil
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
	return []string{"location", "battery", "temperature"}
}

// ParseToEntities creates entities for RAK2270 device
func (p *RAK2270Parser) ParseToEntities(orgSlug, model string, payload *components.RawPayload, deviceLocation *components.Location) ([]components.Entity, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = components.ExtractDevEUI(payload.Metadata)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI is required")
	}

	sensorData, err := decodeRAK2270Payload(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode sensor readings: %w", err)
	}

	var entities []components.Entity
	timestamp := payload.Timestamp

	// Location Entity - populated with trilateration result if available
	locationState := "unknown"
	locationAttributes := map[string]interface{}{
		"source":               "trilateration",
		"requires_calculation": true,
		"gps_capable":          false,
		"device_model":         "RAK2270",
	}

	if deviceLocation != nil {
		locationState = fmt.Sprintf("%f,%f", deviceLocation.Latitude, deviceLocation.Longitude)
		locationAttributes["latitude"] = deviceLocation.Latitude
		locationAttributes["longitude"] = deviceLocation.Longitude
	}

	locationEntity := components.Entity{
		UniqueID: components.GenerateUniqueID(model, devEUI, "location"),
		EntityID: components.GenerateEntityID(
			components.GetEntityDomain("location"),
			orgSlug, "rakwireless", "rak2270", devEUI, "location",
		),
		EntityType:  "location",
		DeviceClass: "location",
		Name:        "Location",
		State:       locationState,
		DisplayType: []string{"map"},
		Attributes:  locationAttributes,
		Enabled:     true,
		Timestamp:   timestamp,
	}
	entities = append(entities, locationEntity)

	// Temperature Entity
	if temp, ok := sensorData["temperature"].(float64); ok {
		tempEntity := components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "temperature"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("temperature"),
				orgSlug, "rakwireless", "rak2270", devEUI, "temperature",
			),
			EntityType:  "temperature",
			DeviceClass: "temperature",
			Name:        "Temperature",
			DisplayType: []string{"chart", "gauge", "value"},
			State:       temp,
			UnitOfMeas:  "°C",
			Enabled:     true,
			Timestamp:   timestamp,
		}
		entities = append(entities, tempEntity)
	}

	// Battery Voltage Entity
	if voltage, ok := sensorData["battery_voltage"].(float64); ok {
		batteryEntity := components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "battery"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("battery"),
				orgSlug, "rakwireless", "rak2270", devEUI, "battery",
			),
			EntityType:  "battery",
			DeviceClass: "battery",
			Name:        "Battery",
			DisplayType: []string{"chart", "gauge", "value"},
			State:       voltage,
			UnitOfMeas:  "V",
			Enabled:     true,
			Timestamp:   timestamp,
		}
		entities = append(entities, batteryEntity)
	}

	return entities, nil
}

// decodeRAK2270Bytes decodes RAK2270 sensor data from raw bytes
// RAK2270 payload format:
// - Channel (1 byte) + Type (1 byte) + Value (2 bytes, big-endian)
// - Type 0x67: Temperature (value / 10)
// - Type 0x02: Analog Input / Battery Voltage (value / 100)
func decodeRAK2270Bytes(bytes []byte) (map[string]interface{}, error) {
	if len(bytes) < 4 {
		return nil, fmt.Errorf("payload too short: %d bytes", len(bytes))
	}

	data := make(map[string]interface{})

	// Parse channel-type-value triplets
	i := 0
	for i+3 < len(bytes) {
		_ = bytes[i]
		typeByte := bytes[i+1]

		// Temperature (type 0x67)
		if typeByte == 0x67 {
			tempRaw := int(bytes[i+2])<<8 | int(bytes[i+3])
			temperature := float64(tempRaw) / 10.0
			data["temperature"] = temperature
			i += 4
		} else if typeByte == 0x02 {
			// Analog Input / Battery Voltage (type 0x02)
			voltageRaw := int(bytes[i+2])<<8 | int(bytes[i+3])
			voltage := float64(voltageRaw) / 100.0
			data["battery_voltage"] = voltage
			i += 4
		} else {
			break
		}
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("no valid sensor data found in payload")
	}

	return data, nil
}

// decodeRAK2270Payload extracts and decodes RAK2270 sensor data from payload
func decodeRAK2270Payload(payload *components.RawPayload) (map[string]interface{}, error) {
	var encoded string

	// Try to get data from RawPayload first
	if payload.Data != "" {
		encoded = payload.Data
	} else {
		// Try common metadata keys
		for _, key := range []string{"data", "frm_payload", "frmPayload"} {
			if val, ok := payload.Metadata[key].(string); ok && val != "" {
				encoded = val
				break
			}
		}
	}

	if encoded == "" {
		return nil, fmt.Errorf("no payload data found")
	}

	// Decode base64 payload
	bytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	data, err := decodeRAK2270Bytes(bytes)
	if err != nil {
		return nil, err
	}

	return data, nil
}
