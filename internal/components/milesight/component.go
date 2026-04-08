package milesight

import (
	"context"
	"fmt"

	"github.com/Space-DF/transformer-service/internal/components"
)

// Milesight Devices
const (
	DeviceTypeCT101 = "CT101"
)

// MilesightComponent handles Milesight CT-series devices
type MilesightComponent struct {
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

// NewMilesightComponent creates a new Milesight component
func NewMilesightComponent() *MilesightComponent {
	component := &MilesightComponent{
		parsers: make(map[components.DeviceType]DeviceParser),
	}

	// Register device-specific parsers
	component.parsers[DeviceTypeCT101] = NewCT101Parser()
	return component
}

// GetInfo returns component metadata
func (c *MilesightComponent) GetInfo() components.ComponentInfo {
	return components.ComponentInfo{
		Name:         "milesight",
		Manufacturer: "Milesight IoT",
		Version:      "1.0.0",
		Description:  "Component for Milesight CT-series current transformer devices including CT101, CT103, and CT105",
		DeviceTypes: []components.DeviceType{
			DeviceTypeCT101,
		},
	}
}

// GetSupportedDevices returns the device types this component supports
func (c *MilesightComponent) GetSupportedDevices() []components.DeviceType {
	return []components.DeviceType{
		DeviceTypeCT101,
	}
}

// CanHandle checks if this component can handle the given device type and payload
func (c *MilesightComponent) CanHandle(deviceType components.DeviceType, payload *components.RawPayload) bool {
	parser, exists := c.parsers[deviceType]
	return exists && parser != nil
}

// Parse converts raw payload into structured ParsedData
func (c *MilesightComponent) Parse(ctx context.Context, deviceType components.DeviceType, payload *components.RawPayload) (*components.ParsedData, error) {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return nil, fmt.Errorf("no parser found for device type %s", deviceType)
	}

	return parser.ParsePayload(payload)
}

// ParseToEntities converts raw payload into multiple entities
func (c *MilesightComponent) ParseToEntities(ctx context.Context, orgSlug, model string, deviceType components.DeviceType, payload *components.RawPayload, deviceLocation *components.Location) (*components.ParseResult, error) {
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
		fmt.Sprintf("%s %s", string(deviceType), payload.DeviceEUI[12:]),
		"Milesight",
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
func (c *MilesightComponent) Validate(deviceType components.DeviceType, data *components.ParsedData) error {
	if data.DeviceEUI == "" {
		return fmt.Errorf("device EUI is required")
	}
	if data.DeviceType != deviceType {
		return fmt.Errorf("device type mismatch: expected %s, got %s", deviceType, data.DeviceType)
	}
	return nil
}

// SupportsGPS returns false - CT101 doesn't have built-in GPS
func (c *MilesightComponent) SupportsGPS(deviceType components.DeviceType) bool {
	return false
}

// GetSupportedPorts returns the fPorts typically used by CT-series devices
func (c *MilesightComponent) GetSupportedPorts(deviceType components.DeviceType) []int {
	return []int{2} // Common fPort for Milesight CT-series
}

// GetSupportedEntityTypes returns the entity types supported by CT-series devices
func (c *MilesightComponent) GetSupportedEntityTypes(deviceType components.DeviceType) []string {
	return []string{"current", "total_current", "temperature"}
}
