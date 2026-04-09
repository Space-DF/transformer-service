package mclimate_ht

import (
	"encoding/hex"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

const (
	keepaliveCommand = 0x51 // 81 - Keepalive message
)

// Decode extracts sensor readings from a Mclimate-HT uplink.
// Binary layout for keepalive (command byte = 0x51):
//
//	[0]     uint8  command (0x51 = keepalive)
//	[1]     uint8  flags + temp high bits
//	        bit 2: occupied flag
//	        bits 1:0: temperature high bits (bits 9:8)
//	[2]     uint8  temperature low bits (bits 7:0)
//	[3]     uint8  relative humidity (RH[%] = (XX * 100) / 256)
//	[4]     uint8  battery voltage (V = ((XX * 2200) / 255) + 1600) / 1000
//	[5]     uint8  PIR trigger count
//
// Temperature formula: t[°C] = (T[9:0] - 400) / 10
// T[9:0] is a 10-bit value with bits 9:8 in byte 1 (bits 1:0) and bits 7:0 in byte 2
func Decode(payload *common.RawPayload) map[string]interface{} {
	// Try to extract sensor readings from metadata first.
	sensors := extractMetadata(payload.Metadata)
	if len(sensors) > 0 {
		return sensors
	}

	// If metadata extraction didn't yield results, parse the raw binary payload.
	b := common.ExtractBytes(payload)
	if len(b) < 6 {
		return sensors
	}

	// Handle hex-ASCII encoded payload (some integrations store hex as ASCII)
	// Check if the payload looks like ASCII hex (all characters are valid hex)
	if isHexASCII(b) {
		if decoded, err := hex.DecodeString(string(b)); err == nil && len(decoded) >= 6 {
			b = decoded
		}
	}

	// Route the message based on the command byte
	if b[0] == keepaliveCommand {
		// This is a keepalive message
		handleKeepalive(b, sensors)
	} else {
		// This is a response message - process response commands first
		handleResponse(b, sensors)
		// For response messages, the last 6 bytes always contain keepalive data
		// (regardless of whether they start with 0x51)
		if len(b) >= 6 {
			keepaliveBytes := b[len(b)-6:]
			handleKeepalive(keepaliveBytes, sensors)
		}
	}

	return sensors
}

// handleKeepalive parses a keepalive message (command = 0x51).
func handleKeepalive(bytes []byte, data map[string]interface{}) {
	if len(bytes) < 6 {
		return
	}

	// Byte 1 bit 2: Occupied flag
	occupiedValue := (bytes[1] & 0x04) >> 2
	data["occupancy"] = occupiedValue == 1
	data["pir_sensor_value"] = occupiedValue
	data["pir_sensor_status"] = map[bool]string{true: "Motion detected", false: "No motion detected"}[occupiedValue == 1]

	// Byte 1 (bits 1:0) and Byte 2: Internal temperature sensor data
	// Formula: t[°C] = (T[9:0] - 400) / 10
	tempHighBits := (bytes[1] & 0x03) << 8
	tempLowBits := bytes[2]
	tempValue := int(tempHighBits) | int(tempLowBits)
	data["temperature"] = float64(tempValue-400) / 10.0

	// Byte 3: Relative Humidity data
	// Formula: RH[%] = (XX * 100) / 256
	data["humidity"] = float64(bytes[3]*100) / 256.0

	// Byte 4: Device battery voltage data
	// Battery voltage [V] = (((XX * 2200) / 255) + 1600) / 1000
	batteryVoltage := (float64(bytes[4])*2200.0/255.0 + 1600.0) / 1000.0
	data["battery_voltage"] = batteryVoltage
	data["battery"] = batteryPercentage(batteryVoltage)

	// Byte 5: PIR trigger count
	data["pir_trigger_count"] = bytes[5]
}

// handleResponse parses a response message and extracts configuration data.
func handleResponse(bytes []byte, data map[string]interface{}) {
	if len(bytes) < 1 {
		return
	}

	// Response messages have various command IDs at byte 0
	// The payload format is: [cmd][len][data...][cmd][len][data...]...
	// followed by status bytes and optionally keepalive data

	// For now, we focus on extracting keepalive data from response messages
	// which is handled by the caller. Configuration responses can be added here if needed.

	// Common response commands:
	// 0x04: Device version (hardware, software)
	// 0x12: Keep alive time
	// 0x19: Join retry period
	// 0x1b: Upllink type
	// 0x1d: Watch dog params
	// 0x2f: Uplink sending on button press
	// 0x3d: PIR sensor status
	// 0x3f: PIR sensor sensitivity
	// 0x49: PIR measurement period
	// 0x4b: PIR check period
	// 0x37: PIR sensor state
	// 0x39: Occupancy timeout
	// 0x4d: PIR blind period
	// 0xa4: Region
	// 0xa6: Crystal oscillator error
}

// batteryPercentage estimates battery percentage from voltage.
// Based on typical Li-SOCl2 battery discharge curve.
func batteryPercentage(voltage float64) float64 {
	// Typical range: 3.6V (full) to 2.0V (empty)
	// This is a rough approximation
	if voltage >= 3.6 {
		return 100.0
	}
	if voltage <= 2.0 {
		return 0.0
	}
	return ((voltage - 2.0) / 1.6) * 100.0
}

// extractMetadata extracts sensor readings from metadata.
func extractMetadata(meta map[string]interface{}) map[string]interface{} {
	sensors := make(map[string]interface{})
	// Check both possible metadata keys
	for _, key := range []string{"decoded_payload", "object"} {
		src, ok := meta[key].(map[string]interface{})
		if !ok {
			continue
		}
		// Extract sensor fields
		for _, field := range []string{
			"temperature", "humidity", "battery_voltage", "battery",
			"occupancy", "pir_trigger_count", "pir_sensor_value",
		} {
			if _, exists := sensors[field]; !exists {
				if v, ok := src[field].(float64); ok {
					sensors[field] = v
				} else if v, ok := src[field].(bool); ok {
					sensors[field] = v
				}
			}
		}
	}
	return sensors
}

// isHexASCII checks if the byte slice contains only ASCII hex characters (0-9, a-f, A-F)
// and has an even length (valid for hex decoding).
func isHexASCII(b []byte) bool {
	if len(b) == 0 || len(b)%2 != 0 {
		return false
	}
	for _, c := range b {
		// Check if character is a valid hex digit
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
