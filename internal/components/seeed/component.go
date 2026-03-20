package seeed

import (
	"context"
	"fmt"

	"github.com/Space-DF/transformer-service/internal/components"
)

// Seeed Studio devices
const (
	DeviceTypeSenseCAP_T1000 = "SENSECAP_T1000"
)

// SeeedComponent implements the DeviceComponent interface for Seeed devices
type SeeedComponent struct {
	parsers map[components.DeviceType]DeviceParser
}

// DeviceParser handles device-specific parsing logic
type DeviceParser interface {
	ParsePayload(payload *components.RawPayload) (*components.ParsedData, error)
	ParseToEntities(orgSlug, model string, payload *components.RawPayload, deviceLocation *components.Location) ([]components.Entity, error)
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
}

// NewSeeedComponent creates a new Seeed component
func NewSeeedComponent() *SeeedComponent {
	component := &SeeedComponent{
		parsers: make(map[components.DeviceType]DeviceParser),
	}

	// Register device-specific parsers
	component.parsers[DeviceTypeSenseCAP_T1000] = NewT1000Parser()
	return component
}

// GetInfo returns component metadata
func (c *SeeedComponent) GetInfo() components.ComponentInfo {
	return components.ComponentInfo{
		Name:         "Seeed",
		Manufacturer: "Seeed Studio",
		Version:      "1.0.0",
		Description:  "Support for Seeed Studio SenseCAP LoRaWAN devices",
		DeviceTypes:  []components.DeviceType{DeviceTypeSenseCAP_T1000},
	}
}

// GetSupportedDevices returns the device types this component supports
func (c *SeeedComponent) GetSupportedDevices() []components.DeviceType {
	return []components.DeviceType{DeviceTypeSenseCAP_T1000}
}

// CanHandle checks if this component can handle the given device type and payload
func (c *SeeedComponent) CanHandle(deviceType components.DeviceType, payload *components.RawPayload) bool {
	parser, exists := c.parsers[deviceType]
	return exists && parser != nil
}

// Parse converts raw payload into structured ParsedData (DEPRECATED: Use ParseToEntities)
func (c *SeeedComponent) Parse(ctx context.Context, deviceType components.DeviceType, payload *components.RawPayload) (*components.ParsedData, error) {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return nil, fmt.Errorf("no parser found for device type %s", deviceType)
	}

	return parser.ParsePayload(payload)
}

// ParseToEntities converts raw payload into multiple entities
func (c *SeeedComponent) ParseToEntities(ctx context.Context, orgSlug, model string, deviceType components.DeviceType, payload *components.RawPayload, deviceLocation *components.Location) (*components.ParseResult, error) {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return nil, fmt.Errorf("no parser found for device type %s", deviceType)
	}

	entities, err := parser.ParseToEntities(orgSlug, model, payload, deviceLocation)
	if err != nil {
		return nil, err
	}

	// Create device info
	deviceInfo := components.CreateDeviceInfo(
		payload.DeviceEUI,
		fmt.Sprintf("%s", string(deviceType)),
		"Seeed",
		string(deviceType),
		string(deviceType),
	)

	return &components.ParseResult{
		DeviceEUI:  payload.DeviceEUI,
		DeviceInfo: deviceInfo,
		Entities:   entities,
		Timestamp:  payload.Timestamp,
	}, nil
}

// Validate performs device-specific validation on the parsed data
func (c *SeeedComponent) Validate(deviceType components.DeviceType, data *components.ParsedData) error {
	if deviceType != DeviceTypeSenseCAP_T1000 {
		return fmt.Errorf("unsupported device type: %s", deviceType)
	}

	// Validate battery level
	if data.BatteryLevel != nil && (*data.BatteryLevel < 0 || *data.BatteryLevel > 100) {
		return fmt.Errorf("battery level out of range: %.2f", *data.BatteryLevel)
	}

	// Validate location if present
	if data.Location != nil {
		if err := components.ValidateCoordinates(data.Location.Latitude, data.Location.Longitude); err != nil {
			return err
		}
	}

	return nil
}

// SupportsGPS returns true if the device has built-in GPS
func (c *SeeedComponent) SupportsGPS(deviceType components.DeviceType) bool {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return false
	}
	return parser.SupportsGPS()
}

// GetSupportedPorts returns the fPorts this device type uses
func (c *SeeedComponent) GetSupportedPorts(deviceType components.DeviceType) []int {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return nil
	}
	return parser.GetSupportedPorts()
}

// GetSupportedEntityTypes returns the entity types this device supports
func (c *SeeedComponent) GetSupportedEntityTypes(deviceType components.DeviceType) []string {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return nil
	}
	return parser.GetSupportedEntityTypes()
}
