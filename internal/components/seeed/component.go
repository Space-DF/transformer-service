package seeed

import (
	"context"
	"fmt"

	"github.com/Space-DF/transformer-service/internal/components"
)

// SeeedComponent implements the DeviceComponent interface for Seeed devices
type SeeedComponent struct {
	parser *T1000Parser
}

// NewSeeedComponent creates a new Seeed component
func NewSeeedComponent() *SeeedComponent {
	return &SeeedComponent{
		parser: NewT1000Parser(),
	}
}

// GetInfo returns component metadata
func (c *SeeedComponent) GetInfo() components.ComponentInfo {
	return components.ComponentInfo{
		Name:         "Seeed",
		Manufacturer: "Seeed Studio",
		Version:      "1.0.0",
		Description:  "Support for Seeed Studio SenseCAP LoRaWAN devices",
		DeviceTypes:  []components.DeviceType{components.DeviceTypeSenseCAP_T1000},
	}
}

// GetSupportedDevices returns the device types this component supports
func (c *SeeedComponent) GetSupportedDevices() []components.DeviceType {
	return []components.DeviceType{components.DeviceTypeSenseCAP_T1000}
}

// CanHandle checks if this component can handle the given device type and payload
func (c *SeeedComponent) CanHandle(deviceType components.DeviceType, payload *components.RawPayload) bool {
	return deviceType == components.DeviceTypeSenseCAP_T1000
}

// Parse converts raw payload into structured ParsedData
func (c *SeeedComponent) Parse(ctx context.Context, deviceType components.DeviceType, payload *components.RawPayload) (*components.ParsedData, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = components.ExtractDevEUI(payload.Metadata)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI not found")
	}

	// Decode payload bytes
	encoded := extractPayloadData(payload.Data)
	if encoded == "" {
		encoded = extractPayloadData(payload.Metadata)
	}
	if encoded == "" {
		return nil, fmt.Errorf("no payload data found")
	}

	bytes, err := decodePayloadBytes(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	// Parse T1000 packet
	t1000Data, err := parseT1000Packet(bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse T1000 packet: %w", err)
	}

	sensorData := make(map[string]interface{})
	var location *components.Location

	// Add battery level
	if t1000Data.BatteryLevel > 0 {
		sensorData["battery_percent"] = float64(t1000Data.BatteryLevel)
	}

	// Add temperature
	sensorData["temperature"] = t1000Data.Temperature

	// Add light level
	sensorData["light_percent"] = float64(t1000Data.Light)

	// Add work mode
	sensorData["work_mode"] = getWorkModeName(t1000Data.WorkMode)

	// Add positioning strategy
	sensorData["positioning_strategy"] = getPositioningStrategyName(t1000Data.PositionStrategy)

	// Add event status
	sensorData["event_status"] = t1000Data.EventStatus
	sensorData["start_moving"] = t1000Data.EventStatus&EventStartMoving != 0
	sensorData["end_moving"] = t1000Data.EventStatus&EventEndMoving != 0
	sensorData["motionless"] = t1000Data.EventStatus&EventMotionless != 0
	sensorData["shock"] = t1000Data.EventStatus&EventShock != 0
	sensorData["temperature_event"] = t1000Data.EventStatus&EventTemperature != 0
	sensorData["light_event"] = t1000Data.EventStatus&EventLight != 0
	sensorData["sos"] = t1000Data.EventStatus&EventSOS != 0

	// Add location if available
	if t1000Data.Latitude != 0 || t1000Data.Longitude != 0 {
		location = &components.Location{
			Latitude:  t1000Data.Latitude,
			Longitude: t1000Data.Longitude,
			Altitude:  t1000Data.Altitude,
		}
		sensorData["latitude"] = t1000Data.Latitude
		sensorData["longitude"] = t1000Data.Longitude
		sensorData["position_source"] = getPositioningSource(t1000Data.PositionStrategy)
	}

	// Add WiFi MACs if available
	if len(t1000Data.WiFiMACs) > 0 {
		sensorData["wifi_mac_addresses"] = t1000Data.WiFiMACs
	}

	// Add BLE MACs if available
	if len(t1000Data.BLEMACs) > 0 {
		sensorData["ble_mac_addresses"] = t1000Data.BLEMACs
	}

	var batteryLevel *float64
	if t1000Data.BatteryLevel > 0 {
		batt := float64(t1000Data.BatteryLevel)
		batteryLevel = &batt
	}

	return &components.ParsedData{
		DeviceEUI:    devEUI,
		DeviceType:   components.DeviceTypeSenseCAP_T1000,
		Timestamp:    payload.Timestamp,
		Location:     location,
		SensorData:   sensorData,
		BatteryLevel: batteryLevel,
		RawData:      encoded,
	}, nil
}

// ParseToEntities converts raw payload into multiple entities
func (c *SeeedComponent) ParseToEntities(ctx context.Context, orgSlug, model string, deviceType components.DeviceType, payload *components.RawPayload, deviceLocation *components.Location) (*components.ParseResult, error) {
	if deviceType != components.DeviceTypeSenseCAP_T1000 {
		return nil, fmt.Errorf("unsupported device type: %s", deviceType)
	}

	entities, err := c.parser.ParseToEntities(orgSlug, model, payload, deviceLocation)
	if err != nil {
		return nil, err
	}

	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = components.ExtractDevEUI(payload.Metadata)
	}

	return &components.ParseResult{
		DeviceEUI: devEUI,
		DeviceInfo: components.DeviceInfo{
			Identifiers:  []string{devEUI},
			Name:         "SenseCAP T1000 Tracker",
			Manufacturer: "Seeed Studio",
			Model:        "SenseCAP T1000",
			ModelID:      "sensecap_t1000",
		},
		Entities:  entities,
		Timestamp: payload.Timestamp,
	}, nil
}

// Validate performs device-specific validation on the parsed data
func (c *SeeedComponent) Validate(deviceType components.DeviceType, data *components.ParsedData) error {
	if deviceType != components.DeviceTypeSenseCAP_T1000 {
		return fmt.Errorf("unsupported device type: %s", deviceType)
	}

	// Validate battery level
	if data.BatteryLevel != nil && (*data.BatteryLevel < 0 || *data.BatteryLevel > 100) {
		return fmt.Errorf("battery level out of range: %.2f", *data.BatteryLevel)
	}

	// Validate location if present
	if data.Location != nil {
		if err := validateCoordinates(data.Location.Latitude, data.Location.Longitude); err != nil {
			return err
		}
	}

	return nil
}

// SupportsGPS returns true if the device has built-in GPS
func (c *SeeedComponent) SupportsGPS(deviceType components.DeviceType) bool {
	return deviceType == components.DeviceTypeSenseCAP_T1000
}

// GetSupportedPorts returns the fPorts this device type uses
func (c *SeeedComponent) GetSupportedPorts(deviceType components.DeviceType) []int {
	return []int{1, 5} // fPort 1 for uplink, fPort 5 for downlink
}

// GetSupportedEntityTypes returns the entity types this device supports
func (c *SeeedComponent) GetSupportedEntityTypes(deviceType components.DeviceType) []string {
	return []string{
		"location",
		"battery",
		"temperature",
		"light",
		"motion",
		"shock_event",
		"temperature_event",
		"light_event",
		"sos_alert",
		"work_mode",
		"positioning_strategy",
	}
}
