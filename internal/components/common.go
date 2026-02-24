package components

import (
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