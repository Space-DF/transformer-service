package abeeway

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
)

// Message type constants for Abeeway trackers
const (
	MsgTypeFramePending   = 0x00
	MsgTypePosition       = 0x03
	MsgTypeEnergyStatus   = 0x04
	MsgTypeHeartbeat      = 0x05
	MsgTypeActivityConfig = 0x07
	MsgTypeShutdown       = 0x09
	MsgTypeGeolocStart    = 0x0A
	MsgTypeDebug          = 0xFF
)

// Position data type constants
const (
	PosTypeGPS      = 0x01
	PosTypeWiFi     = 0x02
	PosTypeBLE      = 0x03
	PosTypeLowPower = 0x04
)

// Message type names
var messageTypeNames = map[byte]string{
	MsgTypeFramePending:   "Frame pending",
	MsgTypePosition:       "Position message",
	MsgTypeEnergyStatus:   "Energy status",
	MsgTypeHeartbeat:      "Heartbeat",
	MsgTypeActivityConfig: "Activity/Config",
	MsgTypeShutdown:       "Shutdown",
	MsgTypeGeolocStart:    "Geolocation start",
	MsgTypeDebug:          "Debug",
}

// Status bit masks
const (
	StatusSOSBit              = 0x10 // Bit 4: SOS mode active
	StatusTrackingIdleBit     = 0x08 // Bit 3: Tracking/idle state
	StatusTrackerMovingBit    = 0x04 // Bit 2: Tracker is moving
	StatusPeriodicPositionBit = 0x02 // Bit 1: Periodic position message
	StatusPODMessageBit       = 0x01 // Bit 0: Position on demand
)

// Operating mode values
const (
	ModeStandby               = 0
	ModeMotionTracking        = 1
	ModePermanentTracking     = 2
	ModeMotionStartEnd        = 3
	ModeActivityTracking      = 4
	ModeOff                   = 5
)

var modeNames = map[int]string{
	ModeStandby:           "Standby",
	ModeMotionTracking:    "Motion tracking",
	ModePermanentTracking: "Permanent tracking",
	ModeMotionStartEnd:    "Motion start/end",
	ModeActivityTracking:  "Activity tracking",
	ModeOff:               "Off",
}

// AbeewayPayload represents the parsed common payload header
type AbeewayPayload struct {
	MessageType byte
	Status      byte
	Battery     byte // Raw battery value
	Temperature byte // Raw temperature value
	Ack         byte
	Data        []byte // Message-specific data
}

// PositionData represents parsed position information
type PositionData struct {
	Type        string  // "gps", "wifi", "ble", "low_power"
	Latitude    float64
	Longitude   float64
	Altitude    float64
	Accuracy    float64
	Satellites  int
	Speed       float64
	Heading     float64
	Age         int     // Position age in seconds
	BSSIDList   []string // For WiFi positioning
	BLEData     []BLEBeacon
}

// BLEBeacon represents a detected BLE beacon
type BLEBeacon struct {
	MAC    string
	RSSI   int
	Major  int
	Minor  int
}

// EnergyData represents energy status information
type EnergyData struct {
	BatteryVoltage   float64
	BatteryLevel     float64
	Temperature      float64
	MainSupply       bool
	Charging         bool
	PowerConsumption float64
}

// extractPayloadData extracts the payload data string from various locations
func extractPayloadData(payload interface{}) string {
	switch v := payload.(type) {
	case string:
		return v
	case map[string]interface{}:
		for _, key := range []string{"data", "payload", "frm_payload", "frmPayload", "payload_hex"} {
			if val, ok := v[key].(string); ok && val != "" {
				return val
			}
		}
		if decoded, ok := v["decoded_raw_data"].(map[string]interface{}); ok {
			if uplink, ok := decoded["uplinkEvent"].(map[string]interface{}); ok {
				if data, ok := uplink["data"].(string); ok && data != "" {
					return data
				}
			}
		}
	}
	return ""
}

// decodePayloadBytes decodes hex or base64 encoded payload string to bytes
func decodePayloadBytes(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, fmt.Errorf("empty payload data")
	}

	// First try as hex
	if decoded, err := hex.DecodeString(encoded); err == nil && len(decoded) > 0 {
		return decoded, nil
	}

	// Try as base64
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	return decoded, nil
}

// parseAbeewayHeader parses the common Abeeway payload header
func parseAbeewayHeader(bytes []byte) (*AbeewayPayload, error) {
	if len(bytes) < 5 {
		return nil, fmt.Errorf("payload too short: %d bytes (minimum 5 required)", len(bytes))
	}

	return &AbeewayPayload{
		MessageType: bytes[0],
		Status:      bytes[1],
		Battery:     bytes[2],
		Temperature: bytes[3],
		Ack:         bytes[4],
		Data:        bytes[5:],
	}, nil
}

// decodeBattery converts raw battery byte to voltage
// Abeeway uses: Voltage (mV) = BatteryValue * 10 + 2000
// Returns voltage in volts
func decodeBattery(battery byte) float64 {
	voltageMV := float64(battery)*10.0 + 2000.0
	return voltageMV / 1000.0
}

// decodeBatteryPercent converts raw battery to percentage (approximate)
// Based on Li-ion battery: 3.0V empty to 4.2V full
func decodeBatteryPercent(battery byte) float64 {
	voltage := decodeBattery(battery)
	const minVoltage = 3.0
	const maxVoltage = 4.2
	percent := ((voltage - minVoltage) / (maxVoltage - minVoltage)) * 100.0
	return math.Max(0, math.Min(100, percent))
}

// decodeTemperature converts raw temperature byte to Celsius
// Temperature is signed, stored in 0.5°C steps
func decodeTemperature(temp byte) float64 {
	if temp > 127 {
		// Negative temperature (two's complement)
		return float64(int16(temp)-256) / 2.0
	}
	return float64(temp) / 2.0
}

// decodeStatus decodes the status byte into individual flags
func decodeStatus(status byte) map[string]interface{} {
	return map[string]interface{}{
		"sos_bit":                (status & StatusSOSBit) != 0,
		"tracking_idle_bit":      (status & StatusTrackingIdleBit) != 0,
		"tracker_is_moving_bit":  (status & StatusTrackerMovingBit) != 0,
		"periodic_position_bit":  (status & StatusPeriodicPositionBit) != 0,
		"pod_message_bit":        (status & StatusPODMessageBit) != 0,
		"raw_status":             fmt.Sprintf("0x%02X", status),
	}
}

// parsePositionData parses position message data (type 0x03)
func parsePositionData(data []byte) (*PositionData, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("position data too short: %d bytes", len(data))
	}

	posType := data[0]

	switch posType {
	case PosTypeGPS, PosTypeLowPower:
		return parseGPSPosition(data)
	case PosTypeWiFi:
		return parseWiFiPosition(data)
	case PosTypeBLE:
		return parseBLEPosition(data)
	default:
		return nil, fmt.Errorf("unknown position type: 0x%02X", posType)
	}
}

// parseGPSPosition parses GPS position data
// Format: Type(1) + Status(1) + Lat(4) + Lon(4) + Alt(2) + Course(2) + Speed(2) + Satellites(1) + [HDOP(1)]
func parseGPSPosition(data []byte) (*PositionData, error) {
	if len(data) < 17 {
		return nil, fmt.Errorf("GPS position data too short: %d bytes", len(data))
	}

	status := data[1]
	lat := int32(binary.BigEndian.Uint32(data[2:6]))
	lon := int32(binary.BigEndian.Uint32(data[6:10]))
	alt := int16(binary.BigEndian.Uint16(data[10:12]))
	course := binary.BigEndian.Uint16(data[12:14])
	speed := binary.BigEndian.Uint16(data[14:16])

	pos := &PositionData{
		Type:     "gps",
		Latitude: float64(lat) / 10000000.0,
		Longitude: float64(lon) / 10000000.0,
		Altitude: float64(alt),
		Heading:  float64(course),
		Speed:    float64(speed),
	}

	// Check if GPS fix is valid (bit 0 of status)
	if status&0x01 == 0 {
		pos.Latitude = 0
		pos.Longitude = 0
	}

	// Extract satellites if available
	if len(data) >= 18 {
		pos.Satellites = int(data[17])
	}

	// Extract HDOP for accuracy estimation if available
	if len(data) >= 19 {
		hdop := float64(data[18]) / 10.0
		pos.Accuracy = hdop * 5 // Rough accuracy estimate
	}

	return pos, nil
}

// parseWiFiPosition parses WiFi fingerprinting position data
// Format: Type(1) + Status(1) + Lat(4) + Lon(4) + Age(2) + NbrBSSID(1) + BSSIDList...
func parseWiFiPosition(data []byte) (*PositionData, error) {
	if len(data) < 13 {
		return nil, fmt.Errorf("WiFi position data too short: %d bytes", len(data))
	}

	status := data[1]
	lat := int32(binary.BigEndian.Uint32(data[2:6]))
	lon := int32(binary.BigEndian.Uint32(data[6:10]))
	age := int(binary.BigEndian.Uint16(data[10:12]))
	nbrBSSID := int(data[12])

	pos := &PositionData{
		Type:      "wifi",
		Age:       age,
		BSSIDList: make([]string, 0),
	}

	// Check if WiFi fix is valid (bit 0 of status)
	if status&0x01 != 0 && lat != 0 && lon != 0 {
		pos.Latitude = float64(lat) / 10000000.0
		pos.Longitude = float64(lon) / 10000000.0
		pos.Accuracy = 100 
	}

	// Parse BSSID list (each BSSID is 6 bytes)
	offset := 13
	for i := 0; i < nbrBSSID && offset+6 <= len(data); i++ {
		bssid := fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
			data[offset], data[offset+1], data[offset+2],
			data[offset+3], data[offset+4], data[offset+5])
		pos.BSSIDList = append(pos.BSSIDList, bssid)
		offset += 6
	}

	return pos, nil
}

// parseBLEPosition parses BLE beacon position data
// Format: Type(1) + Status(1) + Lat(4) + Lon(4) + Age(2) + NbrBeacons(1) + Beacons...
func parseBLEPosition(data []byte) (*PositionData, error) {
	if len(data) < 13 {
		return nil, fmt.Errorf("BLE position data too short: %d bytes", len(data))
	}

	status := data[1]
	lat := int32(binary.BigEndian.Uint32(data[2:6]))
	lon := int32(binary.BigEndian.Uint32(data[6:10]))
	age := int(binary.BigEndian.Uint16(data[10:12]))
	nbrBeacons := int(data[12])

	pos := &PositionData{
		Type:    "ble",
		Age:     age,
		BLEData: make([]BLEBeacon, 0),
	}

	// Check if BLE fix is valid (bit 0 of status)
	if status&0x01 != 0 && lat != 0 && lon != 0 {
		pos.Latitude = float64(lat) / 10000000.0
		pos.Longitude = float64(lon) / 10000000.0
		pos.Accuracy = 50 // BLE positioning typically has ~50m accuracy
	}

	// Parse beacon list
	offset := 13
	for i := 0; i < nbrBeacons && offset+14 <= len(data); i++ {
		beacon := BLEBeacon{
			MAC: fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
				data[offset], data[offset+1], data[offset+2],
				data[offset+3], data[offset+4], data[offset+5]),
			RSSI:  int(int16(data[offset+6])),
			Major: int(binary.BigEndian.Uint16(data[offset+7:offset+9])),
			Minor: int(binary.BigEndian.Uint16(data[offset+9:offset+11])),
		}
		pos.BLEData = append(pos.BLEData, beacon)
		offset += 14
	}

	return pos, nil
}

// parseEnergyStatus parses energy status message data (type 0x04)
func parseEnergyStatus(data []byte) (*EnergyData, error) {
	if len(data) < 5 {
		return nil, fmt.Errorf("energy status data too short: %d bytes", len(data))
	}

	// Energy status format: BatteryMv(2) + Temperature(1) + MainSupply(1) + PowerMode(1)
	batteryMV := int(binary.BigEndian.Uint16(data[0:2]))
	temperature := data[2]
	mainSupply := data[3]
	powerMode := data[4]

	return &EnergyData{
		BatteryVoltage:   float64(batteryMV) / 1000.0,
		BatteryLevel:     calculateBatteryPercent(float64(batteryMV) / 1000.0),
		Temperature:      decodeTemperature(temperature),
		MainSupply:       mainSupply == 0x01,
		Charging:         mainSupply == 0x02,
		PowerConsumption: float64(powerMode),
	}, nil
}

// calculateBatteryPercent converts voltage to approximate percentage
func calculateBatteryPercent(voltage float64) float64 {
	const minVoltage = 3.0
	const maxVoltage = 4.2
	percent := ((voltage - minVoltage) / (maxVoltage - minVoltage)) * 100.0
	return math.Max(0, math.Min(100, percent))
}

// validateCoordinates validates GPS coordinates
func validateCoordinates(latitude, longitude float64) error {
	if latitude == 0.0 && longitude == 0.0 {
		return fmt.Errorf("GPS coordinates are 0,0 - no fix available")
	}
	if latitude < -90 || latitude > 90 {
		return fmt.Errorf("invalid latitude: %f", latitude)
	}
	if longitude < -180 || longitude > 180 {
		return fmt.Errorf("invalid longitude: %f", longitude)
	}
	return nil
}

// GetMessageTypeName returns the name of a message type
func GetMessageTypeName(msgType byte) string {
	if name, ok := messageTypeNames[msgType]; ok {
		return name
	}
	return fmt.Sprintf("Unknown (0x%02X)", msgType)
}

// GetModeName returns the name of an operating mode
func GetModeName(mode int) string {
	if name, ok := modeNames[mode]; ok {
		return name
	}
	return fmt.Sprintf("Unknown (%d)", mode)
}
