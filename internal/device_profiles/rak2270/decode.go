package rak2270

import (
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

// Decode extracts sensor readings from a RAK2270 uplink.
// Binary format: Cayenne-like TLV with 4-byte records.
//
//	0x67: Temperature  (int16 BE / 10.0 °C)
//	0x02: Battery voltage (uint16 BE / 100.0 V)
func Decode(payload *common.RawPayload) map[string]interface{} {
	// Try to extract sensor readings from metadata first.
	sensors := extractMetadata(payload.Metadata)
	if len(sensors) > 0 {
		return sensors
	}

	// If metadata extraction didn't yield results, parse the raw binary payload.
	b := common.ExtractBytes(payload)
	if len(b) == 0 {
		return sensors
	}

	for i := 0; i+3 < len(b); {
		switch b[i+1] {
		case 0x67: // Temperature
			raw := int(b[i+2])<<8 | int(b[i+3])
			if raw > 0x7FFF {
				raw -= 0x10000
			}
			sensors["temperature"] = float64(raw) / 10.0
			i += 4
		case 0x02: // Battery voltage
			raw := int(b[i+2])<<8 | int(b[i+3])
			sensors["battery_voltage"] = float64(raw) / 100.0
			i += 4
		default:
			i++
		}
	}

	return sensors
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
		// Extract numeric sensor fields
		for _, field := range []string{"temperature", "battery_voltage"} {
			if _, exists := sensors[field]; !exists {
				if v, ok := src[field].(float64); ok {
					sensors[field] = v
				}
			}
		}
	}
	return sensors
}
