package common

import (
	"time"

	"github.com/Space-DF/transformer-service/internal/lns"
)

// DeviceType represents the model/type of a device.
type DeviceType string

// DeviceTypeUnknown is the default when the type cannot be determined.
const DeviceTypeUnknown DeviceType = "UNKNOWN"

// Parser is the interface every device model must implement.
type Parser interface {
	ParsePayload(payload *RawPayload) (*ParsedData, error)
	ParseToEntities(orgSlug, model string, payload *RawPayload, deviceLocation *Location) ([]Entity, error)
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
}

// RawPayload represents the incoming device data before parsing.
type RawPayload struct {
	DeviceEUI string                 `json:"device_eui"`
	FPort     int                    `json:"fport"`
	Data      string                 `json:"data"` // Base64-encoded payload
	Timestamp time.Time              `json:"timestamp"`
	RxInfo    []GatewayInfo          `json:"rx_info"`
	Metadata  map[string]interface{} `json:"metadata"`
	LNSType   lns.LNSType            `json:"lns_type"`
}

// GatewayInfo contains information about a gateway that received the message.
type GatewayInfo struct {
	GatewayID string   `json:"gateway_id"`
	RSSI      int      `json:"rssi"`
	SNR       float64  `json:"snr"`
	Location  Location `json:"location,omitempty"`
}

// Location represents geographical coordinates.
type Location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Altitude  float64 `json:"altitude,omitempty"`
}

// ParsedData is the intermediate result of parsing a single device uplink.
type ParsedData struct {
	DeviceEUI    string                 `json:"device_eui"`
	DeviceType   DeviceType             `json:"device_type"`
	Timestamp    time.Time              `json:"timestamp"`
	Location     *Location              `json:"location,omitempty"`
	SensorData   map[string]interface{} `json:"sensor_data"`
	BatteryLevel *float64               `json:"battery_level,omitempty"`
	RawData      string                 `json:"raw_data,omitempty"`
}

// ParseResult is the final result containing all generated entities.
type ParseResult struct {
	DeviceEUI  string     `json:"device_eui"`
	DeviceID   string     `json:"device_id,omitempty"`
	SpaceSlug  string     `json:"space_slug,omitempty"`
	DeviceInfo DeviceInfo `json:"device_info"`
	Entities   []Entity   `json:"entities"`
	Timestamp  time.Time  `json:"timestamp"`
}

// DeviceInfo carries device metadata for Home-Assistant-style discovery.
type DeviceInfo struct {
	Identifiers  []string   `json:"identifiers"`
	Connections  [][]string `json:"connections,omitempty"`
	Name         string     `json:"name"`
	Manufacturer string     `json:"manufacturer"`
	Model        string     `json:"model"`
	ModelID      string     `json:"model_id"`
	SWVersion    string     `json:"sw_version,omitempty"`
	HWVersion    string     `json:"hw_version,omitempty"`
	ViaDevice    string     `json:"via_device,omitempty"`
}

// Entity represents a single device capability or sensor reading.
type Entity struct {
	UniqueID    string                 `json:"unique_id"`
	EntityID    string                 `json:"entity_id"`
	EntityType  string                 `json:"entity_type"`
	DeviceClass string                 `json:"device_class,omitempty"`
	Name        string                 `json:"name"`
	State       interface{}            `json:"state"`
	Attributes  map[string]interface{} `json:"attributes"`
	DisplayType []string               `json:"display_type,omitempty"`
	UnitOfMeas  string                 `json:"unit_of_measurement,omitempty"`
	Icon        string                 `json:"icon,omitempty"`
	Enabled     bool                   `json:"enabled"`
	Timestamp   time.Time              `json:"timestamp"`
}
