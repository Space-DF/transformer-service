package am307

import (
	"encoding/binary"
	"encoding/hex"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

// Channel and Type constants for AM307 TLV format
const (
	// Channel IDs
	ChannelBattery            = 0x01
	ChannelTemperature        = 0x03
	ChannelHumidity           = 0x04
	ChannelPIR                = 0x05
	ChannelLight              = 0x06
	ChannelCO2                = 0x07
	ChannelTVOC               = 0x08
	ChannelPressure           = 0x09
	ChannelHistory            = 0x20
	ChannelHistoryTVOCug      = 0x21
	ChannelResponse           = 0xfe
	ChannelSystem             = 0xff

	// Channel Types
	TypeBattery           = 0x75
	TypeTemperature       = 0x67
	TypeHumidity          = 0x68
	TypePIR               = 0x00
	TypeLight             = 0xcb
	TypeCO2               = 0x7d
	TypeTVOCiaq           = 0x7d
	TypeTVOCug            = 0xe6
	TypePressure          = 0x73
	TypeHistory           = 0xce
	TypeProtocolVersion   = 0x01
	TypeHardwareVersion   = 0x09
	TypeFirmwareVersion   = 0x0a
	TypeDeviceStatus      = 0x0b
	TypeLoRaWANClass      = 0x0f
	TypeSerialNumber      = 0x16
	TypeTSLVersion        = 0xff
)

// Decode extracts sensor readings from a Milesight AM307 uplink.
// The payload uses a TLV (Type-Length-Value) format:
//   [channel_id][channel_type][data...]
//
// Example payload structure:
//   0x01 0x75 0x64           - Battery: 100%
//   0x03 0x67 0x38 0x0b      - Temperature: 28.72°C (int16LE / 10)
//   0x04 0x68 0x5a           - Humidity: 45% (value / 2)
//   0x05 0x00 0x01           - PIR: trigger (0=idle, 1=trigger)
//   0x06 0xcb 0x32           - Light level: 50
//   0x07 0x7d 0x20 0x03      - CO2: 800 ppm (uint16LE)
//   0x08 0x7d 0x10 0x00      - TVOC: 0.16 iaq (uint16LE / 100)
//   0x09 0x73 0x70 0x1f      - Pressure: 8055.2 hPa (uint16LE / 10)
func Decode(payload *common.RawPayload) map[string]interface{} {
	// Try to extract sensor readings from metadata first
	sensors := extractMetadata(payload.Metadata)
	if len(sensors) > 0 {
		return sensors
	}

	// Parse the raw binary payload
	b := common.ExtractBytes(payload)
	if len(b) == 0 {
		return sensors
	}

	// Handle hex-ASCII encoded payload (some integrations store hex as ASCII)
	if isHexASCII(b) {
		if decoded, err := hex.DecodeString(string(b)); err == nil && len(decoded) > 0 {
			b = decoded
		}
	}

	// Parse TLV format
	i := 0
	for i < len(b) {
		if i+2 > len(b) {
			break
		}

		channelID := b[i]
		channelType := b[i+1]
		i += 2

		// Parse based on channel ID and type
		var consumed int
		switch channelID {
		case ChannelBattery:
			if channelType == TypeBattery {
				consumed = parseBattery(b[i:], sensors)
			}
		case ChannelTemperature:
			if channelType == TypeTemperature {
				consumed = parseTemperature(b[i:], sensors)
			}
		case ChannelHumidity:
			if channelType == TypeHumidity {
				consumed = parseHumidity(b[i:], sensors)
			}
		case ChannelPIR:
			if channelType == TypePIR {
				consumed = parsePIR(b[i:], sensors)
			}
		case ChannelLight:
			if channelType == TypeLight {
				consumed = parseLight(b[i:], sensors)
			}
		case ChannelCO2:
			if channelType == TypeCO2 {
				consumed = parseCO2(b[i:], sensors)
			}
		case ChannelTVOC:
			if channelType == TypeTVOCiaq {
				consumed = parseTVOCiaq(b[i:], sensors)
			} else if channelType == TypeTVOCug {
				consumed = parseTVOCug(b[i:], sensors)
			}
		case ChannelPressure:
			if channelType == TypePressure {
				consumed = parsePressure(b[i:], sensors)
			}
		case ChannelHistory, ChannelHistoryTVOCug:
			if channelType == TypeHistory {
				consumed = parseHistory(b[i:], sensors, channelID == ChannelHistoryTVOCug)
			}
		case ChannelSystem, ChannelResponse:
			// System and response channels - skip for now
			// These contain version info, device status, and downlink responses
			consumed = skipSystemData(channelType, b[i:])
		}

		if consumed <= 0 {
			// Unknown channel or insufficient data, stop parsing
			break
		}
		i += consumed
	}

	return sensors
}

// parseBattery parses battery percentage (1 byte)
func parseBattery(b []byte, data map[string]interface{}) int {
	if len(b) < 1 {
		return 0
	}
	data["battery"] = float64(b[0])
	return 1
}

// parseTemperature parses temperature (2 bytes, int16LE / 10)
func parseTemperature(b []byte, data map[string]interface{}) int {
	if len(b) < 2 {
		return 0
	}
	value := int16(binary.LittleEndian.Uint16(b[:2]))
	data["temperature"] = float64(value) / 10.0
	return 2
}

// parseHumidity parses humidity (1 byte / 2)
func parseHumidity(b []byte, data map[string]interface{}) int {
	if len(b) < 1 {
		return 0
	}
	data["humidity"] = float64(b[0]) / 2.0
	return 1
}

// parsePIR parses PIR status (1 byte: 0=idle, 1=trigger)
func parsePIR(b []byte, data map[string]interface{}) int {
	if len(b) < 1 {
		return 0
	}
	value := b[0]
	data["pir_sensor_value"] = int(value)
	data["occupancy"] = value == 1
	status := "idle"
	if value == 1 {
		status = "trigger"
	}
	data["pir_sensor_status"] = status
	return 1
}

// parseLight parses light level (1 byte)
func parseLight(b []byte, data map[string]interface{}) int {
	if len(b) < 1 {
		return 0
	}
	data["light_level"] = float64(b[0])
	return 1
}

// parseCO2 parses CO2 concentration (2 bytes, uint16LE, in ppm)
func parseCO2(b []byte, data map[string]interface{}) int {
	if len(b) < 2 {
		return 0
	}
	data["co2"] = float64(binary.LittleEndian.Uint16(b[:2]))
	return 2
}

// parseTVOCiaq parses TVOC in iaq units (2 bytes, uint16LE / 100)
func parseTVOCiaq(b []byte, data map[string]interface{}) int {
	if len(b) < 2 {
		return 0
	}
	data["tvoc"] = float64(binary.LittleEndian.Uint16(b[:2])) / 100.0
	data["tvoc_unit"] = "iaq"
	return 2
}

// parseTVOCug parses TVOC in µg/m³ (2 bytes, uint16LE)
func parseTVOCug(b []byte, data map[string]interface{}) int {
	if len(b) < 2 {
		return 0
	}
	data["tvoc"] = float64(binary.LittleEndian.Uint16(b[:2]))
	data["tvoc_unit"] = "ugm3"
	return 2
}

// parsePressure parses barometric pressure (2 bytes, uint16LE / 10, in hPa)
func parsePressure(b []byte, data map[string]interface{}) int {
	if len(b) < 2 {
		return 0
	}
	data["pressure"] = float64(binary.LittleEndian.Uint16(b[:2])) / 10.0
	return 2
}

// parseHistory parses historical data (16 bytes)
// Format: [timestamp(4)][temp(2)][humidity(2)][pir(1)][light(1)][co2(2)][tvoc(2)][pressure(2)]
func parseHistory(b []byte, data map[string]interface{}, tvocInUgm3 bool) int {
	if len(b) < 16 {
		return 0
	}

	history := map[string]interface{}{
		"timestamp": float64(binary.LittleEndian.Uint32(b[:4])),
	}

	// Temperature (int16LE / 10)
	tempValue := int16(binary.LittleEndian.Uint16(b[4:6]))
	history["temperature"] = float64(tempValue) / 10.0

	// Humidity (uint16LE / 2)
	history["humidity"] = float64(binary.LittleEndian.Uint16(b[6:8])) / 2.0

	// PIR status
	pirValue := b[8]
	history["pir"] = pirValue == 1
	history["pir_status"] = map[uint8]string{0: "idle", 1: "trigger"}[pirValue]

	// Light level
	history["light_level"] = float64(b[9])

	// CO2 (uint16LE)
	history["co2"] = float64(binary.LittleEndian.Uint16(b[10:12]))

	// TVOC
	tvocValue := float64(binary.LittleEndian.Uint16(b[12:14]))
	if tvocInUgm3 {
		history["tvoc"] = tvocValue
		history["tvoc_unit"] = "ugm3"
	} else {
		history["tvoc"] = tvocValue / 100.0
		history["tvoc_unit"] = "iaq"
	}

	// Pressure (uint16LE / 10)
	history["pressure"] = float64(binary.LittleEndian.Uint16(b[14:16])) / 10.0

	// Store history array
	if _, exists := data["history"]; !exists {
		data["history"] = []map[string]interface{}{}
	}
	data["history"] = append(data["history"].([]map[string]interface{}), history)

	return 16
}

// skipSystemData skips system/response channel data
func skipSystemData(channelType byte, b []byte) int {
	switch channelType {
	case TypeProtocolVersion:
		if len(b) >= 1 {
			return 1
		}
	case TypeHardwareVersion, TypeFirmwareVersion:
		if len(b) >= 2 {
			return 2
		}
	case TypeDeviceStatus, TypeLoRaWANClass:
		if len(b) >= 1 {
			return 1
		}
	case TypeSerialNumber:
		if len(b) >= 8 {
			return 8
		}
	case TypeTSLVersion:
		if len(b) >= 2 {
			return 2
		}
	}
	return 0
}

// extractMetadata extracts sensor readings from metadata
func extractMetadata(meta map[string]interface{}) map[string]interface{} {
	sensors := make(map[string]interface{})
	for _, key := range []string{"decoded_payload", "object"} {
		src, ok := meta[key].(map[string]interface{})
		if !ok {
			continue
		}
		for _, field := range []string{
			"temperature", "humidity", "battery", "battery_voltage",
			"occupancy", "pir_sensor_value", "pir_sensor_status",
			"light_level", "co2", "tvoc", "tvoc_unit", "pressure",
		} {
			if _, exists := sensors[field]; !exists {
				if v, ok := src[field].(float64); ok {
					sensors[field] = v
				} else if v, ok := src[field].(bool); ok {
					sensors[field] = v
				} else if v, ok := src[field].(string); ok {
					sensors[field] = v
				}
			}
		}
	}
	return sensors
}

// isHexASCII checks if the byte slice contains only ASCII hex characters
func isHexASCII(b []byte) bool {
	if len(b) == 0 || len(b)%2 != 0 {
		return false
	}
	for _, c := range b {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
