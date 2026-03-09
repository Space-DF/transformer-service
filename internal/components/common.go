package components

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
)

const (
	CoordScale = 10000000
)

// ExtractDevEUI extracts device EUI from various payload formats and normalizes it to lowercase
func ExtractDevEUI(metadata map[string]interface{}) string {
	if metadata == nil {
		return ""
	}

	// Try Helium format: top root level dev_eui
	for _, key := range []string{"device_eui", "dev_eui", "devEui", "deviceEui"} {
		if val, ok := metadata[key].(string); ok && val != "" {
			return strings.ToLower(val)
		}
	}

	// Try TTN format: end_device_ids.dev_eui
	if endDeviceIDs, ok := metadata["end_device_ids"].(map[string]interface{}); ok {
		if devEUI, ok := endDeviceIDs["dev_eui"].(string); ok {
			return strings.ToLower(devEUI)
		}
	}

	// Try ChirpStack format: deviceInfo.devEui
	if deviceInfo, ok := metadata["deviceInfo"].(map[string]interface{}); ok {
		if devEUI, ok := deviceInfo["devEui"].(string); ok && devEUI != "" {
			return strings.ToLower(devEUI)
		}
	}

	if decoded, ok := metadata["decoded_raw_data"].(map[string]interface{}); ok {
		if devInfo, ok := decoded["deviceInfo"].(map[string]interface{}); ok {
			if devEUI, ok := devInfo["devEui"].(string); ok && devEUI != "" {
				return strings.ToLower(devEUI)
			}
		}
	}

	return ""
}

// ValidateCoordinates validates that coordinates are reasonable
func ValidateCoordinates(lat, lon float64) error {
	if math.Abs(lat) > 90 || math.Abs(lon) > 180 {
		return fmt.Errorf("coordinates out of valid range")
	}
	if lat == 0 && lon == 0 {
		return fmt.Errorf("null island coordinates")
	}
	return nil
}

// Extract payload data
func ExtractPayloadData(payload interface{}) string {
	switch v := payload.(type) {
	case string:
		return v
	case map[string]interface{}:
		if uplink, ok := v["uplinkEvent"].(map[string]interface{}); ok {
			if data, ok := uplink["data"].(string); ok && data != "" {
				return data
			}
		}

		if uplink, ok := v["uplink_message"].(map[string]interface{}); ok {
			if frmPayload, ok := uplink["frm_payload"].(string); ok && frmPayload != "" {
				return frmPayload
			}
		}

		if decoded, ok := v["decoded_raw_data"].(map[string]interface{}); ok {
			// Try ChirpStack format: decoded_raw_data.uplinkEvent.data
			if uplink, ok := decoded["uplinkEvent"].(map[string]interface{}); ok {
				if data, ok := uplink["data"].(string); ok && data != "" {
					return data
				}
			}

			// Try TTN format: decoded_raw_data.uplink_message.frm_payload
			if uplinkMsg, ok := decoded["uplink_message"].(map[string]interface{}); ok {
				if frmPayload, ok := uplinkMsg["frm_payload"].(string); ok && frmPayload != "" {
					return frmPayload
				}
			}
		}
	}
	return ""
}

// DecodePayloadBytes decodes hex or base64 encoded payload string to bytes
func DecodePayloadBytes(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, fmt.Errorf("empty payload data")
	}

	// Try hex decode first
	if decoded, err := hex.DecodeString(encoded); err == nil && len(decoded) > 0 {
		return decoded, nil
	}

	// Try base64 decode
	if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil && len(decoded) > 0 {
		return decoded, nil
	}

	return nil, fmt.Errorf("failed to decode payload as hex or base64")
}

// ExtractFPort extracts the LoRaWAN fPort from metadata.
func ExtractFPort(metadata map[string]interface{}) int {
	// Try direct fPort field
	for _, key := range []string{"fPort", "f_port", "port", "fport"} {
		if v, ok := metadata[key]; ok {
			switch val := v.(type) {
			case float64:
				return int(val)
			case int:
				return val
			case int64:
				return int(val)
			}
		}
	}

	// Try ChirpStack format: uplinkEvent.fPort
	if uplinkEvent, ok := metadata["uplinkEvent"].(map[string]interface{}); ok {
		if fPort, ok := uplinkEvent["fPort"].(float64); ok {
			return int(fPort)
		}
	}

	// Try decoded_raw_data.uplinkEvent.fPort
	if decoded, ok := metadata["decoded_raw_data"].(map[string]interface{}); ok {
		if fPort, ok := decoded["fPort"].(float64); ok {
			return int(fPort)
		}
	}

	// Try TTN format: uplink_message.f_port
	if uplinkMsg, ok := metadata["uplink_message"].(map[string]interface{}); ok {
		if fPort, ok := uplinkMsg["f_port"].(float64); ok {
			return int(fPort)
		}
	}

	return 0
}
