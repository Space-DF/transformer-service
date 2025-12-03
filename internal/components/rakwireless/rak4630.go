package rakwireless

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/fxamacker/cbor/v2"

	"github.com/Space-DF/transformer-service/internal/components"
)

// SensorFrame represents the CBOR sensor frame structure from firmware
type SensorFrame struct {
	ID     int    `cbor:"id"`
	Type   int    `cbor:"type"`
	Fmt    string `cbor:"fmt"`
	Sensor string `cbor:"sensor"`
}

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
func (p *RAK4630Parser) ParseToEntities(orgSlug string, payload *components.RawPayload) ([]components.Entity, error) {
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
			UniqueID: components.GenerateUniqueID(orgSlug, devEUI, "location"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("location"),
				orgSlug, "rakwireless", "rak4630", devEUI, "location",
			),
			EntityType:  "location",
			DeviceClass: "location",
			Name:        "Location",
			State:       "home",
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
			UniqueID: components.GenerateUniqueID(orgSlug, devEUI, "battery"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("battery"),
				orgSlug, "rakwireless", "rak4630", devEUI, "battery",
			),
			EntityType:  "battery",
			DeviceClass: "battery",
			Name:        "Battery Level",
			State:       batteryV,
			UnitOfMeas:  "V",
			Icon:        "mdi:battery",
			Timestamp:   timestamp,
			Enabled:     true,
		}
		entities = append(entities, batteryEntity)
	}

	// Temperature Entity
	if temp, ok := readings["temperature"]; ok {
		tempEntity := components.Entity{
			UniqueID: components.GenerateUniqueID(orgSlug, devEUI, "temperature"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("temperature"),
				orgSlug, "rakwireless", "rak4630", devEUI, "temperature",
			),
			EntityType:  "temperature",
			DeviceClass: "temperature",
			Name:        "Temperature",
			State:       temp,
			UnitOfMeas:  "°C",
			Icon:        "mdi:thermometer",
			Timestamp:   timestamp,
			Enabled:     true,
		}
		entities = append(entities, tempEntity)
	}

	// Humidity Entity
	if humidity, ok := readings["humidity"]; ok {
		humidityEntity := components.Entity{
			UniqueID: components.GenerateUniqueID(orgSlug, devEUI, "humidity"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("humidity"),
				orgSlug, "rakwireless", "rak4630", devEUI, "humidity",
			),
			EntityType:  "humidity",
			DeviceClass: "humidity",
			Name:        "Humidity",
			State:       humidity,
			UnitOfMeas:  "%",
			Icon:        "mdi:water-percent",
			Timestamp:   timestamp,
			Enabled:     true,
		}
		entities = append(entities, humidityEntity)
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
	if len(values) < 16 {
		return nil, fmt.Errorf("insufficient sensor data values: %d", len(values))
	}

	latStr := strings.TrimSpace(values[14])
	lonStr := strings.TrimSpace(values[15])

	lat, err1 := strconv.ParseFloat(latStr, 64)
	lon, err2 := strconv.ParseFloat(lonStr, 64)
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("failed to parse GPS coordinates from sensor data")
	}

	if err := p.validateCoordinates(lat, lon); err != nil {
		return nil, fmt.Errorf("sensor data does not contain valid GPS coordinates: %w", err)
	}

	return &components.Location{
		Latitude:  lat,
		Longitude: lon,
	}, nil
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

// decodeSensorReadings decodes Cayenne LPP sensor values from base64 payload data
func (p *RAK4630Parser) decodeSensorReadings(payload *components.RawPayload) map[string]float64 {
	var encoded string

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

	if strings.TrimSpace(encoded) == "" {
		return nil
	}

	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil
	}

	// Try CBOR map with "sensor" field first (ChirpStack relay format)
	var m map[string]interface{}
	if err := cbor.Unmarshal(raw, &m); err == nil {
		if sensorStr, ok := m["sensor"].(string); ok {
			if readings := parseSensorString(sensorStr); len(readings) > 0 {
				return readings
			}
		}
	}

	// Try Cayenne LPP
	if readings := decodeCayenneLPP(raw); len(readings) > 0 {
		return readings
	}

	// Fallback: pull structured fields from metadata (uplinkEvent.object / decoded_payload)
	return decodeFromMetadata(payload.Metadata)
}

// decodeCayenneLPP decodes a subset of Cayenne LPP payload (temperature, humidity, analog/battery)
func decodeCayenneLPP(data []byte) map[string]float64 {
	readings := make(map[string]float64)

	for i := 0; i < len(data); {
		channel := data[i]
		i++
		if i >= len(data) {
			break
		}
		dataType := data[i]
		i++

		switch dataType {
		case 0x67: // Temperature (2 bytes, signed, 0.1°C)
			if i+2 > len(data) {
				return readings
			}
			raw := int16(data[i])<<8 | int16(data[i+1])
			i += 2
			readings["temperature"] = float64(raw) / 10.0
		case 0x68: // Humidity (1 byte, 0.5% steps)
			if i+1 > len(data) {
				return readings
			}
			raw := data[i]
			i++
			readings["humidity"] = float64(raw) / 2.0
		case 0x02, 0x03: // Analog input/output (2 bytes, 0.01 units)
			if i+2 > len(data) {
				return readings
			}
			raw := int16(data[i])<<8 | int16(data[i+1])
			i += 2
			value := float64(raw) / 100.0
			readings[fmt.Sprintf("analog_input_ch%d", channel)] = value
			if channel == 0 {
				readings["battery_v"] = value
			}
		default:
			// Unsupported type; stop decoding to avoid misalignment
			return readings
		}
	}

	if len(readings) == 0 {
		return nil
	}

	return readings
}

// parseSensorString parses vendor-specific comma-separated sensor payloads from CBOR
// The format contains many placeholder "*" values followed by numeric fields.
// We currently map the last two numeric fields to humidity and temperature as a best-effort extraction.
func parseSensorString(sensorStr string) map[string]float64 {
	parts := strings.Split(sensorStr, ",")
	values := make([]float64, 0, len(parts))
	for _, p := range parts {
		if v, err := strconv.ParseFloat(strings.TrimSpace(p), 64); err == nil {
			values = append(values, v)
		}
	}

	readings := make(map[string]float64)
	if n := len(values); n >= 2 {
		readings["humidity"] = values[n-2]
		readings["temperature"] = values[n-1]
	} else if n == 1 {
		readings["temperature"] = values[0]
	}

	if len(readings) == 0 {
		return nil
	}
	return readings
}

// decodeFromMetadata tries to extract sensor readings from structured payload metadata.
// It looks for common keys like "object" or "decoded_payload" containing numeric fields.
func decodeFromMetadata(metadata map[string]interface{}) map[string]float64 {
	if len(metadata) == 0 {
		return nil
	}

	readings := make(map[string]float64)

	// Helper to pull a float64 from a map by key
	extract := func(m map[string]interface{}, key string) (float64, bool) {
		if v, ok := m[key]; ok {
			switch t := v.(type) {
			case float64:
				return t, true
			case float32:
				return float64(t), true
			case int:
				return float64(t), true
			case int64:
				return float64(t), true
			case json.Number:
				if f, err := t.Float64(); err == nil {
					return f, true
				}
			}
		}
		return 0, false
	}

	// Try known containers in order of preference
	for _, key := range []string{"object", "decoded_payload", "decodedPayload"} {
		if obj, ok := metadata[key].(map[string]interface{}); ok {
			if v, ok := extract(obj, "temperature"); ok {
				readings["temperature"] = v
			}
			if v, ok := extract(obj, "humidity"); ok {
				readings["humidity"] = v
			}
			if v, ok := extract(obj, "battery"); ok {
				readings["battery_v"] = v
			}
			// If we found any, return
			if len(readings) > 0 {
				return readings
			}
		}
	}

	if len(readings) == 0 {
		return nil
	}
	return readings
}
