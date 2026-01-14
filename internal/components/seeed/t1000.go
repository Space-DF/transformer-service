package seeed

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"time"

	"github.com/Space-DF/transformer-service/internal/components"
)

// T1000Parser handles parsing of SenseCAP T1000 device payloads
type T1000Parser struct{}

// NewT1000Parser creates a new T1000 parser
func NewT1000Parser() *T1000Parser {
	return &T1000Parser{}
}

// Packet type constants for T1000
const (
	PacketDeviceStatusEventMode      = 0x01
	PacketDeviceStatusPeriodicMode   = 0x02
	PacketHeartbeat                  = 0x05
	PacketGNSSLocationSensor         = 0x06
	PacketWiFILocationSensor         = 0x07
	PacketBluetoothLocationSensor    = 0x08
	PacketGNSSLocationOnly           = 0x09
	PacketWiFILocationOnly           = 0x0A
	PacketBluetoothLocationOnly      = 0x0B
	PacketErrorCode                  = 0x0D
	PacketPositioningStatusSensor    = 0x11
)

// T1000Payload represents the parsed T1000 payload
type T1000Payload struct {
	PacketID         byte
	BatteryLevel     uint8      // Percentage (0-100)
	Temperature      float64    // Celsius
	Light            uint16     // Percentage (0-10000)
	Latitude         float64
	Longitude        float64
	Altitude         float64
	UTCTime          uint32     // Unix timestamp
	EventStatus      uint32     // Bit flags for events
	WorkMode         uint8
	PositionStrategy uint8
	WiFiMACs         []WiFiMAC
	BLEMACs          []BLEMAC
	MotionSegment    uint8
	PositionStatus   uint8
	ErrorCode        uint32
}

type WiFiMAC struct {
	MAC  string
	RSSI int8
}

type BLEMAC struct {
	MAC  string
	RSSI int8
}

// Work mode values
const (
	WorkModeStandby   = 0x00
	WorkModePeriodic  = 0x01
	WorkModeEvent     = 0x02
)

// Positioning strategy values
const (
	PosGNSS           = 0x00
	PosWiFi           = 0x01
	PosWiFiGNSS       = 0x02
	PosGNSSWiFi       = 0x03
	PosBLE            = 0x04
	PosBLEWiFi        = 0x05
	PosBLEGNSS        = 0x06
	PosBLEWiFiGNSS    = 0x07
)

// Event status bit flags
const (
	EventStartMoving   = 0x000001
	EventEndMoving     = 0x000002
	EventMotionless    = 0x000004
	EventShock         = 0x000008
	EventTemperature   = 0x000010
	EventLight         = 0x000020
	EventSOS           = 0x000040
	EventPressOnce     = 0x000080
)

// ParseToEntities creates entities for T1000 device
func (p *T1000Parser) ParseToEntities(orgSlug, model string, payload *components.RawPayload, deviceLocation *components.Location) ([]components.Entity, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = components.ExtractDevEUI(payload.Metadata)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI is required")
	}

	// Decode payload bytes
	encoded := extractPayloadData(payload.Data)
	if encoded == "" {
		encoded = extractPayloadData(payload.Metadata)
	}
	if encoded == "" {
		return nil, fmt.Errorf("no payload data found")
	}

	bytes, err := decodePayloadBytes(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	// Parse T1000 packet
	t1000Data, err := parseT1000Packet(bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse T1000 packet: %w", err)
	}

	var entities []components.Entity
	timestamp := payload.Timestamp
	modelID := "sensecap_t1000"

	// Location Entity (if we have coordinates)
	if t1000Data.Latitude != 0 || t1000Data.Longitude != 0 {
		locationAttrs := map[string]interface{}{
			"source":       getPositioningSource(t1000Data.PositionStrategy),
			"gps_capable":  true,
			"device_model": "SenseCAP T1000",
			"latitude":     t1000Data.Latitude,
			"longitude":    t1000Data.Longitude,
		}

		if t1000Data.Altitude != 0 {
			locationAttrs["altitude"] = t1000Data.Altitude
		}

		if t1000Data.UTCTime != 0 {
			locationAttrs["gps_time"] = time.Unix(int64(t1000Data.UTCTime), 0).UTC()
		}

		if len(t1000Data.WiFiMACs) > 0 {
			locationAttrs["wifi_mac_addresses"] = t1000Data.WiFiMACs
		}

		if len(t1000Data.BLEMACs) > 0 {
			locationAttrs["ble_mac_addresses"] = t1000Data.BLEMACs
		}

		entities = append(entities, components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "location"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("location"),
				orgSlug, "seeed", modelID, devEUI, "location",
			),
			EntityType:  "location",
			DeviceClass: "location",
			Name:        "Location",
			State:       "home",
			DisplayType: []string{"map"},
			Attributes:  locationAttrs,
			Enabled:     true,
			Timestamp:   timestamp,
		})
	}

	// Battery Entity
	if t1000Data.BatteryLevel > 0 {
		entities = append(entities, components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "battery_level"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("battery"),
				orgSlug, "seeed", modelID, devEUI, "battery_level",
			),
			EntityType:  "battery",
			DeviceClass: "battery",
			Name:        "Battery Level",
			State:       t1000Data.BatteryLevel,
			UnitOfMeas:  "%",
			DisplayType: []string{"chart", "gauge", "value", "slider"},
			Attributes: map[string]interface{}{
				"device_model": "SenseCAP T1000",
			},
			Enabled:   true,
			Timestamp: timestamp,
		})
	}

	// Temperature Entity
	entities = append(entities, components.Entity{
		UniqueID: components.GenerateUniqueID(model, devEUI, "temperature"),
		EntityID: components.GenerateEntityID(
			components.GetEntityDomain("temperature"),
			orgSlug, "seeed", modelID, devEUI, "temperature",
		),
		EntityType:  "temperature",
		DeviceClass: "temperature",
		Name:        "Temperature",
		State:       t1000Data.Temperature,
		UnitOfMeas:  "°C",
		DisplayType: []string{"chart", "gauge", "value"},
		Attributes: map[string]interface{}{
			"device_model": "SenseCAP T1000",
			"sensor_type":  "internal",
		},
		Enabled:   true,
		Timestamp: timestamp,
	})

	// Light Entity (value is directly 0-100%)
	entities = append(entities, components.Entity{
		UniqueID: components.GenerateUniqueID(model, devEUI, "light"),
		EntityID: components.GenerateEntityID(
			"sensor",
			orgSlug, "seeed", modelID, devEUI, "light",
		),
		EntityType:  "sensor",
		DeviceClass: "illuminance",
		Name:        "Light Level",
		State:       float64(t1000Data.Light),
		UnitOfMeas:  "%",
		DisplayType: []string{"chart", "gauge", "value"},
		Attributes: map[string]interface{}{
			"device_model": "SenseCAP T1000",
		},
		Enabled:   true,
		Timestamp: timestamp,
	})

	// Work Mode Entity
	entities = append(entities, components.Entity{
		UniqueID: components.GenerateUniqueID(model, devEUI, "work_mode"),
		EntityID: components.GenerateEntityID(
			"sensor",
			orgSlug, "seeed", modelID, devEUI, "work_mode",
		),
		EntityType:  "sensor",
		DeviceClass: "work_mode",
		Name:        "Work Mode",
		State:       getWorkModeName(t1000Data.WorkMode),
		Attributes: map[string]interface{}{
			"device_model": "SenseCAP T1000",
			"mode_code":    t1000Data.WorkMode,
		},
		Enabled:   true,
		Timestamp: timestamp,
	})

	// Positioning Strategy Entity
	entities = append(entities, components.Entity{
		UniqueID: components.GenerateUniqueID(model, devEUI, "positioning_strategy"),
		EntityID: components.GenerateEntityID(
			"sensor",
			orgSlug, "seeed", modelID, devEUI, "positioning_strategy",
		),
		EntityType:  "sensor",
		DeviceClass: "positioning_strategy",
		Name:        "Positioning Strategy",
		State:       getPositioningStrategyName(t1000Data.PositionStrategy),
		Attributes: map[string]interface{}{
			"device_model": "SenseCAP T1000",
			"strategy_code": t1000Data.PositionStrategy,
		},
		Enabled:   true,
		Timestamp: timestamp,
	})

	// Motion Entity (binary sensor)
	entities = append(entities, components.Entity{
		UniqueID: components.GenerateUniqueID(model, devEUI, "motion"),
		EntityID: components.GenerateEntityID(
			"binary_sensor",
			orgSlug, "seeed", modelID, devEUI, "motion",
		),
		EntityType:  "binary_sensor",
		DeviceClass: "motion",
		Name:        "Motion",
		State:       t1000Data.EventStatus&EventStartMoving != 0 || t1000Data.EventStatus&EventEndMoving != 0,
		Attributes: map[string]interface{}{
			"device_model":     "SenseCAP T1000",
			"start_moving":     t1000Data.EventStatus&EventStartMoving != 0,
			"end_moving":       t1000Data.EventStatus&EventEndMoving != 0,
			"motionless":       t1000Data.EventStatus&EventMotionless != 0,
			"motion_segment":   t1000Data.MotionSegment,
		},
		Enabled:   true,
		Timestamp: timestamp,
	})

	// Shock Event Entity (binary sensor)
	entities = append(entities, components.Entity{
		UniqueID: components.GenerateUniqueID(model, devEUI, "shock_event"),
		EntityID: components.GenerateEntityID(
			"binary_sensor",
			orgSlug, "seeed", modelID, devEUI, "shock_event",
		),
		EntityType:  "binary_sensor",
		DeviceClass: "vibration",
		Name:        "Shock Event",
		State:       t1000Data.EventStatus&EventShock != 0,
		Attributes: map[string]interface{}{
			"device_model": "SenseCAP T1000",
		},
		Enabled:   true,
		Timestamp: timestamp,
	})

	// Temperature Event Entity (binary sensor)
	entities = append(entities, components.Entity{
		UniqueID: components.GenerateUniqueID(model, devEUI, "temperature_event"),
		EntityID: components.GenerateEntityID(
			"binary_sensor",
			orgSlug, "seeed", modelID, devEUI, "temperature_event",
		),
		EntityType:  "binary_sensor",
		DeviceClass: "heat",
		Name:        "Temperature Event",
		State:       t1000Data.EventStatus&EventTemperature != 0,
		Attributes: map[string]interface{}{
			"device_model": "SenseCAP T1000",
		},
		Enabled:   true,
		Timestamp: timestamp,
	})

	// Light Event Entity (binary sensor)
	entities = append(entities, components.Entity{
		UniqueID: components.GenerateUniqueID(model, devEUI, "light_event"),
		EntityID: components.GenerateEntityID(
			"binary_sensor",
			orgSlug, "seeed", modelID, devEUI, "light_event",
		),
		EntityType:  "binary_sensor",
		DeviceClass: "light",
		Name:        "Light Event",
		State:       t1000Data.EventStatus&EventLight != 0,
		Attributes: map[string]interface{}{
			"device_model": "SenseCAP T1000",
		},
		Enabled:   true,
		Timestamp: timestamp,
	})

	// SOS Alert Entity (binary sensor)
	entities = append(entities, components.Entity{
		UniqueID: components.GenerateUniqueID(model, devEUI, "sos_alert"),
		EntityID: components.GenerateEntityID(
			"binary_sensor",
			orgSlug, "seeed", modelID, devEUI, "sos_alert",
		),
		EntityType:  "binary_sensor",
		DeviceClass: "safety",
		Name:        "SOS Alert",
		State:       t1000Data.EventStatus&EventSOS != 0,
		Attributes: map[string]interface{}{
			"device_model": "SenseCAP T1000",
		},
		Enabled:   true,
		Timestamp: timestamp,
	})

	return entities, nil
}

// parseT1000Packet parses T1000 payload based on packet ID
func parseT1000Packet(data []byte) (*T1000Payload, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("payload too short")
	}

	packetID := data[0]

	switch packetID {
	case PacketDeviceStatusEventMode:
		return parseDeviceStatusEventMode(data)
	case PacketDeviceStatusPeriodicMode:
		return parseDeviceStatusPeriodicMode(data)
	case PacketHeartbeat:
		return parseHeartbeat(data)
	case PacketGNSSLocationSensor:
		return parseGNSSLocationSensor(data)
	case PacketWiFILocationSensor:
		return parseWiFILocationSensor(data)
	case PacketBluetoothLocationSensor:
		return parseBluetoothLocationSensor(data)
	case PacketGNSSLocationOnly:
		return parseGNSSLocationOnly(data)
	case PacketWiFILocationOnly:
		return parseWiFILocationOnly(data)
	case PacketBluetoothLocationOnly:
		return parseBluetoothLocationOnly(data)
	case PacketErrorCode:
		return parseErrorCode(data)
	case PacketPositioningStatusSensor:
		return parsePositioningStatusSensor(data)
	default:
		return nil, fmt.Errorf("unknown packet ID: 0x%02X", packetID)
	}
}

// parseDeviceStatusEventMode parses 0x01 packet (47 bytes)
func parseDeviceStatusEventMode(data []byte) (*T1000Payload, error) {
	if len(data) < 47 {
		return nil, fmt.Errorf("packet too short for device status event mode")
	}

	result := &T1000Payload{
		PacketID:         data[0],
		BatteryLevel:     data[1],
		WorkMode:         data[6],
		PositionStrategy: data[7],
		// Skip version and config bytes for simplicity
	}
	return result, nil
}

// parseDeviceStatusPeriodicMode parses 0x02 packet (16 bytes)
func parseDeviceStatusPeriodicMode(data []byte) (*T1000Payload, error) {
	if len(data) < 16 {
		return nil, fmt.Errorf("packet too short for device status periodic mode")
	}

	result := &T1000Payload{
		PacketID:         data[0],
		BatteryLevel:     data[1],
		WorkMode:         data[6],
		PositionStrategy: data[7],
	}
	return result, nil
}

// parseHeartbeat parses 0x05 packet (5 bytes)
func parseHeartbeat(data []byte) (*T1000Payload, error) {
	if len(data) < 5 {
		return nil, fmt.Errorf("packet too short for heartbeat")
	}

	result := &T1000Payload{
		PacketID:         data[0],
		BatteryLevel:     data[1],
		WorkMode:         data[2],
		PositionStrategy: data[3],
	}
	return result, nil
}

// parseGNSSLocationSensor parses 0x06 packet (22 bytes)
func parseGNSSLocationSensor(data []byte) (*T1000Payload, error) {
	if len(data) < 22 {
		return nil, fmt.Errorf("packet too short for GNSS location sensor")
	}

	result := &T1000Payload{
		PacketID:      data[0],
		EventStatus:   binary.BigEndian.Uint32(data[1:4]),
		MotionSegment: data[4],
		UTCTime:       binary.BigEndian.Uint32(data[5:9]),
		Longitude:     float64(int32(binary.BigEndian.Uint32(data[9:13]))) / 1000000.0,     // #nosec G115 
		Latitude:      float64(int32(binary.BigEndian.Uint32(data[13:17]))) / 1000000.0,      // #nosec G115
		Temperature:   float64(int16(binary.BigEndian.Uint16(data[17:19]))) / 10.0,          // #nosec G115
		Light:         binary.BigEndian.Uint16(data[19:21]),
		BatteryLevel:  data[21],
	}
	return result, nil
}

// parseWiFILocationSensor parses 0x07 packet (42 bytes, 4 MACs)
func parseWiFILocationSensor(data []byte) (*T1000Payload, error) {
	if len(data) < 42 {
		return nil, fmt.Errorf("packet too short for WiFi location sensor")
	}

	result := &T1000Payload{
		PacketID:      data[0],
		EventStatus:   binary.BigEndian.Uint32(data[1:4]),
		MotionSegment: data[4],
		UTCTime:       binary.BigEndian.Uint32(data[5:9]),
		Temperature:   float64(int16(binary.BigEndian.Uint16(data[36:38]))) / 10.0, // #nosec G115
		Light:         binary.BigEndian.Uint16(data[38:40]),
		BatteryLevel:  data[41],
	}

	// Parse 4 WiFi MAC addresses
	for i := 0; i < 4; i++ {
		offset := 9 + (i * 7)
		mac := fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
			data[offset], data[offset+1], data[offset+2],
			data[offset+3], data[offset+4], data[offset+5])
		rssi := int8(data[offset+6])
		result.WiFiMACs = append(result.WiFiMACs, WiFiMAC{MAC: mac, RSSI: rssi})
	}

	return result, nil
}

// parseBluetoothLocationSensor parses 0x08 packet (35 bytes, 3 BLE MACs)
func parseBluetoothLocationSensor(data []byte) (*T1000Payload, error) {
	if len(data) < 35 {
		return nil, fmt.Errorf("packet too short for Bluetooth location sensor")
	}

	result := &T1000Payload{
		PacketID:      data[0],
		EventStatus:   binary.BigEndian.Uint32(data[1:4]),
		MotionSegment: data[4],
		UTCTime:       binary.BigEndian.Uint32(data[5:9]),
		Temperature:   float64(int16(binary.BigEndian.Uint16(data[29:31]))) / 10.0, // #nosec G115
		Light:         binary.BigEndian.Uint16(data[31:33]),
		BatteryLevel:  data[34],
	}

	// Parse 3 BLE MAC addresses
	for i := 0; i < 3; i++ {
		offset := 9 + (i * 7)
		mac := fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
			data[offset], data[offset+1], data[offset+2],
			data[offset+3], data[offset+4], data[offset+5])
		rssi := int8(data[offset+6])
		result.BLEMACs = append(result.BLEMACs, BLEMAC{MAC: mac, RSSI: rssi})
	}

	return result, nil
}

// parseGNSSLocationOnly parses 0x09 packet (18 bytes)
func parseGNSSLocationOnly(data []byte) (*T1000Payload, error) {
	if len(data) < 18 {
		return nil, fmt.Errorf("packet too short for GNSS location only")
	}

	result := &T1000Payload{
		PacketID:      data[0],
		EventStatus:   binary.BigEndian.Uint32(data[1:4]),
		MotionSegment: data[4],
		UTCTime:       binary.BigEndian.Uint32(data[5:9]),
		Longitude:     float64(int32(binary.BigEndian.Uint32(data[9:13]))) / 1000000.0, // #nosec G115
		Latitude:      float64(int32(binary.BigEndian.Uint32(data[13:17]))) / 1000000.0,  // #nosec G115
		BatteryLevel:  data[17],
	}
	return result, nil
}

// parseWiFILocationOnly parses 0x0A packet (38 bytes, 4 MACs)
func parseWiFILocationOnly(data []byte) (*T1000Payload, error) {
	if len(data) < 38 {
		return nil, fmt.Errorf("packet too short for WiFi location only")
	}

	result := &T1000Payload{
		PacketID:      data[0],
		EventStatus:   binary.BigEndian.Uint32(data[1:4]),
		MotionSegment: data[4],
		UTCTime:       binary.BigEndian.Uint32(data[5:9]),
		BatteryLevel:  data[37],
	}

	// Parse 4 WiFi MAC addresses
	for i := 0; i < 4; i++ {
		offset := 9 + (i * 7)
		mac := fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
			data[offset], data[offset+1], data[offset+2],
			data[offset+3], data[offset+4], data[offset+5])
		rssi := int8(data[offset+6])
		result.WiFiMACs = append(result.WiFiMACs, WiFiMAC{MAC: mac, RSSI: rssi})
	}

	return result, nil
}

// parseBluetoothLocationOnly parses 0x0B packet (31 bytes, 3 BLE MACs)
func parseBluetoothLocationOnly(data []byte) (*T1000Payload, error) {
	if len(data) < 31 {
		return nil, fmt.Errorf("packet too short for Bluetooth location only")
	}

	result := &T1000Payload{
		PacketID:      data[0],
		EventStatus:   binary.BigEndian.Uint32(data[1:4]),
		MotionSegment: data[4],
		UTCTime:       binary.BigEndian.Uint32(data[5:9]),
		BatteryLevel:  data[30],
	}

	// Parse 3 BLE MAC addresses
	for i := 0; i < 3; i++ {
		offset := 9 + (i * 7)
		mac := fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
			data[offset], data[offset+1], data[offset+2],
			data[offset+3], data[offset+4], data[offset+5])
		rssi := int8(data[offset+6])
		result.BLEMACs = append(result.BLEMACs, BLEMAC{MAC: mac, RSSI: rssi})
	}

	return result, nil
}

// parseErrorCode parses 0x0D packet (5 bytes)
func parseErrorCode(data []byte) (*T1000Payload, error) {
	if len(data) < 5 {
		return nil, fmt.Errorf("packet too short for error code")
	}

	result := &T1000Payload{
		PacketID:  data[0],
		ErrorCode: binary.BigEndian.Uint32(data[1:5]),
	}
	return result, nil
}

// parsePositioningStatusSensor parses 0x11 packet (14 bytes)
func parsePositioningStatusSensor(data []byte) (*T1000Payload, error) {
	if len(data) < 14 {
		return nil, fmt.Errorf("packet too short for positioning status sensor")
	}

	result := &T1000Payload{
		PacketID:       data[0],
		PositionStatus: data[1],
		EventStatus:    binary.BigEndian.Uint32(data[2:5]),
		UTCTime:        binary.BigEndian.Uint32(data[5:9]),
		Temperature:    float64(int16(binary.BigEndian.Uint16(data[9:11]))) / 10.0, // #nosec G115
		Light:          binary.BigEndian.Uint16(data[11:13]),
		BatteryLevel:   data[13],
	}
	return result, nil
}

// Helper functions

func getWorkModeName(mode uint8) string {
	switch mode {
	case WorkModeStandby:
		return "Standby"
	case WorkModePeriodic:
		return "Periodic"
	case WorkModeEvent:
		return "Event"
	default:
		return fmt.Sprintf("Unknown (0x%02X)", mode)
	}
}

func getPositioningStrategyName(strategy uint8) string {
	switch strategy {
	case PosGNSS:
		return "GNSS Only"
	case PosWiFi:
		return "WiFi Only"
	case PosWiFiGNSS:
		return "WiFi + GNSS"
	case PosGNSSWiFi:
		return "GNSS + WiFi"
	case PosBLE:
		return "Bluetooth Only"
	case PosBLEWiFi:
		return "Bluetooth + WiFi"
	case PosBLEGNSS:
		return "Bluetooth + GNSS"
	case PosBLEWiFiGNSS:
		return "Bluetooth + WiFi + GNSS"
	default:
		return fmt.Sprintf("Unknown (0x%02X)", strategy)
	}
}

func getPositioningSource(strategy uint8) string {
	switch strategy {
	case PosGNSS:
		return "gnss"
	case PosWiFi:
		return "wifi"
	case PosWiFiGNSS, PosGNSSWiFi:
		return "wifi_gnss"
	case PosBLE:
		return "ble"
	case PosBLEWiFi:
		return "wifi_ble"
	case PosBLEGNSS:
		return "ble_gnss"
	case PosBLEWiFiGNSS:
		return "wifi_ble_gnss"
	default:
		return "unknown"
	}
}

// Extract payload data helper functions
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

func decodePayloadBytes(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, fmt.Errorf("empty payload data")
	}

	// Try hex decode first
	if decoded, err := hex.DecodeString(encoded); err == nil && len(decoded) > 0 {
		return decoded, nil
	}

	return nil, fmt.Errorf("failed to decode payload as hex")
}

// validateCoordinates validates that coordinates are reasonable
func validateCoordinates(lat, lon float64) error {
	if math.Abs(lat) > 90 || math.Abs(lon) > 180 {
		return fmt.Errorf("coordinates out of valid range")
	}
	if lat == 0 && lon == 0 {
		return fmt.Errorf("null island coordinates")
	}
	return nil
}
