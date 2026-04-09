package ct101

import (
	"fmt"
	"strings"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

// Decode extracts sensor readings from a Milesight CT101 uplink.
// Payload format: Channel-based TLV (Type-Length-Value)
// Each channel: channel_id (1 byte) + channel_type (1 byte) + data
func Decode(payload *common.RawPayload) map[string]any {
	// Try to extract sensor readings from metadata first
	if sensors := extractMetadata(payload.Metadata); len(sensors) > 0 {
		return sensors
	}

	// Parse the raw binary payload
	b := common.ExtractBytes(payload)
	if len(b) < 4 {
		return make(map[string]any)
	}

	return decodeCT101Bytes(b)
}

// extractMetadata extracts sensor readings from metadata.
func extractMetadata(meta map[string]any) map[string]any {
	sensors := make(map[string]any)

	// Check both possible metadata keys
	for _, key := range []string{"decoded_payload", "object"} {
		src, ok := meta[key].(map[string]any)
		if !ok {
			continue
		}

		// Extract all relevant sensor fields
		for _, field := range []string{
			"current", "total_current", "temperature",
			"current_max", "current_min",
			"hardware_version", "firmware_version", "serial_number",
			"current_sensor_status", "temperature_sensor_status",
		} {
			if _, exists := sensors[field]; !exists {
				if v, ok := src[field]; ok {
					sensors[field] = v
				}
			}
		}

		// Handle current_alarm (nested map) - support both any and interface{} for compatibility
		if v, ok := src["current_alarm"].(map[string]any); ok {
			sensors["current_alarm"] = v
		} else if v, ok := src["current_alarm"].(map[string]interface{}); ok {
			// Convert interface{} to any for consistency
			converted := make(map[string]any, len(v))
			for k, val := range v {
				converted[k] = val
			}
			sensors["current_alarm"] = converted
		}

		// Handle temperature_alarm (string)
		if v, ok := src["temperature_alarm"].(string); ok {
			sensors["temperature_alarm"] = v
		}
	}

	return sensors
}

// decodeCT101Bytes decodes CT101 sensor data from raw bytes.
// CT101 payload format:
// Channel (1 byte) + Type (1 byte) + Data (variable)
func decodeCT101Bytes(bytes []byte) map[string]any {
	if len(bytes) < 4 {
		return make(map[string]any)
	}

	data := make(map[string]any)

	// Parse channel-type-value triplets
	i := 0
	for i < len(bytes) {
		if i+1 >= len(bytes) {
			return data
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
				value := common.U32LE(bytes, i+2)
				data["total_current"] = float64(value) / 100.0
				i += 6
			}

		case 0x0498: // CURRENT - 2 bytes UInt16LE / 100, or 0xFFFF for sensor status
			if i+3 < len(bytes) {
				value := common.U16LE(bytes, i+2)
				if value == 0xFFFF {
					data["current_sensor_status"] = "read failed"
				} else {
					data["current"] = float64(value) / 100.0
				}
				i += 4
			}

		case 0x0967: // TEMPERATURE - 2 bytes Int16LE / 10
			if i+3 < len(bytes) {
				value := common.U16LE(bytes, i+2)
				switch value {
				case 0xFFFD:
					data["temperature_sensor_status"] = "over range alarm"
				case 0xFFFF:
					data["temperature_sensor_status"] = "read failed"
				default:
					tempValue := common.I16LE(bytes, i+2)
					data["temperature"] = float64(tempValue) / 10.0
				}
				i += 4
			}

		case 0x8498: // CURRENT ALARM - 7 bytes data + 2 bytes header = 9 total
			if i+8 < len(bytes) {
				data["current_max"] = float64(common.U16LE(bytes, i+2)) / 100.0
				data["current_min"] = float64(common.U16LE(bytes, i+4)) / 100.0
				data["current"] = float64(common.U16LE(bytes, i+6)) / 100.0
				data["current_alarm"] = readCurrentAlarm(bytes[i+8])
				i += 9
			}

		case 0x8967: // TEMPERATURE ALARM - 3 bytes data + 2 bytes header = 5 total
			if i+4 < len(bytes) {
				tempValue := common.I16LE(bytes, i+2)
				data["temperature"] = float64(tempValue) / 10.0
				data["temperature_alarm"] = readTemperatureAlarm(bytes[i+4])
				i += 5
			}

		default:
			// Unknown channel, stop parsing
			return data
		}
	}

	return data
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
	var sb strings.Builder
	sb.Grow(len(bytes) * 2)
	for _, b := range bytes {
		fmt.Fprintf(&sb, "%02X", b)
	}
	return sb.String()
}

func readCurrentAlarm(value byte) map[string]any {
	alarm := make(map[string]any)

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
