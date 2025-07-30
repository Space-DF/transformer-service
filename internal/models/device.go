package models

// ReceiveDataRequest represents the incoming webhook payload
type ReceiveDataRequest struct {
	Payload map[string]interface{} `json:"payload"`
}

// UplinkMessage represents the uplink message structure from LoRaWAN
type UplinkMessage struct {
	RxMetadata []Gateway             `json:"rx_metadata"`
	Settings   UplinkSettings        `json:"settings"`
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