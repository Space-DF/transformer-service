package models

// ReceiveDataRequest represents the incoming webhook payload
type ReceiveDataRequest struct {
	Payload map[string]interface{} `json:"payload"`
}

// UplinkMessage represents the uplink message structure from LoRaWAN
type UplinkMessage struct {
	RxMetadata []Gateway      `json:"rx_metadata"`
	Settings   UplinkSettings `json:"settings"`
}

// Gateway represents a LoRaWAN gateway
type Gateway struct {
	Location *Location `json:"location,omitempty"`
	RSSI     int       `json:"rssi"`
}

// Location represents geographical coordinates
type Location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// UplinkSettings contains uplink transmission settings
type UplinkSettings struct {
	Frequency int64 `json:"frequency"`
}

// EndDeviceIDs represents the device identifiers
type EndDeviceIDs struct {
	DevEUI string `json:"dev_eui"`
}

// DeviceLocationData represents the calculated device location
type DeviceLocationData struct {
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	DevEUI       string  `json:"dev_eui"`
	Organization string  `json:"organization"`
	Manufacture  string  `json:"manufacture,omitempty"`
}

// TransformedDeviceData represents the final transformed output (DEPRECATED: Use TelemetryPayload)
type TransformedDeviceData struct {
	DeviceEUI    string                 `json:"device_eui"`
	DeviceID     string                 `json:"device_id"`
	SpaceSlug    string                 `json:"space_slug"`
	IsPublished  bool                   `json:"is_published"`
	Location     LocationCoordinates    `json:"location"`
	Timestamp    string                 `json:"timestamp"`
	Organization string                 `json:"organization"`
	Metadata     map[string]interface{} `json:"metadata"`
	Source       string                 `json:"source"`
}

// TelemetryPayload represents the new entity-based telemetry output
type TelemetryPayload struct {
	Organization string                 `json:"organization"`
	DeviceEUI    string                 `json:"device_eui"`
	DeviceID     string                 `json:"device_id,omitempty"`
	SpaceSlug    string                 `json:"space_slug,omitempty"`
	DeviceInfo   TelemetryDeviceInfo    `json:"device_info"`
	Entities     []TelemetryEntity      `json:"entities"`
	Timestamp    string                 `json:"timestamp"`
	Source       string                 `json:"source"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// EntityTelemetryPayload represents telemetry for a single entity (no entities array)
type EntityTelemetryPayload struct {
	Organization string                 `json:"organization"`
	DeviceEUI    string                 `json:"device_eui"`
	DeviceID     string                 `json:"device_id,omitempty"`
	SpaceSlug    string                 `json:"space_slug,omitempty"`
	Entity       TelemetryEntity        `json:"entity"`
	Timestamp    string                 `json:"timestamp"`
	Source       string                 `json:"source"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// TelemetryDeviceInfo represents device information for telemetry
type TelemetryDeviceInfo struct {
	Identifiers  []string `json:"identifiers"`  // ["70b3d57ed005b847"]
	Name         string   `json:"name"`         // "Conference Room Tracker"
	Manufacturer string   `json:"manufacturer"` // "RAKwireless"
	Model        string   `json:"model"`        // "RAK2270"
	ModelID      string   `json:"model_id"`     // "rak2270"
}

// TelemetryEntity represents a single entity in telemetry output
type TelemetryEntity struct {
	UniqueID    string                 `json:"unique_id"`                     // "acme_70b3d57ed005b847_location"
	EntityID    string                 `json:"entity_id"`                     // "device_tracker.acme_rakwireless_rak2270_70b3d57ed005b847_location"
	EntityType  string                 `json:"entity_type"`                   // "device_tracker", "sensor"
	DeviceClass string                 `json:"device_class,omitempty"`        // "location", "battery"
	Name        string                 `json:"name"`                          // "Location", "Battery Level"
	State       interface{}            `json:"state"`                         // "home", 85, 22.5
	Attributes  map[string]interface{} `json:"attributes,omitempty"`          // Additional properties
	DisplayType []string               `json:"display_type,omitempty"`        // UI hints (e.g., map, chart)
	UnitOfMeas  string                 `json:"unit_of_measurement,omitempty"` // "%", "°C"
	Icon        string                 `json:"icon,omitempty"`
	Timestamp   string                 `json:"timestamp"`
}

// LocationCoordinates represents geographic coordinates
type LocationCoordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Accuracy  float64 `json:"accuracy"`
}

// LocationPoint represents a point in 2D space
type LocationPoint struct {
	Latitude  float64
	Longitude float64
	RSSI      int
}

// RawDataLog represents the structure for logging raw data for training
type RawDataLog struct {
	ID              string                 `json:"id"`
	Timestamp       string                 `json:"timestamp"`
	DeviceEUI       string                 `json:"device_eui,omitempty"`
	DeviceID        string                 `json:"device_id,omitempty"`
	DeviceName      string                 `json:"device_name,omitempty"`
	EventType       string                 `json:"event_type,omitempty"`
	RawData         string                 `json:"raw_data,omitempty"`
	DecodedRawData  interface{}            `json:"decoded_raw_data,omitempty"`
	OriginalPayload map[string]interface{} `json:"original_payload"`
	ProcessingInfo  ProcessingInfo         `json:"processing_info"`
}

// ProcessingInfo contains information about how the data was processed
type ProcessingInfo struct {
	LocationCalculated bool            `json:"location_calculated"`
	ErrorMessage       string          `json:"error_message,omitempty"`
	GatewayCount       int             `json:"gateway_count"`
	HasLocationData    bool            `json:"has_location_data"`
	LocationResult     *LocationResult `json:"location_result,omitempty"`
}

// LocationResult contains the calculated location information for logging
type LocationResult struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Accuracy  float64 `json:"accuracy"`
}

// DeviceLookupResponse represents the payload returned by the device lookup API
type DeviceLookupResponse struct {
	ID          string `json:"id"`
	DeviceID    string `json:"device_id"`
	DeviceModel string `json:"device_model"`
	SpaceSlug   string `json:"space_slug"`
	IsPublished bool   `json:"is_published"`
}

// DeviceMapping represents a device EUI to profile mapping
// DEPRECATED: Use Device instead for new code
type DeviceMapping struct {
	Profile      string `json:"device_profile"`
	Organization string `json:"organization"`
	DeviceID     string `json:"id"`
	DeviceName   string `json:"device_name"`
	Manufacture  string `json:"manufacture"`
	Description  string `json:"description"`
	SpaceSlug    string `json:"space_slug"`
	IsPublished  bool   `json:"is_published"`
	Skip         bool   `json:"skip,omitempty"`
}

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

// Device represents a unified device entry combining DeviceMapping and DeviceEntry
type Device struct {
	// Core identification
	ID          string             `json:"id"`          // Unique device ID
	DeviceEUI   string             `json:"device_eui"`  // Primary LoRaWAN DevEUI (for backward compatibility)
	Identifiers []DeviceIdentifier `json:"identifiers"` // Multiple identifiers
	Connections []DeviceConnection `json:"connections"` // Network connections

	// Device information
	Name         string `json:"name,omitempty"`
	Description  string `json:"description,omitempty"`
	Manufacturer string `json:"manufacturer,omitempty"`
	Model        string `json:"model,omitempty"`
	ModelID      string `json:"model_id,omitempty"`

	// Device profile and configuration
	Profile            string   `json:"device_profile"` // Device profile name
	SupportedPorts     []int    `json:"supported_ports,omitempty"`
	SupportedProtocols []string `json:"supported_protocols,omitempty"`
	HasGPS             bool     `json:"has_gps,omitempty"`

	// Organization and space (SpaceDF specific)
	Organization string `json:"organization"`
	SpaceSlug    string `json:"space_slug,omitempty"`

	// Status and control
	IsPublished bool   `json:"is_published"`
	Skip        bool   `json:"skip,omitempty"`
	DisabledBy  string `json:"disabled_by,omitempty"`

	// Version information
	HWVersion string `json:"hw_version,omitempty"`
	SWVersion string `json:"sw_version,omitempty"`

	// Timestamps
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	LastSeen  string `json:"last_seen,omitempty"`

	// Additional metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Helper methods for Device

// GetIdentifierByKey returns the first identifier with the specified key
func (d *Device) GetIdentifierByKey(key string) *DeviceIdentifier {
	for _, identifier := range d.Identifiers {
		if identifier.Key == key {
			return &identifier
		}
	}
	return nil
}

// GetIdentifierValue returns the value of the first identifier with the specified key
func (d *Device) GetIdentifierValue(key string) string {
	if identifier := d.GetIdentifierByKey(key); identifier != nil {
		return identifier.Value
	}
	return ""
}

// GetConnectionByType returns the first connection with the specified type
func (d *Device) GetConnectionByType(connType string) *DeviceConnection {
	for _, connection := range d.Connections {
		if connection.Type == connType {
			return &connection
		}
	}
	return nil
}

// GetConnectionValue returns the value of the first connection with the specified type
func (d *Device) GetConnectionValue(connType string) string {
	if connection := d.GetConnectionByType(connType); connection != nil {
		return connection.Value
	}
	return ""
}

// GetDevEUI returns the LoRaWAN DevEUI identifier value
func (d *Device) GetDevEUI() string {
	// Check primary DeviceEUI field first for backward compatibility
	if d.DeviceEUI != "" {
		return d.DeviceEUI
	}
	// Fallback to identifiers
	return d.GetIdentifierValue("dev_eui")
}

// GetESN returns the satellite ESN identifier value
func (d *Device) GetESN() string {
	return d.GetIdentifierValue("esn")
}

// GetMAC returns the network MAC address
func (d *Device) GetMAC() string {
	return d.GetConnectionValue("mac")
}

// AddIdentifier adds a new identifier to the device
func (d *Device) AddIdentifier(identifierType, key, value string) {
	d.Identifiers = append(d.Identifiers, DeviceIdentifier{
		Type:  identifierType,
		Key:   key,
		Value: value,
	})
}

// AddConnection adds a new connection to the device
func (d *Device) AddConnection(connType, value string) {
	d.Connections = append(d.Connections, DeviceConnection{
		Type:  connType,
		Value: value,
	})
}

// SupportsPort checks if the device supports a specific fPort
func (d *Device) SupportsPort(port int) bool {
	for _, p := range d.SupportedPorts {
		if p == port {
			return true
		}
	}
	return false
}

// SupportsProtocol checks if the device supports a specific protocol
func (d *Device) SupportsProtocol(protocol string) bool {
	for _, p := range d.SupportedProtocols {
		if p == protocol {
			return true
		}
	}
	return false
}

// ToDeviceMapping converts the unified Device to legacy DeviceMapping for backward compatibility
func (d *Device) ToDeviceMapping() DeviceMapping {
	return DeviceMapping{
		Profile:      d.Profile,
		Organization: d.Organization,
		DeviceID:     d.ID,
		DeviceName:   d.Name,
		Description:  d.Description,
		SpaceSlug:    d.SpaceSlug,
		IsPublished:  d.IsPublished,
		Skip:         d.Skip,
	}
}

// FromDeviceMapping creates a Device from legacy DeviceMapping
func FromDeviceMapping(dm DeviceMapping, deviceEUI string) Device {
	device := Device{
		ID:           dm.DeviceID,
		DeviceEUI:    deviceEUI, // Maintain backward compatibility
		Name:         dm.DeviceName,
		Description:  dm.Description,
		Profile:      dm.Profile,
		Organization: dm.Organization,
		SpaceSlug:    dm.SpaceSlug,
		IsPublished:  dm.IsPublished,
		Skip:         dm.Skip,
		Identifiers:  []DeviceIdentifier{},
		Connections:  []DeviceConnection{},
	}

	// Add DevEUI as an identifier
	if deviceEUI != "" {
		device.AddIdentifier("lorawan", "dev_eui", deviceEUI)
	}

	return device
}
