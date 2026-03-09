package seeed

import (
	"encoding/binary"
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
	PacketDeviceStatusEventMode    = 0x01
	PacketDeviceStatusPeriodicMode = 0x02
	PacketHeartbeat                = 0x05
	PacketGNSSLocationSensor       = 0x06
	PacketWiFILocationSensor       = 0x07
	PacketBluetoothLocationSensor  = 0x08
	PacketGNSSLocationOnly         = 0x09
	PacketWiFILocationOnly         = 0x0A
	PacketBluetoothLocationOnly    = 0x0B
	PacketErrorCode                = 0x0D
	PacketPositioningStatusSensor  = 0x11
)

// T1000Payload represents the parsed T1000 payload
type T1000Payload struct {
	PacketID         byte
	BatteryLevel     uint8   // Percentage (0-100)
	Temperature      float64 // Celsius
	Light            uint16  // Percentage (0-10000)
	Latitude         float64
	Longitude        float64
	Altitude         float64
	UTCTime          uint32 // Unix timestamp
	EventStatus      uint32 // Bit flags for events
	WorkMode         uint8
	PositionStrategy uint8
	WiFiMACs         []WiFiMAC
	BLEMACs          []BLEMAC
	MotionSegment    uint8
	PositionStatus   uint8
	ErrorCode        uint32

	// Device Status Event Mode fields (0x01 packet)
	SoftwareVersion                uint16
	HardwareVersion                uint16
	HeartbeatInterval              uint16 // seconds
	UplinkInterval                 uint16 // seconds
	EventModeUplinkInterval        uint16 // seconds
	TempLightSwitch                uint8
	SOSMode                        uint8
	EnableMotionEvent              uint8
	MotionThreshold                uint16
	MotionStartInterval            uint16 // seconds
	EnableMotionlessEvent          uint8
	MotionlessTimeout              uint16 // seconds
	EnableShockEvent               uint8
	ShockThreshold                 uint16
	EnableTemperatureEvent         uint8
	TemperatureEventUplinkInterval uint16 // seconds
	TemperatureSampleInterval      uint16 // seconds
	TemperatureThresholdMax        int16  // 0.1°C
	TemperatureThresholdMin        int16  // 0.1°C
	TemperatureWarningType         uint8
	EnableLightEvent               uint8
	LightEventUplinkInterval       uint16 // seconds
	LightSampleInterval            uint16 // seconds
	LightThresholdMax              uint16
	LightThresholdMin              uint16
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
	WorkModeStandby  = 0x00
	WorkModePeriodic = 0x01
	WorkModeEvent    = 0x02
)

// Positioning strategy values
const (
	PosGNSS        = 0x00
	PosWiFi        = 0x01
	PosWiFiGNSS    = 0x02
	PosGNSSWiFi    = 0x03
	PosBLE         = 0x04
	PosBLEWiFi     = 0x05
	PosBLEGNSS     = 0x06
	PosBLEWiFiGNSS = 0x07
)

// Event status bit flags
const (
	EventStartMoving = 0x000001
	EventEndMoving   = 0x000002
	EventMotionless  = 0x000004
	EventShock       = 0x000008
	EventTemperature = 0x000010
	EventLight       = 0x000020
	EventSOS         = 0x000040
	EventPressOnce   = 0x000080
)

// Positioning status values for 0x11 packet
const (
	PositionSuccess           = 0x00
	PositionGNSSFailed        = 0x01
	PositionWiFiFailed        = 0x02
	PositionWiFiGNSSFailed    = 0x03
	PositionGNSSWiFiFailed    = 0x04
	PositionBLEFailed         = 0x05
	PositionBLEWiFiFailed     = 0x06
	PositionBLEGNSSFailed     = 0x07
	PositionBLEWiFiGNSSFailed = 0x08
	PositionServerGNSSFailed  = 0x09
	PositionServerWiFiFailed  = 0x0A
	PositionServerBLEFailed   = 0x0B
	PositionPoorAccuracy      = 0x0C
	PositionTimeSyncFailed    = 0x0D
	PositionOldAlmanac        = 0x0E
)

// Scaling constants
const (
	TemperatureScale = 10.0  // Temperature is stored as int16 * 0.1°C
	LightMaxValue    = 10000 // Light sensor max value (0-100% range)
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
	encoded := components.ExtractPayloadData(payload.Data)
	if encoded == "" {
		encoded = components.ExtractPayloadData(payload.Metadata)
	}
	if encoded == "" {
		return nil, fmt.Errorf("no payload data found")
	}

	bytes, err := components.DecodePayloadBytes(encoded)
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

	// Determine which fields are present based on packet type
	hasBatteryLevel := packetTypeHasBattery(t1000Data.PacketID)
	hasTemperature := packetTypeHasTemperature(t1000Data.PacketID)
	hasLight := packetTypeHasLight(t1000Data.PacketID)
	hasEventStatus := packetTypeHasEventStatus(t1000Data.PacketID)
	hasWorkMode := packetTypeHasWorkMode(t1000Data.PacketID)
	hasPositionStrategy := packetTypeHasPositionStrategy(t1000Data.PacketID)
	hasPositionStatus := packetTypeHasPositionStatus(t1000Data.PacketID)
	hasErrorCode := packetTypeHasErrorCode(t1000Data.PacketID)
	hasVersions := packetTypeHasVersions(t1000Data.PacketID)

	// Software Version Entity - check if packet has version info
	if hasVersions && t1000Data.SoftwareVersion > 0 {
		entities = append(entities, components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "software_version"),
			EntityID: components.GenerateEntityID(
				"sensor",
				orgSlug, "seeed", modelID, devEUI, "software_version",
			),
			EntityType:  "sensor",
			DeviceClass: "firmware",
			Name:        "Software Version",
			State:       fmt.Sprintf("%d.%d", t1000Data.SoftwareVersion>>8, t1000Data.SoftwareVersion&0xFF),
			Attributes: map[string]interface{}{
				"device_model":  "SenseCAP T1000",
				"version_raw":   t1000Data.SoftwareVersion,
				"version_major": t1000Data.SoftwareVersion >> 8,
				"version_minor": t1000Data.SoftwareVersion & 0xFF,
			},
			Enabled:   true,
			Timestamp: timestamp,
		})
	}

	// Hardware Version Entity - check if packet has version info
	if hasVersions && t1000Data.HardwareVersion > 0 {
		entities = append(entities, components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "hardware_version"),
			EntityID: components.GenerateEntityID(
				"sensor",
				orgSlug, "seeed", modelID, devEUI, "hardware_version",
			),
			EntityType:  "sensor",
			DeviceClass: "firmware",
			Name:        "Hardware Version",
			State:       fmt.Sprintf("%d.%d", t1000Data.HardwareVersion>>8, t1000Data.HardwareVersion&0xFF),
			Attributes: map[string]interface{}{
				"device_model":  "SenseCAP T1000",
				"version_raw":   t1000Data.HardwareVersion,
				"version_major": t1000Data.HardwareVersion >> 8,
				"version_minor": t1000Data.HardwareVersion & 0xFF,
			},
			Enabled:   true,
			Timestamp: timestamp,
		})
	}

	// Position Status Entity - check if packet has positioning status
	if hasPositionStatus {
		entities = append(entities, components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "positioning_status"),
			EntityID: components.GenerateEntityID(
				"sensor",
				orgSlug, "seeed", modelID, devEUI, "positioning_status",
			),
			EntityType:  "sensor",
			DeviceClass: "positioning_status",
			Name:        "Positioning Status",
			State:       getPositioningStatusName(t1000Data.PositionStatus),
			Attributes: map[string]interface{}{
				"device_model":   "SenseCAP T1000",
				"status_code":    t1000Data.PositionStatus,
				"positioning_ok": t1000Data.PositionStatus == PositionSuccess,
			},
			Enabled:   true,
			Timestamp: timestamp,
		})
	}

	// Error Code Entity - check if packet has error code
	if hasErrorCode && t1000Data.ErrorCode > 0 {
		entities = append(entities, components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "error_code"),
			EntityID: components.GenerateEntityID(
				"sensor",
				orgSlug, "seeed", modelID, devEUI, "error_code",
			),
			EntityType:  "sensor",
			DeviceClass: "problem",
			Name:        "Error Code",
			State:       fmt.Sprintf("0x%08X", t1000Data.ErrorCode),
			Attributes: map[string]interface{}{
				"device_model": "SenseCAP T1000",
				"error_code":   t1000Data.ErrorCode,
			},
			Enabled:   true,
			Timestamp: timestamp,
		})
	}

	// Battery Entity - check if packet type has battery AND battery > 0
	if hasBatteryLevel && t1000Data.BatteryLevel > 0 {
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
	if hasTemperature && t1000Data.Temperature != math.MaxFloat64 {
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
	}

	// Light Entity - check if packet type has light
	if hasLight {
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
	}

	// Work Mode Entity - check if packet type has work mode
	if hasWorkMode {
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
	}

	// Positioning Strategy Entity - check if packet type has position strategy
	if hasPositionStrategy {
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
				"device_model":  "SenseCAP T1000",
				"strategy_code": t1000Data.PositionStrategy,
			},
			Enabled:   true,
			Timestamp: timestamp,
		})
	}

	// Event-based entities - only if packet type has event status
	if hasEventStatus {
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
				"device_model":   "SenseCAP T1000",
				"start_moving":   t1000Data.EventStatus&EventStartMoving != 0,
				"end_moving":     t1000Data.EventStatus&EventEndMoving != 0,
				"motionless":     t1000Data.EventStatus&EventMotionless != 0,
				"motion_segment": t1000Data.MotionSegment,
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

		// Press Once Event Entity (binary sensor)
		entities = append(entities, components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "press_once_event"),
			EntityID: components.GenerateEntityID(
				"binary_sensor",
				orgSlug, "seeed", modelID, devEUI, "press_once_event",
			),
			EntityType:  "binary_sensor",
			DeviceClass: "button",
			Name:        "Press Once Event",
			State:       t1000Data.EventStatus&EventPressOnce != 0,
			Attributes: map[string]interface{}{
				"device_model": "SenseCAP T1000",
			},
			Enabled:   true,
			Timestamp: timestamp,
		})
	}

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
// Byte 0: ID (0x01)
// Byte 1: battery level
// Bytes 2-3: software version (big-endian uint16)
// Bytes 4-5: hardware version (big-endian uint16)
// Byte 6: work mode
// Byte 7: positioning strategy
// Bytes 8-9: heartbeat interval (big-endian uint16)
// Bytes 10-11: uplink interval (big-endian uint16)
// Bytes 12-13: event mode uplink interval (big-endian uint16)
// Byte 14: temp & light switch
// Byte 15: SOS mode
// Byte 16: enable motion event
// Bytes 17-18: 3-axis motion threshold (big-endian uint16)
// Bytes 19-20: motion start interval (big-endian uint16)
// Byte 21: enable motionless event
// Bytes 22-23: motionless timeout (big-endian uint16)
// Byte 24: enable shock event
// Bytes 25-26: 3-axis shock threshold (big-endian uint16)
// Byte 27: enable temperature event
// Bytes 28-29: temperature event uplink interval (big-endian uint16)
// Bytes 30-31: temperature sample interval (big-endian uint16)
// Bytes 32-33: temperature threshold max (big-endian int16, 0.1°C)
// Bytes 34-35: temperature threshold min (big-endian int16, 0.1°C)
// Byte 36: temperature warning type
// Byte 37: enable light event
// Bytes 38-39: light event uplink interval (big-endian uint16)
// Bytes 40-41: light sample interval (big-endian uint16)
// Bytes 42-43: light threshold max (big-endian uint16)
// Bytes 44-45: light threshold min (big-endian uint16)
func parseDeviceStatusEventMode(data []byte) (*T1000Payload, error) {
	if len(data) < 47 {
		return nil, fmt.Errorf("packet too short for device status event mode")
	}

	result := &T1000Payload{
		PacketID:                       data[0],
		BatteryLevel:                   data[1],
		SoftwareVersion:                binary.BigEndian.Uint16(data[2:4]),
		HardwareVersion:                binary.BigEndian.Uint16(data[4:6]),
		WorkMode:                       data[6],
		PositionStrategy:               data[7],
		HeartbeatInterval:              binary.BigEndian.Uint16(data[8:10]),
		UplinkInterval:                 binary.BigEndian.Uint16(data[10:12]),
		EventModeUplinkInterval:        binary.BigEndian.Uint16(data[12:14]),
		TempLightSwitch:                data[14],
		SOSMode:                        data[15],
		EnableMotionEvent:              data[16],
		MotionThreshold:                binary.BigEndian.Uint16(data[17:19]),
		MotionStartInterval:            binary.BigEndian.Uint16(data[19:21]),
		EnableMotionlessEvent:          data[21],
		MotionlessTimeout:              binary.BigEndian.Uint16(data[22:24]),
		EnableShockEvent:               data[24],
		ShockThreshold:                 binary.BigEndian.Uint16(data[25:27]),
		EnableTemperatureEvent:         data[27],
		TemperatureEventUplinkInterval: binary.BigEndian.Uint16(data[28:30]),
		TemperatureSampleInterval:      binary.BigEndian.Uint16(data[30:32]),
		TemperatureThresholdMax:        int16(binary.BigEndian.Uint16(data[32:34])), // #nosec G115
		TemperatureThresholdMin:        int16(binary.BigEndian.Uint16(data[34:36])), // #nosec G115
		TemperatureWarningType:         data[36],
		EnableLightEvent:               data[37],
		LightEventUplinkInterval:       binary.BigEndian.Uint16(data[38:40]),
		LightSampleInterval:            binary.BigEndian.Uint16(data[40:42]),
		LightThresholdMax:              binary.BigEndian.Uint16(data[42:44]),
		LightThresholdMin:              binary.BigEndian.Uint16(data[44:46]),
	}
	return result, nil
}

// parseDeviceStatusPeriodicMode parses 0x02 packet (16 bytes)
// Byte 0: ID (0x02)
// Byte 1: battery level
// Bytes 2-3: software version (big-endian uint16)
// Bytes 4-5: hardware version (big-endian uint16)
// Byte 6: work mode
// Byte 7: positioning strategy
// Bytes 8-9: heartbeat interval (big-endian uint16)
// Bytes 10-11: uplink interval (big-endian uint16)
// Bytes 12-13: event mode uplink interval (big-endian uint16)
// Byte 14: temp & light switch
// Byte 15: SOS mode
func parseDeviceStatusPeriodicMode(data []byte) (*T1000Payload, error) {
	if len(data) < 16 {
		return nil, fmt.Errorf("packet too short for device status periodic mode: got %d bytes, need 16", len(data))
	}

	result := &T1000Payload{
		PacketID:                data[0],                              // Byte 0: 0x02
		BatteryLevel:            data[1],                              // Byte 1: battery level
		SoftwareVersion:         binary.BigEndian.Uint16(data[2:4]),   // Bytes 2-3: software version
		HardwareVersion:         binary.BigEndian.Uint16(data[4:6]),   // Bytes 4-5: hardware version
		WorkMode:                data[6],                              // Byte 6: work mode
		PositionStrategy:        data[7],                              // Byte 7: positioning strategy
		HeartbeatInterval:       binary.BigEndian.Uint16(data[8:10]),  // Bytes 8-9: heartbeat interval
		UplinkInterval:          binary.BigEndian.Uint16(data[10:12]), // Bytes 10-11: uplink interval
		EventModeUplinkInterval: binary.BigEndian.Uint16(data[12:14]), // Bytes 12-13: event mode uplink interval
		TempLightSwitch:         data[14],                             // Byte 14: temp & light switch
		SOSMode:                 data[15],                             // Byte 15: SOS mode
	}
	return result, nil
}

// parseHeartbeat parses 0x05 packet (5 bytes)
// Byte 0: ID (0x05)
// Byte 1: battery level
// Byte 2: work mode
// Byte 3: positioning strategy
// Byte 4: reserved (not used)
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

// parseGNSSLocationSensor parses 0x06 packet (21-22 bytes)
func parseGNSSLocationSensor(data []byte) (*T1000Payload, error) {
	minLen := 22
	if len(data) < minLen {
		return nil, fmt.Errorf("packet too short for GNSS location sensor: got %d bytes, need at least %d", len(data), minLen)
	}

	rawLat := int32(binary.BigEndian.Uint32(data[9:13]))  // #nosec G115
	rawLon := int32(binary.BigEndian.Uint32(data[13:17])) // #nosec G115

	lat := float64(rawLat) / components.CoordScale
	lon := float64(rawLon) / components.CoordScale

	if math.Abs(lat) > 90 || math.Abs(lon) > 180 {
		swappedLat := float64(rawLon) / components.CoordScale
		swappedLon := float64(rawLat) / components.CoordScale
		if math.Abs(swappedLat) <= 90 && math.Abs(swappedLon) <= 180 {
			lat = swappedLat
			lon = swappedLon
		}
	}

	result := &T1000Payload{
		PacketID: data[0],
		// EventStatus is uint24 (3 bytes) at bytes 1-3 (0-indexed)
		EventStatus:   uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3]),
		MotionSegment: data[4],
		UTCTime:       binary.BigEndian.Uint32(data[5:9]),
		Latitude:      lat,
		Longitude:     lon,
		Temperature:   float64(int16(binary.BigEndian.Uint16(data[17:19]))) / TemperatureScale, // #nosec G115
		Light:         binary.BigEndian.Uint16(data[19:21]),                                    // Already 0-100% per spec
		BatteryLevel:  data[21],
	}

	return result, nil
}

// parseWiFILocationSensor parses 0x07 packet (42 bytes, 4 MACs)
// Byte 0: ID (0x07)
// Bytes 1-3: event status (uint24)
// Byte 4: motion segment number
// Bytes 5-8: UTC time (big-endian uint32)
// Bytes 9-14: MAC address 1
// Byte 15: RSSI of MAC address 1 (int8)
// Bytes 16-21: MAC address 2
// Byte 22: RSSI of MAC address 2 (int8)
// Bytes 23-28: MAC address 3
// Byte 29: RSSI of MAC address 3 (int8)
// Bytes 30-35: MAC address 4
// Byte 36: RSSI of MAC address 4 (int8)
// Bytes 37-38: Temperature (big-endian int16, 0.1°C)
// Bytes 39-40: Light (big-endian uint16, 0-100%)
// Byte 41: battery level
func parseWiFILocationSensor(data []byte) (*T1000Payload, error) {
	if len(data) < 42 {
		return nil, fmt.Errorf("packet too short for WiFi location sensor: got %d bytes, need at least 42", len(data))
	}

	result := &T1000Payload{
		PacketID: data[0],
		// EventStatus is uint24 (3 bytes) at bytes 1-3 (0-indexed)
		EventStatus:   uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3]),
		MotionSegment: data[4],
		UTCTime:       binary.BigEndian.Uint32(data[5:9]),
		Temperature:   float64(int16(binary.BigEndian.Uint16(data[37:39]))) / TemperatureScale, // #nosec G115
		Light:         binary.BigEndian.Uint16(data[39:41]),                                    // Already 0-100% per spec
		BatteryLevel:  data[41],
	}

	// Parse 4 WiFi MAC addresses (each is 6 bytes MAC + 1 byte RSSI = 7 bytes)
	// Starting at byte 9 (after UTCTime which is bytes 5-8)
	for i := 0; i < 4; i++ {
		offset := 9 + (i * 7)
		mac := fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
			data[offset], data[offset+1], data[offset+2],
			data[offset+3], data[offset+4], data[offset+5])
		rssi := int8(data[offset+6]) //#nosec G115
		result.WiFiMACs = append(result.WiFiMACs, WiFiMAC{MAC: mac, RSSI: rssi})
	}

	return result, nil
}

// parseBluetoothLocationSensor parses 0x08 packet (35 bytes, 3 BLE MACs)
// Byte 0: ID (0x08)
// Bytes 1-3: event status (uint24)
// Byte 4: motion segment number
// Bytes 5-8: UTC time (big-endian uint32)
// Bytes 9-14: MAC address 1
// Byte 15: RSSI of MAC address 1 (int8)
// Bytes 16-21: MAC address 2
// Byte 22: RSSI of MAC address 2 (int8)
// Bytes 23-28: MAC address 3
// Byte 29: RSSI of MAC address 3 (int8)
// Bytes 30-31: Temperature (big-endian int16, 0.1°C)
// Bytes 32-33: Light (big-endian uint16, 0-100%)
// Byte 34: Battery level
func parseBluetoothLocationSensor(data []byte) (*T1000Payload, error) {
	if len(data) < 35 {
		return nil, fmt.Errorf("packet too short for Bluetooth location sensor: got %d bytes, need at least 35", len(data))
	}

	result := &T1000Payload{
		PacketID: data[0],
		// EventStatus is uint24 (3 bytes) at bytes 1-3 (0-indexed)
		EventStatus:   uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3]),
		MotionSegment: data[4],
		UTCTime:       binary.BigEndian.Uint32(data[5:9]),
		Temperature:   float64(int16(binary.BigEndian.Uint16(data[30:32]))) / TemperatureScale, // #nosec G115
		Light:         binary.BigEndian.Uint16(data[32:34]),                                    // Already 0-100% per spec
		BatteryLevel:  data[34],
	}

	// Parse 3 BLE MAC addresses (each is 6 bytes MAC + 1 byte RSSI = 7 bytes)
	// Starting at byte 9 (after UTCTime which is bytes 5-8)
	for i := 0; i < 3; i++ {
		offset := 9 + (i * 7)
		mac := fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
			data[offset], data[offset+1], data[offset+2],
			data[offset+3], data[offset+4], data[offset+5])
		rssi := int8(data[offset+6]) //#nosec G115
		result.BLEMACs = append(result.BLEMACs, BLEMAC{MAC: mac, RSSI: rssi})
	}

	return result, nil
}

// parseGNSSLocationOnly parses 0x09 packet (18 bytes)
// Byte 0: ID (0x09)
// Bytes 1-3: event status (uint24)
// Byte 4: motion segment number
// Bytes 5-8: UTC time (big-endian uint32)
// Bytes 9-12: longitude (big-endian int32, per spec - but device may swap)
// Bytes 13-16: latitude (big-endian int32, per spec - but device may swap)
// Byte 17: battery level
func parseGNSSLocationOnly(data []byte) (*T1000Payload, error) {
	if len(data) < 18 {
		return nil, fmt.Errorf("packet too short for GNSS location only: got %d bytes, need at least 18", len(data))
	}

	rawLat := int32(binary.BigEndian.Uint32(data[9:13]))  // #nosec G115
	rawLon := int32(binary.BigEndian.Uint32(data[13:17])) // #nosec G115

	lat := float64(rawLat) / components.CoordScale
	lon := float64(rawLon) / components.CoordScale

	// Check if lat/lon need to be swapped
	if math.Abs(lat) > 90 || math.Abs(lon) > 180 {
		swappedLat := float64(rawLon) / components.CoordScale
		swappedLon := float64(rawLat) / components.CoordScale
		if math.Abs(swappedLat) <= 90 && math.Abs(swappedLon) <= 180 {
			lat = swappedLat
			lon = swappedLon
		}
	}

	result := &T1000Payload{
		PacketID: data[0],
		// EventStatus is uint24 (3 bytes) at bytes 1-3 (0-indexed)
		EventStatus:   uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3]),
		MotionSegment: data[4],
		UTCTime:       binary.BigEndian.Uint32(data[5:9]),
		Latitude:      lat,
		Longitude:     lon,
		BatteryLevel:  data[17],
	}
	return result, nil
}

// parseWiFILocationOnly parses 0x0A packet (38 bytes, 4 MACs)
// Byte 0: ID (0x0A)
// Bytes 1-3: event status (uint24)
// Byte 4: motion segment number
// Bytes 5-8: UTC time (big-endian uint32)
// Bytes 9-14: MAC address 1
// Byte 15: RSSI of MAC address 1 (int8)
// Bytes 16-21: MAC address 2
// Byte 22: RSSI of MAC address 2 (int8)
// Bytes 23-28: MAC address 3
// Byte 29: RSSI of MAC address 3 (int8)
// Bytes 30-35: MAC address 4
// Byte 36: RSSI of MAC address 4 (int8)
// Byte 37: battery level
func parseWiFILocationOnly(data []byte) (*T1000Payload, error) {
	if len(data) < 38 {
		return nil, fmt.Errorf("packet too short for WiFi location only: got %d bytes, need at least 38", len(data))
	}

	result := &T1000Payload{
		PacketID: data[0],
		// EventStatus is uint24 (3 bytes) at bytes 1-3 (0-indexed)
		EventStatus:   uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3]),
		MotionSegment: data[4],
		UTCTime:       binary.BigEndian.Uint32(data[5:9]),
		BatteryLevel:  data[37],
	}

	// Parse 4 WiFi MAC addresses (each is 6 bytes MAC + 1 byte RSSI = 7 bytes)
	// Starting at byte 9 (after UTCTime which is bytes 5-8)
	for i := 0; i < 4; i++ {
		offset := 9 + (i * 7)
		mac := fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
			data[offset], data[offset+1], data[offset+2],
			data[offset+3], data[offset+4], data[offset+5])
		rssi := int8(data[offset+6]) //#nosec G115
		result.WiFiMACs = append(result.WiFiMACs, WiFiMAC{MAC: mac, RSSI: rssi})
	}

	return result, nil
}

// parseBluetoothLocationOnly parses 0x0B packet (31 bytes, 3 BLE MACs)
// Byte 0: ID (0x0B)
// Bytes 1-3: event status (uint24)
// Byte 4: motion segment number
// Bytes 5-8: UTC time (big-endian uint32)
// Bytes 9-14: MAC address 1
// Byte 15: RSSI of MAC address 1 (int8)
// Bytes 16-21: MAC address 2
// Byte 22: RSSI of MAC address 2 (int8)
// Bytes 23-28: MAC address 3
// Byte 29: RSSI of MAC address 3 (int8)
// Byte 30: battery level
func parseBluetoothLocationOnly(data []byte) (*T1000Payload, error) {
	if len(data) < 31 {
		return nil, fmt.Errorf("packet too short for Bluetooth location only: got %d bytes, need at least 31", len(data))
	}

	result := &T1000Payload{
		PacketID: data[0],
		// EventStatus is uint24 (3 bytes) at bytes 1-3 (0-indexed)
		EventStatus:   uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3]),
		MotionSegment: data[4],
		UTCTime:       binary.BigEndian.Uint32(data[5:9]),
		BatteryLevel:  data[30],
	}

	// Parse 3 BLE MAC addresses (each is 6 bytes MAC + 1 byte RSSI = 7 bytes)
	// Starting at byte 9 (after UTCTime which is bytes 5-8)
	for i := 0; i < 3; i++ {
		offset := 9 + (i * 7)
		mac := fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
			data[offset], data[offset+1], data[offset+2],
			data[offset+3], data[offset+4], data[offset+5])
		rssi := int8(data[offset+6]) //#nosec G115
		result.BLEMACs = append(result.BLEMACs, BLEMAC{MAC: mac, RSSI: rssi})
	}

	return result, nil
}

// parseErrorCode parses 0x0D packet (5 bytes)
// Byte 0: ID (0x0D)
// Bytes 1-4: error code (big-endian uint32)
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
// Byte 0: ID (0x11)
// Byte 1: positioning status
// Bytes 2-4: event status (uint24)
// Bytes 5-8: UTC timestamp (big-endian uint32)
// Bytes 9-10: temperature (big-endian int16, 0.1°C)
// Bytes 11-12: light (big-endian uint16, 0-100%)
// Byte 13: battery level
func parsePositioningStatusSensor(data []byte) (*T1000Payload, error) {
	if len(data) < 14 {
		return nil, fmt.Errorf("packet too short for positioning status sensor: got %d bytes, need at least 14", len(data))
	}

	result := &T1000Payload{
		PacketID:       data[0],
		PositionStatus: data[1],
		// EventStatus is uint24 (3 bytes) at bytes 2-4 (0-indexed)
		EventStatus:  uint32(data[2])<<16 | uint32(data[3])<<8 | uint32(data[4]),
		UTCTime:      binary.BigEndian.Uint32(data[5:9]),
		Temperature:  float64(int16(binary.BigEndian.Uint16(data[9:11]))) / TemperatureScale, // #nosec G115
		Light:        binary.BigEndian.Uint16(data[11:13]),                                   // Already 0-100% per spec
		BatteryLevel: data[13],
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

func getPositioningStatusName(status uint8) string {
	switch status {
	case PositionSuccess:
		return "Success"
	case PositionGNSSFailed:
		return "GNSS Timeout"
	case PositionWiFiFailed:
		return "WiFi Timeout"
	case PositionWiFiGNSSFailed:
		return "WiFi + GNSS Timeout"
	case PositionGNSSWiFiFailed:
		return "GNSS + WiFi Timeout"
	case PositionBLEFailed:
		return "Bluetooth Timeout"
	case PositionBLEWiFiFailed:
		return "Bluetooth + WiFi Timeout"
	case PositionBLEGNSSFailed:
		return "Bluetooth + GNSS Timeout"
	case PositionBLEWiFiGNSSFailed:
		return "Bluetooth + WiFi + GNSS Timeout"
	case PositionServerGNSSFailed:
		return "Server GNSS Parse Failed"
	case PositionServerWiFiFailed:
		return "Server WiFi Parse Failed"
	case PositionServerBLEFailed:
		return "Server Bluetooth Parse Failed"
	case PositionPoorAccuracy:
		return "Poor Accuracy"
	case PositionTimeSyncFailed:
		return "Time Sync Failed"
	case PositionOldAlmanac:
		return "Old Almanac"
	default:
		return fmt.Sprintf("Unknown (0x%02X)", status)
	}
}

// Helper functions to determine which fields are present based on packet type
func packetTypeHasBattery(packetID byte) bool {
	switch packetID {
	case PacketDeviceStatusEventMode, PacketDeviceStatusPeriodicMode, PacketHeartbeat,
		PacketGNSSLocationSensor, PacketWiFILocationSensor, PacketBluetoothLocationSensor,
		PacketGNSSLocationOnly, PacketWiFILocationOnly, PacketBluetoothLocationOnly,
		PacketPositioningStatusSensor:
		return true
	default:
		return false
	}
}

func packetTypeHasTemperature(packetID byte) bool {
	switch packetID {
	case PacketGNSSLocationSensor, PacketWiFILocationSensor, PacketBluetoothLocationSensor,
		PacketPositioningStatusSensor:
		return true
	default:
		return false
	}
}

func packetTypeHasLight(packetID byte) bool {
	switch packetID {
	case PacketGNSSLocationSensor, PacketWiFILocationSensor, PacketBluetoothLocationSensor,
		PacketPositioningStatusSensor:
		return true
	default:
		return false
	}
}

func packetTypeHasEventStatus(packetID byte) bool {
	switch packetID {
	case PacketGNSSLocationSensor, PacketWiFILocationSensor, PacketBluetoothLocationSensor,
		PacketGNSSLocationOnly, PacketWiFILocationOnly, PacketBluetoothLocationOnly,
		PacketPositioningStatusSensor:
		return true
	default:
		return false
	}
}

func packetTypeHasWorkMode(packetID byte) bool {
	switch packetID {
	case PacketDeviceStatusEventMode, PacketDeviceStatusPeriodicMode, PacketHeartbeat:
		return true
	default:
		return false
	}
}

func packetTypeHasPositionStrategy(packetID byte) bool {
	switch packetID {
	case PacketDeviceStatusEventMode, PacketDeviceStatusPeriodicMode, PacketHeartbeat:
		return true
	default:
		return false
	}
}

func packetTypeHasPositionStatus(packetID byte) bool {
	return packetID == PacketPositioningStatusSensor
}

func packetTypeHasErrorCode(packetID byte) bool {
	return packetID == PacketErrorCode
}

func packetTypeHasVersions(packetID byte) bool {
	switch packetID {
	case PacketDeviceStatusEventMode, PacketDeviceStatusPeriodicMode:
		return true
	default:
		return false
	}
}

// GetSupportedEntityTypes returns the entity types this device supports
func (c *T1000Parser) GetSupportedEntityTypes() []string {
	return []string{
		"location",
		"battery",
		"temperature",
		"light",
		"motion",
		"shock_event",
		"temperature_event",
		"light_event",
		"sos_alert",
		"work_mode",
		"positioning_strategy",
	}
}

// GetSupportedPorts returns the fPorts this device type uses
func (c *T1000Parser) GetSupportedPorts() []int {
	return []int{1, 5}
}

func (c *T1000Parser) SupportsGPS() bool {
	return true
}

// Parse converts raw payload into structured ParsedData
func (c *T1000Parser) ParsePayload(payload *components.RawPayload) (*components.ParsedData, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = components.ExtractDevEUI(payload.Metadata)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI not found")
	}

	// Decode payload bytes
	encoded := components.ExtractPayloadData(payload.Data)
	if encoded == "" {
		encoded = components.ExtractPayloadData(payload.Metadata)
	}
	if encoded == "" {
		return nil, fmt.Errorf("no payload data found")
	}

	bytes, err := components.DecodePayloadBytes(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	// Parse T1000 packet
	t1000Data, err := parseT1000Packet(bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse T1000 packet: %w", err)
	}

	sensorData := make(map[string]interface{})
	var location *components.Location

	// Add battery level
	if t1000Data.BatteryLevel > 0 {
		sensorData["battery_percent"] = float64(t1000Data.BatteryLevel)
	}

	// Add temperature
	sensorData["temperature"] = t1000Data.Temperature

	// Add light level
	sensorData["light_percent"] = float64(t1000Data.Light)

	// Add work mode
	sensorData["work_mode"] = getWorkModeName(t1000Data.WorkMode)

	// Add positioning strategy
	sensorData["positioning_strategy"] = getPositioningStrategyName(t1000Data.PositionStrategy)

	// Add event status
	sensorData["event_status"] = t1000Data.EventStatus
	sensorData["start_moving"] = t1000Data.EventStatus&EventStartMoving != 0
	sensorData["end_moving"] = t1000Data.EventStatus&EventEndMoving != 0
	sensorData["motionless"] = t1000Data.EventStatus&EventMotionless != 0
	sensorData["shock"] = t1000Data.EventStatus&EventShock != 0
	sensorData["temperature_event"] = t1000Data.EventStatus&EventTemperature != 0
	sensorData["light_event"] = t1000Data.EventStatus&EventLight != 0
	sensorData["sos"] = t1000Data.EventStatus&EventSOS != 0

	// Add location if available
	if t1000Data.Latitude != 0 || t1000Data.Longitude != 0 {
		location = &components.Location{
			Latitude:  t1000Data.Latitude,
			Longitude: t1000Data.Longitude,
			Altitude:  t1000Data.Altitude,
		}
		sensorData["latitude"] = t1000Data.Latitude
		sensorData["longitude"] = t1000Data.Longitude
		sensorData["position_source"] = getPositioningSource(t1000Data.PositionStrategy)
	}

	// Add WiFi MACs if available
	if len(t1000Data.WiFiMACs) > 0 {
		sensorData["wifi_mac_addresses"] = t1000Data.WiFiMACs
	}

	// Add BLE MACs if available
	if len(t1000Data.BLEMACs) > 0 {
		sensorData["ble_mac_addresses"] = t1000Data.BLEMACs
	}

	var batteryLevel *float64
	if t1000Data.BatteryLevel > 0 {
		batt := float64(t1000Data.BatteryLevel)
		batteryLevel = &batt
	}

	return &components.ParsedData{
		DeviceEUI:    devEUI,
		DeviceType:   DeviceTypeSenseCAP_T1000,
		Timestamp:    payload.Timestamp,
		Location:     location,
		SensorData:   sensorData,
		BatteryLevel: batteryLevel,
		RawData:      encoded,
	}, nil
}
