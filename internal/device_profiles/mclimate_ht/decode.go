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
	sensors := make(map[string]interface{})

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
		// This is a response message
		// TODO: Implement response parsing when response message format is finalized
		// For now, extract keepalive data from the last 6 bytes (always present in response messages)
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
	tempHighBits := uint16(bytes[1]&0x03) << 8
	tempLowBits := uint16(bytes[2])
	tempValue := int(tempHighBits | tempLowBits)
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
