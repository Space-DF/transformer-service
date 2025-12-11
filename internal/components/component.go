package components

import (
	"context"
	"fmt"
	"time"
)

// DeviceType represents the model/type of a device
type DeviceType string

const (
	DeviceTypeRAK2270 DeviceType = "RAK2270"
	DeviceTypeRAK7200 DeviceType = "RAK7200"
	DeviceTypeRAK4630 DeviceType = "RAK4630"
	DeviceTypeWLBV1   DeviceType = "WLBV1"
	DeviceTypeUnknown DeviceType = "UNKNOWN"
)

// ComponentInfo provides metadata about a component
type ComponentInfo struct {
	Name         string       `json:"name"`
	Manufacturer string       `json:"manufacturer"`
	Version      string       `json:"version"`
	Description  string       `json:"description"`
	DeviceTypes  []DeviceType `json:"device_types"`
}

// RawPayload represents the incoming device data before parsing
type RawPayload struct {
	DeviceEUI string                 `json:"device_eui"`
	FPort     int                    `json:"fport"`
	Data      string                 `json:"data"` // Base64-encoded payload
	Timestamp time.Time              `json:"timestamp"`
	RxInfo    []GatewayInfo          `json:"rx_info"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// GatewayInfo contains information about the gateway that received the message
type GatewayInfo struct {
	GatewayID string   `json:"gateway_id"`
	RSSI      int      `json:"rssi"`
	SNR       float64  `json:"snr"`
	Location  Location `json:"location,omitempty"`
}

// Location represents geographical coordinates
type Location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Altitude  float64 `json:"altitude,omitempty"`
}

// ParsedData represents the device-specific parsed data (DEPRECATED: Use ParseResult instead)
type ParsedData struct {
	DeviceEUI    string                 `json:"device_eui"`
	DeviceType   DeviceType             `json:"device_type"`
	Timestamp    time.Time              `json:"timestamp"`
	Location     *Location              `json:"location,omitempty"`
	SensorData   map[string]interface{} `json:"sensor_data"`
	BatteryLevel *float64               `json:"battery_level,omitempty"`
	RawData      string                 `json:"raw_data,omitempty"`
}

// ParseResult represents the result of parsing device data into multiple entities
type ParseResult struct {
	DeviceEUI  string     `json:"device_eui"`
	DeviceID   string     `json:"device_id,omitempty"`
	SpaceSlug  string     `json:"space_slug,omitempty"`
	DeviceInfo DeviceInfo `json:"device_info"`
	Entities   []Entity   `json:"entities"`
	Timestamp  time.Time  `json:"timestamp"`
}

// DeviceInfo represents device metadata
type DeviceInfo struct {
	Identifiers  []string   `json:"identifiers"`           // ["70b3d57ed005b847"]
	Connections  [][]string `json:"connections,omitempty"` // [["mac", "02:5b:26:a8:dc:12"]]
	Name         string     `json:"name"`                  // "Conference Room Tracker"
	Manufacturer string     `json:"manufacturer"`          // "RAKwireless"
	Model        string     `json:"model"`                 // "RAK2270"
	ModelID      string     `json:"model_id"`              // "rak2270"
	SWVersion    string     `json:"sw_version,omitempty"`
	HWVersion    string     `json:"hw_version,omitempty"`
	ViaDevice    string     `json:"via_device,omitempty"`
}

// Entity represents a single device capability
type Entity struct {
	UniqueID    string                 `json:"unique_id"`                     // "acme_70b3d57ed005b847_location"
	EntityID    string                 `json:"entity_id"`                     // "device_tracker.acme_rakwireless_rak2270_70b3d57ed005b847_location"
	EntityType  string                 `json:"entity_type"`                   // "device_tracker", "sensor", "binary_sensor"
	DeviceClass string                 `json:"device_class,omitempty"`        // "location", "battery", "temperature"
	Name        string                 `json:"name"`                          // "Location", "Battery Level"
	State       interface{}            `json:"state"`                         // "home", 85, 22.5
	Attributes  map[string]interface{} `json:"attributes"`                    // Additional properties
	DisplayType []string							 `json:"display_type,omitempty"` 
	UnitOfMeas  string                 `json:"unit_of_measurement,omitempty"` // "%", "°C"
	Icon        string                 `json:"icon,omitempty"`
	Enabled     bool                   `json:"enabled"` // Default: true
	Timestamp   time.Time              `json:"timestamp"`
}

// DeviceComponent defines the interface that each device component must implement
// This follows a component/platform pattern
type DeviceComponent interface {
	// GetInfo returns component metadata
	GetInfo() ComponentInfo

	// GetSupportedDevices returns the device types this component supports
	GetSupportedDevices() []DeviceType

	// CanHandle checks if this component can handle the given device type and payload
	CanHandle(deviceType DeviceType, payload *RawPayload) bool

	// Parse converts raw payload into structured ParsedData (DEPRECATED: Use ParseToEntities)
	Parse(ctx context.Context, deviceType DeviceType, payload *RawPayload) (*ParsedData, error)

	// ParseToEntities converts raw payload into multiple entities
	ParseToEntities(ctx context.Context, orgSlug, model string, deviceType DeviceType, payload *RawPayload) (*ParseResult, error)

	// Validate performs device-specific validation on the parsed data
	Validate(deviceType DeviceType, data *ParsedData) error

	// SupportsGPS returns true if the device has built-in GPS
	SupportsGPS(deviceType DeviceType) bool

	// GetSupportedPorts returns the fPorts this device type uses
	GetSupportedPorts(deviceType DeviceType) []int

	// GetSupportedEntityTypes returns the entity types this device supports
	GetSupportedEntityTypes(deviceType DeviceType) []string
}

// Helper functions for entity ID generation

// GenerateUniqueID creates a simple unique ID for entity registry
func GenerateUniqueID(model, devEUI, entityType string) string {
	return fmt.Sprintf("%s_%s_%s", model, devEUI, entityType)
}

// GenerateEntityID creates a descriptive entity ID with model information
func GenerateEntityID(domain, orgSlug, manufacturer, model, devEUI, entityType string) string {
	// Format: domain.org_manufacturer_model_deveui_entitytype
	return fmt.Sprintf("%s.%s_%s_%s_%s_%s",
		domain, orgSlug, manufacturer, model, devEUI, entityType)
}

// GetEntityDomain returns the appropriate domain for an entity type
func GetEntityDomain(entityType string) string {
	switch entityType {
	case "location":
		return "device_tracker"
	case "battery", "temperature", "humidity", "pressure":
		return "sensor"
	case "motion", "door", "window":
		return "binary_sensor"
	case "light", "switch":
		return entityType
	default:
		return "sensor"
	}
}

// CreateDeviceInfo creates device info structure
func CreateDeviceInfo(devEUI, name, manufacturer, model, modelID string) DeviceInfo {
	return DeviceInfo{
		Identifiers:  []string{devEUI},
		Name:         name,
		Manufacturer: manufacturer,
		Model:        model,
		ModelID:      modelID,
	}
}

// ComponentWithSetup extends DeviceComponent with optional setup/teardown
type ComponentWithSetup interface {
	DeviceComponent

	// Setup is called when the component is loaded
	Setup(ctx context.Context) error

	// Teardown is called when the component is unloaded
	Teardown(ctx context.Context) error
}
