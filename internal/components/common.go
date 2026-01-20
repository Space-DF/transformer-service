package components

// ExtractDevEUI extracts device EUI from various payload formats
func ExtractDevEUI(metadata map[string]interface{}) string {
	if metadata == nil {
		return ""
	}

	for _, key := range []string{"device_eui", "dev_eui", "devEui", "deviceEui"} {
		if val, ok := metadata[key].(string); ok && val != "" {
			return val
		}
	}

	if deviceInfo, ok := metadata["deviceInfo"].(map[string]interface{}); ok {
		if devEUI, ok := deviceInfo["devEui"].(string); ok && devEUI != "" {
			return devEUI
		}
	}

	if decoded, ok := metadata["decoded_raw_data"].(map[string]interface{}); ok {
		if devInfo, ok := decoded["deviceInfo"].(map[string]interface{}); ok {
			if devEUI, ok := devInfo["devEui"].(string); ok && devEUI != "" {
				return devEUI
			}
		}
	}

	return ""
}
