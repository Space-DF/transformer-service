package dut

import (
	"context"
	"fmt"

	"github.com/Space-DF/transformer-service/internal/components"
)

const (
	DeviceTypeWLBV1 = "WLBV1"
)

// This follows a manufacturer-based component pattern
type DUTComponent struct {
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

// NewDUTComponent creates a new DUT component
func NewDUTComponent() *DUTComponent {
	component := &DUTComponent{
		parsers: make(map[components.DeviceType]DeviceParser),
	}

	// Register device-specific parsers
	component.parsers[DeviceTypeWLBV1] = NewWLBV1Parser()
	return component
}

// GetInfo returns component metadata
func (c *DUTComponent) GetInfo() components.ComponentInfo {
	return components.ComponentInfo{
		Name:         "DUT",
		Manufacturer: "DUT",
		Version:      "1.0.0",
		Description:  "Component for WLBV1 devices",
		DeviceTypes: []components.DeviceType{
			DeviceTypeWLBV1,
		},
	}
}

// GetSupportedDevices returns the device types this component supports
func (c *DUTComponent) GetSupportedDevices() []components.DeviceType {
	return []components.DeviceType{
		DeviceTypeWLBV1,
	}
}

// CanHandle checks if this component can handle the given device type and payload
func (c *DUTComponent) CanHandle(deviceType components.DeviceType, payload *components.RawPayload) bool {
	parser, exists := c.parsers[deviceType]
	return exists && parser != nil
}

// Parse converts raw payload into structured ParsedData (DEPRECATED: Use ParseToEntities)
func (c *DUTComponent) Parse(ctx context.Context, deviceType components.DeviceType, payload *components.RawPayload) (*components.ParsedData, error) {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return nil, fmt.Errorf("no parser found for device type %s", deviceType)
	}

	return parser.ParsePayload(payload)
}

// ParseToEntities converts raw payload into multiple entities
func (c *DUTComponent) ParseToEntities(ctx context.Context, orgSlug, model string, deviceType components.DeviceType, payload *components.RawPayload, deviceLocation *components.Location) (*components.ParseResult, error) {
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
		"DUT",
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
func (c *DUTComponent) Validate(deviceType components.DeviceType, data *components.ParsedData) error {
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
func (c *DUTComponent) SupportsGPS(deviceType components.DeviceType) bool {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return false
	}
	return parser.SupportsGPS()
}

// GetSupportedPorts returns the fPorts this device type uses
func (c *DUTComponent) GetSupportedPorts(deviceType components.DeviceType) []int {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return nil
	}
	return parser.GetSupportedPorts()
}

// GetSupportedEntityTypes returns the entity types this device supports
func (c *DUTComponent) GetSupportedEntityTypes(deviceType components.DeviceType) []string {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return nil
	}
	return parser.GetSupportedEntityTypes()
}
