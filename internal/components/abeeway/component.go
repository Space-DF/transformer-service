package abeeway

import (
	"context"
	"fmt"

	"github.com/Space-DF/transformer-service/internal/components"
)

// AbeewayComponent handles Abeeway Industrial Tracker devices
type AbeewayComponent struct {
	parser *IndustrialTrackerParser
}

// NewAbeewayComponent creates a new Abeeway component
func NewAbeewayComponent() *AbeewayComponent {
	return &AbeewayComponent{
		parser: NewIndustrialTrackerParser(),
	}
}

// GetInfo returns component metadata
func (c *AbeewayComponent) GetInfo() components.ComponentInfo {
	return components.ComponentInfo{
		Name:         "abeeway",
		Manufacturer: "Abeeway (Actility)",
		Version:      "1.0.0",
		Description:  "Component for Abeeway Industrial Tracker LoRaWAN asset tracking device",
		DeviceTypes: []components.DeviceType{
			components.DeviceTypeAbeewayIndustrialTracker,
		},
	}
}

// GetSupportedDevices returns the device types this component supports
func (c *AbeewayComponent) GetSupportedDevices() []components.DeviceType {
	return []components.DeviceType{
		components.DeviceTypeAbeewayIndustrialTracker,
	}
}

// CanHandle checks if this component can handle the given device type and payload
func (c *AbeewayComponent) CanHandle(deviceType components.DeviceType, payload *components.RawPayload) bool {
	return deviceType == components.DeviceTypeAbeewayIndustrialTracker
}

// Parse converts raw payload into structured ParsedData (DEPRECATED: Use ParseToEntities)
func (c *AbeewayComponent) Parse(ctx context.Context, deviceType components.DeviceType, payload *components.RawPayload) (*components.ParsedData, error) {
	return c.parser.ParsePayload(payload)
}

// ParseToEntities converts raw payload into multiple entities
func (c *AbeewayComponent) ParseToEntities(ctx context.Context, orgSlug, model string, deviceType components.DeviceType, payload *components.RawPayload, deviceLocation *components.Location) (*components.ParseResult, error) {
	entities, err := c.parser.ParseToEntities(orgSlug, model, payload, deviceLocation)
	if err != nil {
		return nil, err
	}

	// Create device info
	deviceInfo := components.CreateDeviceInfo(
		payload.DeviceEUI,
		fmt.Sprintf("Industrial Tracker %s", payload.DeviceEUI[len(payload.DeviceEUI)-4:]),
		"Abeeway",
		"Industrial Tracker",
		"abeeway_industrial_tracker",
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
	return deviceType == components.DeviceTypeAbeewayIndustrialTracker
}

// GetSupportedPorts returns the fPorts this device type uses
func (c *AbeewayComponent) GetSupportedPorts(deviceType components.DeviceType) []int {
	return c.parser.GetSupportedPorts()
}

// GetSupportedEntityTypes returns the entity types this device supports
func (c *AbeewayComponent) GetSupportedEntityTypes(deviceType components.DeviceType) []string {
	return c.parser.GetSupportedEntityTypes()
}

// Helper function to extract DevEUI from various payload formats
func extractDevEUI(metadata map[string]interface{}) string {
	if deviceInfo, ok := metadata["deviceInfo"].(map[string]interface{}); ok {
		if devEUI, ok := deviceInfo["devEui"].(string); ok {
			return devEUI
		}
	}
	
	return ""
}
