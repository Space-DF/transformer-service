package rakwireless

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fxamacker/cbor/v2"

	"github.com/Space-DF/transformer-service/internal/components"
	"github.com/Space-DF/transformer-service/internal/lns"
)

// RAK4630Parser handles parsing of RAK4630 device payloads
type RAK4630Parser struct{}

// NewRAK4630Parser creates a new RAK4630 parser
func NewRAK4630Parser() *RAK4630Parser {
	return &RAK4630Parser{}
}

// ParsePayload parses RAK4630 device payload and extracts GPS coordinates and sensor data
func (p *RAK4630Parser) ParsePayload(payload *components.RawPayload) (*components.ParsedData, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = components.ExtractDevEUI(payload.Metadata, payload.LNSType)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI not found")
	}

	// LNS type must be known
	if payload.LNSType == "" || payload.LNSType == lns.LNSTypeUnknown {
		return nil, fmt.Errorf("LNS type is required for RAK4630 parsing")
	}

	// Get LNS handler
	lnsHandler, err := lns.GetLNSHandler(payload.LNSType)
	if err != nil {
		return nil, fmt.Errorf("no handler registered for LNS type %s: %w", payload.LNSType, err)
	}

	// Extract raw payload bytes
	payloadBytes, err := lnsHandler.ExtractPayloadBytes(payload.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to extract payload bytes: %w", err)
	}

	// Parse all sensor data from CBOR (includes GPS, temperature, humidity, pressure, battery, etc.)
	readings := p.decodeSensorReadingsFromBytes(payloadBytes)
	if readings == nil {
		return nil, fmt.Errorf("failed to decode RAK4630 sensor data: invalid or missing CBOR data")
	}

	// Extract location from readings
	var location *components.Location
	if lat, latOk := readings["latitude"]; latOk {
		if lon, lonOk := readings["longitude"]; lonOk && components.ValidateCoordinates(lat, lon) == nil {
			location = &components.Location{Latitude: lat, Longitude: lon}
		}
	}

	// Build sensor data map
	sensorData := make(map[string]interface{})
	for k, v := range readings {
		if k != "latitude" && k != "longitude" && k != "altitude" {
			sensorData[k] = v
		}
	}

	return &components.ParsedData{
		DeviceEUI:  devEUI,
		DeviceType: DeviceTypeRAK4630,
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
	return []string{"location", "battery_voltage", "temperature", "humidity", "pressure"} // RAK4630 environmental sensor
}

// ParseToEntities creates entities for RAK4630 device
func (p *RAK4630Parser) ParseToEntities(orgSlug, model string, payload *components.RawPayload, deviceLocation *components.Location) ([]components.Entity, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = components.ExtractDevEUI(payload.Metadata, payload.LNSType)
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

	// Battery Voltage Entity
	if batteryV, ok := readings["battery_v"]; ok {
		batteryEntity := components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "battery_voltage"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("battery_voltage"),
				orgSlug, "rakwireless", "rak4630", devEUI, "battery_voltage",
			),
			EntityType:  "battery_voltage",
			DeviceClass: "battery_voltage",
			Name:        "Battery Voltage",
			State:       batteryV,
			DisplayType: []string{"chart", "gauge", "value", "slider"},
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
			DisplayType: []string{"chart", "gauge", "value"},
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
			DisplayType: []string{"chart", "gauge", "value"},
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

// parseRAK4630SensorString parses RAK4630-specific comma-separated sensor payloads from CBOR.
// Expected format: temperature,humidity,pressure,*,*,*,*,*,*,*,*,*,*,*,*,latitude,longitude,altitude,*,*,*,*,*,*,*,battery_v
func parseRAK4630SensorString(sensorStr string) map[string]float64 {
	parts := strings.Split(sensorStr, ",")

	readings := make(map[string]float64)
	get := func(idx int) (float64, bool) {
		if idx < 0 || idx >= len(parts) {
			return 0, false
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(parts[idx]), 64)
		if err != nil {
			return 0, false
		}
		return v, true
	}

	// Parse based on field positions (RAK4630 standard format)
	if v, ok := get(0); ok {
		readings["temperature"] = v
	}
	if v, ok := get(1); ok {
		readings["humidity"] = v
	}
	if v, ok := get(2); ok {
		readings["pressure"] = v
	}
	// Index 3-15 are placeholders (*)
	if v, ok := get(16); ok {
		readings["latitude"] = v
	}
	if v, ok := get(17); ok {
		readings["longitude"] = v
	}
	if v, ok := get(18); ok {
		readings["altitude"] = v
	}
	if v, ok := get(19); ok {
		readings["snr_or_altitude"] = v
	}
	if v, ok := get(20); ok {
		readings["raw_signal"] = v
	}
	if v, ok := get(24); ok {
		readings["battery_v"] = v
	}

	if len(readings) == 0 {
		return nil
	}
	return readings
}

// decodeSensorReadings decodes sensor values from base64 CBOR payload data
func (p *RAK4630Parser) decodeSensorReadings(payload *components.RawPayload) map[string]float64 {
	if payload.LNSType == "" || payload.LNSType == lns.LNSTypeUnknown {
		return nil
	}

	lnsHandler, err := lns.GetLNSHandler(payload.LNSType)
	if err != nil {
		return nil
	}

	payloadBytes, err := lnsHandler.ExtractPayloadBytes(payload.Metadata)
	if err != nil {
		return nil
	}
	return p.decodeSensorReadingsFromBytes(payloadBytes)
}

// decodeSensorReadingsFromBytes decodes sensor values from raw CBOR payload bytes
func (p *RAK4630Parser) decodeSensorReadingsFromBytes(payloadBytes []byte) map[string]float64 {
	if len(payloadBytes) == 0 {
		return nil
	}

	// Check for RAK WisToolBox format marker ("v2" at the start)
	cborData := payloadBytes
	if len(payloadBytes) > 2 && payloadBytes[0] == 'v' && payloadBytes[1] == '2' {
		cborData = payloadBytes[2:]
	}

	var m map[string]interface{}
	if err := cbor.Unmarshal(cborData, &m); err != nil {
		return nil
	}

	// Extract sensor string from CBOR
	sensorStr, ok := m["sensor"].(string)
	if !ok {
		return nil
	}

	return parseRAK4630SensorString(sensorStr)
}
