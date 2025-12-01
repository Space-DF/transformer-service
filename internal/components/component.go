package components

import (
	"context"
	"time"

	"github.com/Space-DF/transformer-service/internal/models"
)

// DeviceType represents the model/type of a device
type DeviceType string

const (
	DeviceTypeRAK2270 DeviceType = "RAK2270"
	DeviceTypeRAK7200 DeviceType = "RAK7200"
	DeviceTypeRAK4630 DeviceType = "RAK4630"
	DeviceTypeUnknown DeviceType = "UNKNOWN"
)

// ComponentInfo provides metadata about a component
type ComponentInfo struct {
	Name         string     `json:"name"`
	Manufacturer string     `json:"manufacturer"`
	Version      string     `json:"version"`
	Description  string     `json:"description"`
	DeviceTypes  []DeviceType `json:"device_types"`
}

// RawPayload represents the incoming device data before parsing
type RawPayload struct {
	DeviceEUI   string                 `json:"device_eui"`
	FPort       int                    `json:"fport"`
	Data        string                 `json:"data"` // Base64-encoded payload
	Timestamp   time.Time              `json:"timestamp"`
	RxInfo      []GatewayInfo          `json:"rx_info"`
	Metadata    map[string]interface{} `json:"metadata"`
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

// ParsedData represents the device-specific parsed data
type ParsedData struct {
	DeviceEUI    string                 `json:"device_eui"`
	DeviceType   DeviceType             `json:"device_type"`
	Timestamp    time.Time              `json:"timestamp"`
	Location     *Location              `json:"location,omitempty"`
	SensorData   map[string]interface{} `json:"sensor_data"`
	BatteryLevel *float64               `json:"battery_level,omitempty"`
	RawData      string                 `json:"raw_data,omitempty"`
}

// DeviceComponent defines the interface that each device component must implement
// This follows Home Assistant's component/platform pattern
type DeviceComponent interface {
	// GetInfo returns component metadata
	GetInfo() ComponentInfo

	// GetSupportedDevices returns the device types this component supports
	GetSupportedDevices() []DeviceType

	// CanHandle checks if this component can handle the given device type and payload
	CanHandle(deviceType DeviceType, payload *RawPayload) bool

	// Parse converts raw payload into structured ParsedData
	Parse(ctx context.Context, deviceType DeviceType, payload *RawPayload) (*ParsedData, error)

	// Validate performs device-specific validation on the parsed data
	Validate(deviceType DeviceType, data *ParsedData) error

	// SupportsGPS returns true if the device has built-in GPS
	SupportsGPS(deviceType DeviceType) bool

	// GetSupportedPorts returns the fPorts this device type uses
	GetSupportedPorts(deviceType DeviceType) []int
}

// ComponentWithSetup extends DeviceComponent with optional setup/teardown
type ComponentWithSetup interface {
	DeviceComponent
	
	// Setup is called when the component is loaded
	Setup(ctx context.Context) error
	
	// Teardown is called when the component is unloaded
	Teardown(ctx context.Context) error
}