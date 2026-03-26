package abeeway

import (
	"context"
	"fmt"

	"github.com/Space-DF/transformer-service/internal/components"
)

// Abeeway Devices
const (
	DeviceTypeAbeewayIndustrialTracker = "ABEEWAY_INDUSTRIAL_TRACKER"
)

// AbeewayComponent handles Abeeway Industrial Tracker devices
type AbeewayComponent struct {
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

// NewAbeewayComponent creates a new Abeeway component
func NewAbeewayComponent() *AbeewayComponent {
	component := &AbeewayComponent{
		parsers: make(map[components.DeviceType]DeviceParser),
	}

	// Register device-specific parsers
	component.parsers[DeviceTypeAbeewayIndustrialTracker] = NewIndustrialTrackerParser()
	return component
}

// GetInfo returns component metadata
func (c *AbeewayComponent) GetInfo() components.ComponentInfo {
	return components.ComponentInfo{
		Name:         "abeeway",
		Manufacturer: "Abeeway (Actility)",
		Version:      "1.0.0",
		Description:  "Component for Abeeway Industrial Tracker LoRaWAN asset tracking device",
		DeviceTypes: []components.DeviceType{
			DeviceTypeAbeewayIndustrialTracker,
		},
	}
}

// GetSupportedDevices returns the device types this component supports
func (c *AbeewayComponent) GetSupportedDevices() []components.DeviceType {
	return []components.DeviceType{
		DeviceTypeAbeewayIndustrialTracker,
	}
}

// CanHandle checks if this component can handle the given device type and payload
func (c *AbeewayComponent) CanHandle(deviceType components.DeviceType, payload *components.RawPayload) bool {
	parser, exists := c.parsers[deviceType]
	return exists && parser != nil
}

// Parse converts raw payload into structured ParsedData (DEPRECATED: Use ParseToEntities)
func (c *AbeewayComponent) Parse(ctx context.Context, deviceType components.DeviceType, payload *components.RawPayload) (*components.ParsedData, error) {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return nil, fmt.Errorf("no parser found for device type %s", deviceType)
	}

	return parser.ParsePayload(payload)
}

// ParseToEntities converts raw payload into multiple entities
func (c *AbeewayComponent) ParseToEntities(ctx context.Context, orgSlug, model string, deviceType components.DeviceType, payload *components.RawPayload, deviceLocation *components.Location) (*components.ParseResult, error) {
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
		"Abeeway",
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
func (c *AbeewayComponent) Validate(deviceType components.DeviceType, data *components.ParsedData) error {
	if data.DeviceEUI == "" {
		return fmt.Errorf("device EUI is required")
	}
	if data.DeviceType != deviceType {
		return fmt.Errorf("device type mismatch: expected %s, got %s", deviceType, data.DeviceType)
	}
	return nil
}

// SupportsGPS returns true if the device has built-in GPS
func (c *AbeewayComponent) SupportsGPS(deviceType components.DeviceType) bool {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return false
	}
	return parser.SupportsGPS()
}

// GetSupportedPorts returns the fPorts this device type uses
func (c *AbeewayComponent) GetSupportedPorts(deviceType components.DeviceType) []int {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return nil
	}
	return parser.GetSupportedPorts()
}

// GetSupportedEntityTypes returns the entity types this device supports
func (c *AbeewayComponent) GetSupportedEntityTypes(deviceType components.DeviceType) []string {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return nil
	}
	return parser.GetSupportedEntityTypes()
}
