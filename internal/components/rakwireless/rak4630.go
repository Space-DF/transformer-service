package rakwireless

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/fxamacker/cbor/v2"

	"github.com/Space-DF/transformer-service/internal/components"
)

// RAK4630Parser handles parsing of RAK4630 device payloads
type RAK4630Parser struct{}

// NewRAK4630Parser creates a new RAK4630 parser
func NewRAK4630Parser() *RAK4630Parser {
	return &RAK4630Parser{}
}

// ParsePayload parses RAK4630 device payload and extracts GPS coordinates
func (p *RAK4630Parser) ParsePayload(payload *components.RawPayload) (*components.ParsedData, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = extractDevEUI(payload.Metadata)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI not found")
	}

	// Try multiple parsing strategies
	location, err := p.parseFromCBORData(payload.Data, payload.Metadata)
	if err != nil {
		location, err = p.parseFromFrmPayload(payload.Metadata)
	}
	if err != nil {
		location, err = p.parseFromDecodedPayload(payload.Metadata)
	}
	if err != nil {
		return nil, fmt.Errorf("RAK4630 parsing not yet implemented: %w", err)
	}

	sensorData := make(map[string]interface{})
	if readings := p.decodeSensorReadings(payload); len(readings) > 0 {
		for k, v := range readings {
			sensorData[k] = v
		}
	}

	return &components.ParsedData{
		DeviceEUI:  devEUI,
		DeviceType: components.DeviceTypeRAK4630,
		Timestamp:  payload.Timestamp,
		Location:   location,
		SensorData: sensorData,
		RawData:    payload.Data,
	}, nil
}

// SupportsGPS returns true since RAK4630 can have GPS capability depending on configuration
func (p *RAK4630Parser) SupportsGPS() bool {
	return true // RAK4630 can support GPS
}

// GetSupportedPorts returns the fPorts typically used by RAK4630
func (p *RAK4630Parser) GetSupportedPorts() []int {
	return []int{1, 2, 3, 4, 5} // RAK4630 supports multiple fPorts
}

// GetSupportedEntityTypes returns entity types supported by RAK4630
func (p *RAK4630Parser) GetSupportedEntityTypes() []string {
	return []string{"location", "battery", "temperature", "humidity", "pressure"} // RAK4630 environmental sensor
}

// ParseToEntities creates entities for RAK4630 device
func (p *RAK4630Parser) ParseToEntities(orgSlug, model string, payload *components.RawPayload) ([]components.Entity, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = extractDevEUI(payload.Metadata)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI is required")
	}

	parsedData, err := p.ParsePayload(payload)
	if err != nil {
		return nil, err
	}

	readings := p.decodeSensorReadings(payload)
	var entities []components.Entity
	timestamp := payload.Timestamp

	// Location Entity (GPS-capable)
	if parsedData.Location != nil {
		locationEntity := components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "location"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("location"),
				orgSlug, "rakwireless", "rak4630", devEUI, "location",
			),
			EntityType:  "location",
			DeviceClass: "location",
			Name:        "Location",
			State:       "home",
			DisplayType: []string{"map"},
			Attributes: map[string]interface{}{
				"source":       "gps",
				"gps_capable":  true,
				"device_model": "RAK4630",
				"latitude":     parsedData.Location.Latitude,
				"longitude":    parsedData.Location.Longitude,
			},
			Enabled:   true,
			Timestamp: timestamp,
		}
		entities = append(entities, locationEntity)
	}

	// Battery Entity
	if batteryV, ok := readings["battery_v"]; ok {
		batteryEntity := components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "battery"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("battery"),
				orgSlug, "rakwireless", "rak4630", devEUI, "battery",
			),
			EntityType:  "battery",
			DeviceClass: "battery",
			Name:        "Battery Level",
			State:       batteryV,
			DisplayType: []string{"chart"},
			UnitOfMeas:  "V",
			Timestamp:   timestamp,
			Enabled:     true,
		}
		entities = append(entities, batteryEntity)
	}

	// Temperature Entity
	if temp, ok := readings["temperature"]; ok {
		tempEntity := components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "temperature"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("temperature"),
				orgSlug, "rakwireless", "rak4630", devEUI, "temperature",
			),
			EntityType:  "temperature",
			DeviceClass: "temperature",
			DisplayType: []string{"chart"},
			Name:        "Temperature",
			State:       temp,
			UnitOfMeas:  "°C",
			Timestamp:   timestamp,
			Enabled:     true,
		}
		entities = append(entities, tempEntity)
	}

	// Humidity Entity
	if humidity, ok := readings["humidity"]; ok {
		humidityEntity := components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "humidity"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("humidity"),
				orgSlug, "rakwireless", "rak4630", devEUI, "humidity",
			),
			EntityType:  "humidity",
			DeviceClass: "humidity",
			Name:        "Humidity",
			DisplayType: []string{"chart"},
			State:       humidity,
			UnitOfMeas:  "%",
			Timestamp:   timestamp,
			Enabled:     true,
		}
		entities = append(entities, humidityEntity)
	}

	// Pressure Entity
	if pressure, ok := readings["pressure"]; ok {
		pressureEntity := components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "pressure"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("pressure"),
				orgSlug, "rakwireless", "rak4630", devEUI, "pressure",
			),
			EntityType:  "pressure",
			DeviceClass: "pressure",
			DisplayType: []string{"chart"},
			Name:        "Pressure",
			State:       pressure,
			UnitOfMeas:  "kPa",
			Timestamp:   timestamp,
			Enabled:     true,
		}
		entities = append(entities, pressureEntity)
	}

	return entities, nil
}

// parseFromCBORData parses CBOR-encoded sensor data from the payload data/metadata
func (p *RAK4630Parser) parseFromCBORData(data string, metadata map[string]interface{}) (*components.Location, error) {
	dataStr := data
	if dataStr == "" {
		if decoded, ok := metadata["decoded_raw_data"].(map[string]interface{}); ok {
			dataStr, _ = decoded["data"].(string)
		}
		if dataStr == "" {
			if v, ok := metadata["data"].(string); ok {
				dataStr = v
			}
		}
	}

	if dataStr == "" {
		return nil, fmt.Errorf("data field not found for CBOR parsing")
	}

	cborData, err := base64.StdEncoding.DecodeString(dataStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 data: %w", err)
	}

	var frame SensorFrame
	if err := cbor.Unmarshal(cborData, &frame); err != nil {
		return nil, fmt.Errorf("failed to parse CBOR data: %w", err)
	}

	location, err := p.parseSensorData(frame)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sensor data: %w", err)
	}

	return location, nil
}

// parseSensorData extracts GPS coordinates from sensor frame data
func (p *RAK4630Parser) parseSensorData(frame SensorFrame) (*components.Location, error) {
	sensorData := frame.Sensor
	if sensorData == "" {
		return nil, fmt.Errorf("sensor data is empty")
	}

	// Remove optional prefix (e.g., "xs")
	sensorData = strings.TrimPrefix(sensorData, "xs")

	values := strings.Split(sensorData, ",")
	indices := [][2]int{
		{16, 17}, // Newer firmware positions
		{14, 15}, // Backward-compatible ff
	}

	for _, pair := range indices {
		if len(values) <= pair[1] {
			continue
		}

		latStr := strings.TrimSpace(values[pair[0]])
		lonStr := strings.TrimSpace(values[pair[1]])

		lat, err1 := strconv.ParseFloat(latStr, 64)
		lon, err2 := strconv.ParseFloat(lonStr, 64)
		if err1 != nil || err2 != nil {
			continue
		}

		if err := p.validateCoordinates(lat, lon); err != nil {
			continue
		}

		return &components.Location{
			Latitude:  lat,
			Longitude: lon,
		}, nil
	}

	return nil, fmt.Errorf("sensor data does not contain valid GPS coordinates")
}

// parseFromFrmPayload extracts GPS coordinates from hex frm_payload
func (p *RAK4630Parser) parseFromFrmPayload(metadata map[string]interface{}) (*components.Location, error) {
	var frmPayload string

	if uplink, ok := metadata["uplink_message"].(map[string]interface{}); ok {
		frmPayload, _ = uplink["frm_payload"].(string)
	}
	if frmPayload == "" {
		frmPayload, _ = metadata["frm_payload"].(string)
	}

	if frmPayload == "" {
		return nil, fmt.Errorf("frm_payload not found")
	}

	payloadBytes, err := hex.DecodeString(frmPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode hex payload: %w", err)
	}

	if len(payloadBytes) < 8 {
		return nil, fmt.Errorf("payload too short for GPS data: %d bytes", len(payloadBytes))
	}

	lat, lon, err := p.parseGPSCoordinates(payloadBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GPS coordinates: %w", err)
	}

	return &components.Location{
		Latitude:  lat,
		Longitude: lon,
	}, nil
}

// parseFromDecodedPayload extracts GPS coordinates from already decoded payload
func (p *RAK4630Parser) parseFromDecodedPayload(metadata map[string]interface{}) (*components.Location, error) {
	var decoded map[string]interface{}
	var ok bool

	if decoded, ok = metadata["decoded_payload"].(map[string]interface{}); !ok {
		if decoded, ok = metadata["decoded_raw_data"].(map[string]interface{}); !ok {
			return nil, fmt.Errorf("no decoded payload data found")
		}
	}

	var lat, lon float64
	var found bool

	if v, ok := decoded["latitude"].(float64); ok {
		if w, ok := decoded["longitude"].(float64); ok {
			lat, lon = v, w
			found = true
		}
	}
	if !found {
		if v, ok := decoded["lat"].(float64); ok {
			if w, ok := decoded["lng"].(float64); ok {
				lat, lon = v, w
				found = true
			}
		}
	}
	if !found {
		if gps, ok := decoded["gps"].(map[string]interface{}); ok {
			if v, ok := gps["latitude"].(float64); ok {
				if w, ok := gps["longitude"].(float64); ok {
					lat, lon = v, w
					found = true
				}
			}
		}
	}

	if !found {
		return nil, fmt.Errorf("GPS coordinates not found in decoded payload")
	}

	if err := p.validateCoordinates(lat, lon); err != nil {
		return nil, err
	}

	return &components.Location{
		Latitude:  lat,
		Longitude: lon,
	}, nil
}

// parseGPSCoordinates extracts GPS coordinates from RAK4630 payload bytes
func (p *RAK4630Parser) parseGPSCoordinates(payloadBytes []byte) (float64, float64, error) {
	if len(payloadBytes) < 8 {
		return 0, 0, fmt.Errorf("insufficient data for GPS coordinates")
	}

	latInt := int32(payloadBytes[0]) | int32(payloadBytes[1])<<8 | int32(payloadBytes[2])<<16 | int32(payloadBytes[3])<<24
	lonInt := int32(payloadBytes[4]) | int32(payloadBytes[5])<<8 | int32(payloadBytes[6])<<16 | int32(payloadBytes[7])<<24

	lat := float64(latInt) / 10000000.0
	lon := float64(lonInt) / 10000000.0

	if err := p.validateCoordinates(lat, lon); err != nil {
		return 0, 0, err
	}

	return lat, lon, nil
}

// validateCoordinates validates GPS coordinates
func (p *RAK4630Parser) validateCoordinates(latitude, longitude float64) error {
	if latitude == 0.0 && longitude == 0.0 {
		return fmt.Errorf("GPS coordinates are 0,0 - no GPS fix available")
	}
	if latitude < -90 || latitude > 90 {
		return fmt.Errorf("invalid latitude: %f", latitude)
	}
	if longitude < -180 || longitude > 180 {
		return fmt.Errorf("invalid longitude: %f", longitude)
	}
	return nil
}

// decodeSensorReadings decodes sensor values from base64 CBOR payload data
func (p *RAK4630Parser) decodeSensorReadings(payload *components.RawPayload) map[string]float64 {
	var encoded string

	// Get the base64 data from payload
	if payload.Data != "" {
		encoded = payload.Data
	} else {
		// Try common metadata keys
		for _, key := range []string{"data", "frm_payload", "frmPayload"} {
			if val, ok := payload.Metadata[key].(string); ok && strings.TrimSpace(val) != "" {
				encoded = val
				break
			}
		}
	}

	// Decode base64 CBOR data
	if strings.TrimSpace(encoded) != "" {
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil
		}

		var m map[string]interface{}
		if err := cbor.Unmarshal(raw, &m); err != nil {
			return nil
		}

		// Extract sensor string from CBOR
		if sensorStr, ok := m["sensor"].(string); ok {
			if readings := parseSensorString(sensorStr); len(readings) > 0 {
				return readings
			}
		}
	}

	return nil
}
