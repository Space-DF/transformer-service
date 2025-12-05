package rakwireless

import (
	"context"
	"fmt"

	"github.com/Space-DF/transformer-service/internal/components"
)

// RAKwirelessComponent handles all RAKwireless devices
// This follows a manufacturer-based component pattern
type RAKwirelessComponent struct {
	parsers map[components.DeviceType]DeviceParser
}

// DeviceParser handles device-specific parsing logic
type DeviceParser interface {
	ParsePayload(payload *components.RawPayload) (*components.ParsedData, error)
	ParseToEntities(orgSlug string, payload *components.RawPayload) ([]components.Entity, error)
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
}

// NewRAKwirelessComponent creates a new RAKwireless component
func NewRAKwirelessComponent() *RAKwirelessComponent {
	component := &RAKwirelessComponent{
		parsers: make(map[components.DeviceType]DeviceParser),
	}

	// Register device-specific parsers
	component.parsers[components.DeviceTypeRAK2270] = NewRAK2270Parser()
	component.parsers[components.DeviceTypeRAK7200] = NewRAK7200Parser()
	component.parsers[components.DeviceTypeRAK4630] = NewRAK4630Parser()

	return component
}

// GetInfo returns component metadata
func (c *RAKwirelessComponent) GetInfo() components.ComponentInfo {
	return components.ComponentInfo{
		Name:         "rakwireless",
		Manufacturer: "RAKwireless",
		Version:      "1.0.0",
		Description:  "Component for RAKwireless devices including RAK2270, RAK7200, and RAK4630",
		DeviceTypes: []components.DeviceType{
			components.DeviceTypeRAK2270,
			components.DeviceTypeRAK7200,
			components.DeviceTypeRAK4630,
		},
	}
}

// GetSupportedDevices returns the device types this component supports
func (c *RAKwirelessComponent) GetSupportedDevices() []components.DeviceType {
	return []components.DeviceType{
		components.DeviceTypeRAK2270,
		components.DeviceTypeRAK7200,
		components.DeviceTypeRAK4630,
	}
}

// CanHandle checks if this component can handle the given device type and payload
func (c *RAKwirelessComponent) CanHandle(deviceType components.DeviceType, payload *components.RawPayload) bool {
	parser, exists := c.parsers[deviceType]
	return exists && parser != nil
}

// Parse converts raw payload into structured ParsedData (DEPRECATED: Use ParseToEntities)
func (c *RAKwirelessComponent) Parse(ctx context.Context, deviceType components.DeviceType, payload *components.RawPayload) (*components.ParsedData, error) {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return nil, fmt.Errorf("no parser found for device type %s", deviceType)
	}

	return parser.ParsePayload(payload)
}

// ParseToEntities converts raw payload into multiple entities
func (c *RAKwirelessComponent) ParseToEntities(ctx context.Context, orgSlug string, deviceType components.DeviceType, payload *components.RawPayload) (*components.ParseResult, error) {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return nil, fmt.Errorf("no parser found for device type %s", deviceType)
	}

	entities, err := parser.ParseToEntities(orgSlug, payload)
	if err != nil {
		return nil, err
	}

	// Create device info
	deviceInfo := components.CreateDeviceInfo(
		payload.DeviceEUI,
		fmt.Sprintf("%s %s", string(deviceType), payload.DeviceEUI[12:]), // "RAK2270 b847"
		"RAKwireless",
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
func (c *RAKwirelessComponent) Validate(deviceType components.DeviceType, data *components.ParsedData) error {
	// Basic validation
	if data.DeviceEUI == "" {
		return fmt.Errorf("device EUI is required")
	}

	if data.DeviceType != deviceType {
		return fmt.Errorf("device type mismatch: expected %s, got %s", deviceType, data.DeviceType)
	}

	// Device-specific validation could be added here
	return nil
}

// SupportsGPS returns true if the device has built-in GPS
func (c *RAKwirelessComponent) SupportsGPS(deviceType components.DeviceType) bool {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return false
	}
	return parser.SupportsGPS()
}

// GetSupportedPorts returns the fPorts this device type uses
func (c *RAKwirelessComponent) GetSupportedPorts(deviceType components.DeviceType) []int {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return nil
	}
	return parser.GetSupportedPorts()
}

// GetSupportedEntityTypes returns the entity types this device supports
func (c *RAKwirelessComponent) GetSupportedEntityTypes(deviceType components.DeviceType) []string {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return nil
	}
	return parser.GetSupportedEntityTypes()
}

// Helper function to extract DevEUI from various payload formats
func extractDevEUI(payload map[string]interface{}) string {
	// Try multiple locations for device EUI
	if endDeviceIDs, ok := payload["end_device_ids"].(map[string]interface{}); ok {
		if devEUI, ok := endDeviceIDs["dev_eui"].(string); ok {
			return devEUI
		}
	}

	if devEUI, ok := payload["dev_eui"].(string); ok {
		return devEUI
	}

	if devEUI, ok := payload["devEui"].(string); ok {
		return devEUI
	}

	if deviceInfo, ok := payload["deviceInfo"].(map[string]interface{}); ok {
		if devEUI, ok := deviceInfo["devEui"].(string); ok {
			return devEUI
		}
	}

	return ""
}
