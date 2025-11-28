package devices

import (
	"context"
	"fmt"
	"time"
)

// DeviceType represents the model/type of a device
type DeviceType string

const (
	DeviceTypeRAK2270 DeviceType = "RAK2270"
	DeviceTypeUnknown DeviceType = "UNKNOWN"
)

// DeviceIdentifier represents a single identifier for a device
type DeviceIdentifier struct {
	Type  string `json:"type"`  // "lorawan", "satellite", "network", etc.
	Key   string `json:"key"`   // "dev_eui", "esn", "mac", etc.
	Value string `json:"value"` // The actual identifier value
}

// DeviceConnection represents a network-based connection identifier
type DeviceConnection struct {
	Type  string `json:"type"`  // "mac", "ip", "bluetooth", etc.
	Value string `json:"value"` // The connection identifier value
}

// DeviceEntry represents a comprehensive device entry
type DeviceEntry struct {
	// Core identification
	ID          string             `json:"id"`          // Unique device ID
	Identifiers []DeviceIdentifier `json:"identifiers"` // Multiple identifiers
	Connections []DeviceConnection `json:"connections"` // Network connections
	
	// Device information
	Name         string     `json:"name,omitempty"`
	Manufacturer string     `json:"manufacturer,omitempty"`
	Model        string     `json:"model,omitempty"`
	ModelID      string     `json:"model_id,omitempty"`
	DeviceType   DeviceType `json:"device_type"`
	
	// Version information
	HWVersion string `json:"hw_version,omitempty"`
	SWVersion string `json:"sw_version,omitempty"`
	
	// Configuration and capabilities
	SupportedPorts    []int    `json:"supported_ports,omitempty"`
	SupportedProtocols []string `json:"supported_protocols,omitempty"`
	HasGPS            bool     `json:"has_gps,omitempty"`
	
	// Organization and area (SpaceDF specific)
	Organization string  `json:"organization,omitempty"`
	AreaID       *string `json:"area_id,omitempty"`
	
	// Status and control
	DisabledBy   *string `json:"disabled_by,omitempty"`
	ConfigURL    *string `json:"config_url,omitempty"`
	
	// Timestamps
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	LastSeen  *time.Time `json:"last_seen,omitempty"`
	
	// Additional metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Helper methods for DeviceEntry

// GetIdentifierByKey returns the first identifier with the specified key
func (d *DeviceEntry) GetIdentifierByKey(key string) *DeviceIdentifier {
	for _, identifier := range d.Identifiers {
		if identifier.Key == key {
			return &identifier
		}
	}
	return nil
}

// GetIdentifierValue returns the value of the first identifier with the specified key
func (d *DeviceEntry) GetIdentifierValue(key string) string {
	if identifier := d.GetIdentifierByKey(key); identifier != nil {
		return identifier.Value
	}
	return ""
}

// GetConnectionByType returns the first connection with the specified type
func (d *DeviceEntry) GetConnectionByType(connType string) *DeviceConnection {
	for _, connection := range d.Connections {
		if connection.Type == connType {
			return &connection
		}
	}
	return nil
}

// GetConnectionValue returns the value of the first connection with the specified type
func (d *DeviceEntry) GetConnectionValue(connType string) string {
	if connection := d.GetConnectionByType(connType); connection != nil {
		return connection.Value
	}
	return ""
}

// Common identifier getters for SpaceDF

// GetDevEUI returns the LoRaWAN DevEUI identifier value
func (d *DeviceEntry) GetDevEUI() string {
	return d.GetIdentifierValue("dev_eui")
}

// GetESN returns the satellite ESN identifier value
func (d *DeviceEntry) GetESN() string {
	return d.GetIdentifierValue("esn")
}

// GetMAC returns the network MAC address
func (d *DeviceEntry) GetMAC() string {
	return d.GetConnectionValue("mac")
}

// AddIdentifier adds a new identifier to the device
func (d *DeviceEntry) AddIdentifier(identifierType, key, value string) {
	d.Identifiers = append(d.Identifiers, DeviceIdentifier{
		Type:  identifierType,
		Key:   key,
		Value: value,
	})
	d.UpdatedAt = time.Now()
}

// AddConnection adds a new connection to the device
func (d *DeviceEntry) AddConnection(connType, value string) {
	d.Connections = append(d.Connections, DeviceConnection{
		Type:  connType,
		Value: value,
	})
	d.UpdatedAt = time.Now()
}

// UpdateLastSeen updates the last seen timestamp
func (d *DeviceEntry) UpdateLastSeen() {
	now := time.Now()
	d.LastSeen = &now
	d.UpdatedAt = now
}

// SupportsPort checks if the device supports a specific fPort
func (d *DeviceEntry) SupportsPort(port int) bool {
	for _, p := range d.SupportedPorts {
		if p == port {
			return true
		}
	}
	return false
}

// SupportsProtocol checks if the device supports a specific protocol
func (d *DeviceEntry) SupportsProtocol(protocol string) bool {
	for _, p := range d.SupportedProtocols {
		if p == protocol {
			return true
		}
	}
	return false
}

// RawPayload represents the incoming device data before parsing
type RawPayload struct {
	DeviceEUI   string                 `json:"device_eui"`
	FPort       int                    `json:"fport"`
	Data        string                 `json:"data"` // Base64-encoded payload
	Timestamp   time.Time              `json:"timestamp"`
	RxInfo      []GatewayInfo          `json:"rx_info"`
	Metadata    map[string]interface{} `json:"metadata"`
	
	// Enhanced with multi-identifier support
	Identifiers []DeviceIdentifier `json:"identifiers,omitempty"` // Multiple device IDs
	Connections []DeviceConnection `json:"connections,omitempty"` // Network connections
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
