package devices

import (
	"context"
	"time"
)

// DeviceType represents the model/type of a device
type DeviceType string

const (
	DeviceTypeRAK2270 DeviceType = "RAK2270"
	DeviceTypeUnknown DeviceType = "UNKNOWN"
)

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

// DeviceParser defines the interface that each device plugin must implement
type DeviceParser interface {
	// GetDeviceType returns the device type this parser handles
	GetDeviceType() DeviceType

	// CanParse checks if this parser can handle the given payload
	// This is used for device detection/routing
	CanParse(payload *RawPayload) bool

	// Parse converts raw payload into structured ParsedData
	Parse(ctx context.Context, payload *RawPayload) (*ParsedData, error)

	// Validate performs device-specific validation on the parsed data
	Validate(data *ParsedData) error
}

// ParserMetadata provides information about the device parser
type ParserMetadata struct {
	DeviceType   DeviceType
	Manufacturer string
	Model        string
	Version      string
	Description  string
	SupportedPorts []int // FPorts this device uses
}

// DeviceParserWithMetadata extends DeviceParser with metadata
type DeviceParserWithMetadata interface {
	DeviceParser
	GetMetadata() ParserMetadata
}
