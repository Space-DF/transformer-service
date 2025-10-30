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
}

// TransformedDeviceData represents the final transformed output
type TransformedDeviceData struct {
	DeviceEUI    string                 `json:"device_eui"`
	DeviceID     string                 `json:"device_id"`
	Location     LocationCoordinates    `json:"location"`
	Timestamp    string                 `json:"timestamp"`
	Organization string                 `json:"organization"`
	Metadata     map[string]interface{} `json:"metadata"`
	Source       string                 `json:"source"`
}

// LocationCoordinates represents geographic coordinates
type LocationCoordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Accuracy  string  `json:"accuracy"`
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
	Accuracy  string  `json:"accuracy"`
}

// DeviceProfile represents a device profile configuration
type DeviceProfile struct {
	Name                        string `json:"name"`
	Description                 string `json:"description"`
	HasGPS                      bool   `json:"has_gps"`
	ParserType                  string `json:"parser_type"`
	LocationCalculationRequired bool   `json:"location_calculation_required"`
	SupportedPorts              []int  `json:"supported_ports"`
	PayloadFormat               string `json:"payload_format"`
}

// DeviceMapping represents a device EUI to profile mapping
type DeviceMapping struct {
	Profile      string `json:"device_profile"`
	Organization string `json:"organization"`
	DeviceID     string `json:"device_id"`
	DeviceName   string `json:"device_name"`
	Description  string `json:"description"`
	Skip         bool   `json:"skip,omitempty"`
}

// DeviceProfiles represents the complete device profiles configuration
type DeviceProfiles struct {
	DeviceProfiles map[string]DeviceProfile `json:"device_profiles"`
	DeviceMappings map[string]DeviceMapping `json:"device_mappings"`
}
