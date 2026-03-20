package digitalmatter

import (
	"context"
	"fmt"

	"github.com/Space-DF/transformer-service/internal/components"
)

// Digital Matter device types
const (
	DeviceTypeYabbyEdge components.DeviceType = "YABBY_EDGE"
)

// DigitalMatterComponent implements the DeviceComponent interface for Digital Matter devices.
type DigitalMatterComponent struct {
	parsers map[components.DeviceType]DeviceParser
}

// DeviceParser handles device-specific parsing logic.
type DeviceParser interface {
	ParsePayload(payload *components.RawPayload) (*components.ParsedData, error)
	ParseToEntities(orgSlug, model string, payload *components.RawPayload, deviceLocation *components.Location) ([]components.Entity, error)
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
}

// NewDigitalMatterComponent creates a new Digital Matter component.
func NewDigitalMatterComponent() *DigitalMatterComponent {
	component := &DigitalMatterComponent{
		parsers: make(map[components.DeviceType]DeviceParser),
	}

	// Register device-specific parsers
	component.parsers[DeviceTypeYabbyEdge] = NewYabbyEdgeParser()
	return component
}

// GetInfo returns component metadata.
func (c *DigitalMatterComponent) GetInfo() components.ComponentInfo {
	return components.ComponentInfo{
		Name:         "Digital Matter",
		Manufacturer: "Digital Matter Pty Ltd",
		Version:      "1.0.0",
		Description:  "Support for Digital Matter LoRaWAN tracking devices",
		DeviceTypes:  []components.DeviceType{DeviceTypeYabbyEdge},
	}
}

// GetSupportedDevices returns the device types this component supports.
func (c *DigitalMatterComponent) GetSupportedDevices() []components.DeviceType {
	return []components.DeviceType{DeviceTypeYabbyEdge}
}

// CanHandle checks if this component can handle the given device type and payload.
func (c *DigitalMatterComponent) CanHandle(deviceType components.DeviceType, payload *components.RawPayload) bool {
	parser, exists := c.parsers[deviceType]
	return exists && parser != nil
}

// Parse converts raw payload into structured ParsedData (DEPRECATED: Use ParseToEntities).
func (c *DigitalMatterComponent) Parse(ctx context.Context, deviceType components.DeviceType, payload *components.RawPayload) (*components.ParsedData, error) {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return nil, fmt.Errorf("no parser found for device type %s", deviceType)
	}

	return parser.ParsePayload(payload)
}

// ParseToEntities converts raw payload into multiple entities.
func (c *DigitalMatterComponent) ParseToEntities(ctx context.Context, orgSlug, model string, deviceType components.DeviceType, payload *components.RawPayload, deviceLocation *components.Location) (*components.ParseResult, error) {
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
		string(deviceType),
		"Digital Matter",
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

// Validate performs device-specific validation on the parsed data.
func (c *DigitalMatterComponent) Validate(deviceType components.DeviceType, data *components.ParsedData) error {
	if _, exists := c.parsers[deviceType]; !exists {
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

// SupportsGPS returns true if the device has built-in GPS.
func (c *DigitalMatterComponent) SupportsGPS(deviceType components.DeviceType) bool {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return false
	}
	return parser.SupportsGPS()
}

// GetSupportedPorts returns the fPorts this device type uses.
func (c *DigitalMatterComponent) GetSupportedPorts(deviceType components.DeviceType) []int {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return nil
	}
	return parser.GetSupportedPorts()
}

// GetSupportedEntityTypes returns the entity types this device supports.
func (c *DigitalMatterComponent) GetSupportedEntityTypes(deviceType components.DeviceType) []string {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return nil
	}
	return parser.GetSupportedEntityTypes()
}
