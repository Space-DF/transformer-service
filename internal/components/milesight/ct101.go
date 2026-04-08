package milesight

import (
	"encoding/binary"
	"fmt"

	"github.com/Space-DF/transformer-service/internal/components"
)

// CT101Parser handles parsing of Milesight CT101 device payload
// Payload format: Channel-based TLV (Type-Length-Value)
// Each channel: channel_id (1 byte) + channel_type (1 byte) + data
type CT101Parser struct{}

// NewCT101Parser creates a new CT101 parser
func NewCT101Parser() *CT101Parser {
	return &CT101Parser{}
}

// ParsePayload parses CT101 device payload
func (p *CT101Parser) ParsePayload(payload *components.RawPayload) (*components.ParsedData, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = components.ExtractDevEUI(payload.Metadata, payload.LNSType)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI not found")
	}

	sensorData, err := p.decodeCT101Payload(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode sensor readings: %w", err)
	}

	return &components.ParsedData{
		DeviceEUI:  devEUI,
		DeviceType: DeviceTypeCT101,
		Timestamp:  payload.Timestamp,
		SensorData: sensorData,
		RawData:    payload.Data,
	}, nil
}

// SupportsGPS returns false since CT101 doesn't have built-in GPS
func (p *CT101Parser) SupportsGPS() bool {
	return false
}

// GetSupportedPorts returns the fPorts typically used by CT101
func (p *CT101Parser) GetSupportedPorts() []int {
	return []int{2}
}

// GetSupportedEntityTypes returns entity types supported by CT101
func (p *CT101Parser) GetSupportedEntityTypes() []string {
	return []string{"current", "total_current", "temperature", "current_alarm", "temperature_alarm"}
}

// ParseToEntities creates entities for CT101 device
func (p *CT101Parser) ParseToEntities(orgSlug, model string, payload *components.RawPayload, deviceLocation *components.Location) ([]components.Entity, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = components.ExtractDevEUI(payload.Metadata, payload.LNSType)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI is required")
	}

	sensorData, err := p.decodeCT101Payload(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode sensor readings: %w", err)
	}

	var entities []components.Entity
	timestamp := payload.Timestamp

	// Current Entity
	if current, ok := sensorData["current"].(float64); ok {
		currentEntity := components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "current"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("current"),
				orgSlug, "milesight", "ct101", devEUI, "current",
			),
			EntityType:  "current",
			DeviceClass: "current",
			Name:        "Current",
			DisplayType: []string{"chart", "gauge", "value"},
			State:       current,
			UnitOfMeas:  "A",
			Enabled:     true,
			Timestamp:   timestamp,
		}
		entities = append(entities, currentEntity)
	}

	// Total Current Entity (cumulative)
	if totalCurrent, ok := sensorData["total_current"].(float64); ok {
		totalCurrentEntity := components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "total_current"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("total_current"),
				orgSlug, "milesight", "ct101", devEUI, "total_current",
			),
			EntityType:  "total_current",
			DeviceClass: "total_current",
			Name:        "Total Current",
			DisplayType: []string{"chart", "value"},
			State:       totalCurrent,
			UnitOfMeas:  "A",
			Enabled:     true,
			Timestamp:   timestamp,
		}
		entities = append(entities, totalCurrentEntity)
	}

	// Temperature Entity
	if temp, ok := sensorData["temperature"].(float64); ok {
		tempEntity := components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "temperature"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("temperature"),
				orgSlug, "milesight", "ct101", devEUI, "temperature",
			),
			EntityType:  "temperature",
			DeviceClass: "temperature",
			Name:        "Temperature",
			DisplayType: []string{"chart", "gauge", "value"},
			State:       temp,
			UnitOfMeas:  "°C",
			Enabled:     true,
			Timestamp:   timestamp,
		}
		entities = append(entities, tempEntity)
	}

	// Current Alarm Status (binary sensor)
	if currentAlarm, ok := sensorData["current_alarm"].(map[string]interface{}); ok {
		alarmStatus := "off"
		if threshold, ok := currentAlarm["current_threshold_alarm"].(bool); ok && threshold {
			alarmStatus = "on"
		} else if overRange, ok := currentAlarm["current_over_range_alarm"].(bool); ok && overRange {
			alarmStatus = "on"
		}

		currentAlarmEntity := components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "current_alarm"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("motion"),
				orgSlug, "milesight", "ct101", devEUI, "current_alarm",
			),
			EntityType:  "current_alarm",
			DeviceClass: "problem",
			Name:        "Current Alarm",
			DisplayType: []string{"indicator"},
			State:       alarmStatus,
			Attributes:  currentAlarm,
			Enabled:     true,
			Timestamp:   timestamp,
		}
		entities = append(entities, currentAlarmEntity)
	}

	// Temperature Alarm Status (binary sensor)
	if tempAlarm, ok := sensorData["temperature_alarm"].(string); ok && tempAlarm != "" {
		alarmStatus := "off"
		if tempAlarm == "temperature threshold alarm" {
			alarmStatus = "on"
		}

		tempAlarmEntity := components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "temperature_alarm"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("motion"),
				orgSlug, "milesight", "ct101", devEUI, "temperature_alarm",
			),
			EntityType:  "temperature_alarm",
			DeviceClass: "problem",
			Name:        "Temperature Alarm",
			DisplayType: []string{"indicator"},
			State:       alarmStatus,
			Attributes: map[string]interface{}{
				"alarm_type": tempAlarm,
			},
			Enabled:   true,
			Timestamp: timestamp,
		}
		entities = append(entities, tempAlarmEntity)
	}

	// Current Sensor Status (if sensor has issues)
	if currentSensorStatus, ok := sensorData["current_sensor_status"].(string); ok && currentSensorStatus != "" {
		sensorStatusEntity := components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "current_sensor_status"),
			EntityID: components.GenerateEntityID(
				"sensor",
				orgSlug, "milesight", "ct101", devEUI, "current_sensor_status",
			),
			EntityType:  "sensor",
			DeviceClass: "problem",
			Name:        "Current Sensor Status",
			DisplayType: []string{"text"},
			State:       currentSensorStatus,
			Enabled:     true,
			Timestamp:   timestamp,
		}
		entities = append(entities, sensorStatusEntity)
	}

	// Temperature Sensor Status (if sensor has issues)
	if tempSensorStatus, ok := sensorData["temperature_sensor_status"].(string); ok && tempSensorStatus != "" {
		sensorStatusEntity := components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "temperature_sensor_status"),
			EntityID: components.GenerateEntityID(
				"sensor",
				orgSlug, "milesight", "ct101", devEUI, "temperature_sensor_status",
			),
			EntityType:  "sensor",
			DeviceClass: "problem",
			Name:        "Temperature Sensor Status",
			DisplayType: []string{"text"},
			State:       tempSensorStatus,
			Enabled:     true,
			Timestamp:   timestamp,
		}
		entities = append(entities, sensorStatusEntity)
	}

	// Add device metadata as attributes to first entity if available
	if len(entities) > 0 {
		metadata := make(map[string]interface{})
		if hwVersion, ok := sensorData["hardware_version"].(string); ok {
			metadata["hardware_version"] = hwVersion
		}
		if fwVersion, ok := sensorData["firmware_version"].(string); ok {
			metadata["firmware_version"] = fwVersion
		}
		if sn, ok := sensorData["serial_number"].(string); ok {
			metadata["serial_number"] = sn
		}
		if len(metadata) > 0 {
			if entities[0].Attributes == nil {
				entities[0].Attributes = make(map[string]interface{})
			}
			for k, v := range metadata {
				entities[0].Attributes[k] = v
			}
		}
	}

	return entities, nil
}

// decodeCT101Payload extracts and decodes CT101 sensor data from payload
func (p *CT101Parser) decodeCT101Payload(payload *components.RawPayload) (map[string]interface{}, error) {
	// Use LNS-aware payload extraction
	encoded := components.ExtractPayloadDataFromMetadata(payload.Metadata, payload.LNSType)

	if encoded == "" {
		return nil, fmt.Errorf("no payload data found")
	}

	// Decode base64 payload
	bytes, err := components.DecodePayloadBytes(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	data, err := p.decodeCT101Bytes(bytes)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// decodeCT101Bytes decodes CT101 sensor data from raw bytes
// CT101 payload format:
// Channel (1 byte) + Type (1 byte) + Data (variable)
func (p *CT101Parser) decodeCT101Bytes(bytes []byte) (map[string]interface{}, error) {
	if len(bytes) < 4 {
		return nil, fmt.Errorf("payload too short: %d bytes", len(bytes))
	}

	data := make(map[string]interface{})

	// Parse channel-type-value triplets
	i := 0
	for i < len(bytes) {
		if i+1 >= len(bytes) {
			break
		}

		channelID := bytes[i]
		channelType := bytes[i+1]

		// Combine channel ID and type into a single key for switch matching
		key := (uint16(channelID) << 8) | uint16(channelType)

		switch key {
		case 0xFF01: // IPSO VERSION
			if i+2 < len(bytes) {
				data["ipso_version"] = readProtocolVersion(bytes[i+2])
				i += 3
			}

		case 0xFF09: // HARDWARE VERSION
			if i+3 < len(bytes) {
				data["hardware_version"] = readHardwareVersion(bytes[i+2 : i+4])
				i += 4
			}

		case 0xFF0A: // FIRMWARE VERSION
			if i+3 < len(bytes) {
				data["firmware_version"] = readFirmwareVersion(bytes[i+2 : i+4])
				i += 4
			}

		case 0xFF16: // SERIAL NUMBER
			if i+9 < len(bytes) {
				data["serial_number"] = readSerialNumber(bytes[i+2 : i+10])
				i += 10
			}

		case 0x0397: // TOTAL CURRENT - 4 bytes UInt32LE / 100
			if i+5 < len(bytes) {
				value := readUInt32LE(bytes[i+2 : i+6])
				data["total_current"] = float64(value) / 100.0
				i += 6
			}

		case 0x0498: // CURRENT - 2 bytes UInt16LE / 100, or 0xFFFF for sensor status
			if i+3 < len(bytes) {
				value := readUInt16LE(bytes[i+2 : i+4])
				if value == 0xFFFF {
					data["current_sensor_status"] = "read failed"
				} else {
					data["current"] = float64(value) / 100.0
				}
				i += 4
			}

		case 0x0967: // TEMPERATURE - 2 bytes Int16LE / 10
			if i+3 < len(bytes) {
				value := readUInt16LE(bytes[i+2 : i+4])
				switch value {
				case 0xFFFD:
					data["temperature_sensor_status"] = "over range alarm"
				case 0xFFFF:
					data["temperature_sensor_status"] = "read failed"
				default:
					tempValue := readInt16LE(bytes[i+2 : i+4])
					data["temperature"] = float64(tempValue) / 10.0
				}
				i += 4
			}

		case 0x8498: // CURRENT ALARM - 7 bytes data + 2 bytes header = 9 total
			if i+8 < len(bytes) {
				data["current_max"] = float64(readUInt16LE(bytes[i+2:i+4])) / 100.0
				data["current_min"] = float64(readUInt16LE(bytes[i+4:i+6])) / 100.0
				data["current"] = float64(readUInt16LE(bytes[i+6:i+8])) / 100.0
				data["current_alarm"] = readCurrentAlarm(bytes[i+8])
				i += 9
			}

		case 0x8967: // TEMPERATURE ALARM - 3 bytes data + 2 bytes header = 5 total
			if i+4 < len(bytes) {
				tempValue := readInt16LE(bytes[i+2 : i+4])
				data["temperature"] = float64(tempValue) / 10.0
				data["temperature_alarm"] = readTemperatureAlarm(bytes[i+4])
				i += 5
			}

		default:
			// Unknown channel, stop parsing
			break
		}
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("no valid sensor data found in payload")
	}

	return data, nil
}

// Helper functions for reading binary data

func readProtocolVersion(b byte) string {
	major := (b & 0xF0) >> 4
	minor := b & 0x0F
	return fmt.Sprintf("v%d.%d", major, minor)
}

func readHardwareVersion(bytes []byte) string {
	major := fmt.Sprintf("%02X", bytes[0])
	minor := (bytes[1] & 0xF0) >> 4
	return fmt.Sprintf("v%s.%d", major, minor)
}

func readFirmwareVersion(bytes []byte) string {
	major := fmt.Sprintf("%02X", bytes[0])
	minor := fmt.Sprintf("%02X", bytes[1])
	return fmt.Sprintf("v%s.%s", major, minor)
}

func readSerialNumber(bytes []byte) string {
	result := ""
	for _, b := range bytes {
		result += fmt.Sprintf("%02X", b)
	}
	return result
}

func readUInt16LE(bytes []byte) uint16 {
	return binary.LittleEndian.Uint16(bytes)
}

func readInt16LE(bytes []byte) int16 {
	return int16(readUInt16LE(bytes))
}

func readUInt32LE(bytes []byte) uint32 {
	return binary.LittleEndian.Uint32(bytes)
}

func readCurrentAlarm(value byte) map[string]interface{} {
	alarm := make(map[string]interface{})

	// Bit 0: current_threshold_alarm
	alarm["current_threshold_alarm"] = (value>>0)&0x01 == 1
	// Bit 1: current_threshold_alarm_release
	alarm["current_threshold_alarm_release"] = (value>>1)&0x01 == 1
	// Bit 2: current_over_range_alarm
	alarm["current_over_range_alarm"] = (value>>2)&0x01 == 1
	// Bit 3: current_over_range_alarm_release
	alarm["current_over_range_alarm_release"] = (value>>3)&0x01 == 1

	return alarm
}

func readTemperatureAlarm(value byte) string {
	if value == 1 {
		return "temperature threshold alarm"
	}
	return "temperature threshold alarm release"
}
