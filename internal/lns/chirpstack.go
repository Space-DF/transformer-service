package lns

import (
	"encoding/base64"
	"fmt"
)

// ChirpStackHandler handles data from ChirpStack webhook payloads
// ChirpStack payload structure:
//
//	{
//	  "deviceInfo": {"devEui": "..."},
//	  "uplinkEvent": {
//	    "fPort": 1,
//	    "fCnt": 123,
//	    "data": "...",
//	    "txInfo": {"frequency": 868300000},
//	    "rxInfo": [...]
//	  }
//	}
type ChirpStackHandler struct{}

func (h *ChirpStackHandler) Name() string {
	return "chirpstack"
}

// ExtractDevEUI extracts DevEUI from ChirpStack payload
// Location: deviceInfo.devEui or uplinkEvent.deviceInfo.devEui
func (h *ChirpStackHandler) ExtractDevEUI(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}

	// Try direct deviceInfo.devEui
	if deviceInfo, ok := payload["deviceInfo"].(map[string]interface{}); ok {
		if devEUI, ok := deviceInfo["devEui"].(string); ok && devEUI != "" {
			return devEUI
		}
	}

	// Try nested in decoded_raw_data
	if decoded, ok := payload["decoded_raw_data"].(map[string]interface{}); ok {
		return h.ExtractDevEUI(decoded)
	}

	// Try uplinkEvent.deviceInfo.devEui
	if uplinkEvent, ok := payload["uplinkEvent"].(map[string]interface{}); ok {
		if deviceInfo, ok := uplinkEvent["deviceInfo"].(map[string]interface{}); ok {
			if devEUI, ok := deviceInfo["devEui"].(string); ok && devEUI != "" {
				return devEUI
			}
		}
	}

	return ""
}

// ExtractFPort extracts fPort from ChirpStack payload
// Location: fPort at root or uplinkEvent.fPort
func (h *ChirpStackHandler) ExtractFPort(payload map[string]interface{}) int {
	if payload == nil {
		return 0
	}

	// Try direct fPort at root level (ChirpStack HTTP webhook format)
	if fPort, ok := payload["fPort"].(float64); ok {
		return int(fPort)
	}

	// Try uplinkEvent.fPort (ChirpStack MQTT format)
	if uplinkEvent, ok := payload["uplinkEvent"].(map[string]interface{}); ok {
		if fPort, ok := uplinkEvent["fPort"].(float64); ok {
			return int(fPort)
		}
	}

	// Try nested in decoded_raw_data
	if decoded, ok := payload["decoded_raw_data"].(map[string]interface{}); ok {
		return h.ExtractFPort(decoded)
	}

	return 0
}

// ExtractFrequency extracts frequency from ChirpStack payload
// Location: txInfo.frequency at root or uplinkEvent.txInfo.frequency
func (h *ChirpStackHandler) ExtractFrequency(payload map[string]interface{}) (float64, error) {
	if payload == nil {
		return 0, fmt.Errorf("payload is nil")
	}

	// Try direct txInfo.frequency at root level (ChirpStack HTTP webhook format)
	if txInfo, ok := payload["txInfo"].(map[string]interface{}); ok {
		if freq, ok := txInfo["frequency"].(float64); ok {
			return freq, nil
		}
	}

	// Try uplinkEvent.txInfo.frequency (ChirpStack MQTT format)
	if uplinkEvent, ok := payload["uplinkEvent"].(map[string]interface{}); ok {
		if txInfo, ok := uplinkEvent["txInfo"].(map[string]interface{}); ok {
			if freq, ok := txInfo["frequency"].(float64); ok {
				return freq, nil
			}
		}
	}

	// Try nested in decoded_raw_data
	if decoded, ok := payload["decoded_raw_data"].(map[string]interface{}); ok {
		return h.ExtractFrequency(decoded)
	}

	return 0, fmt.Errorf("frequency not found in ChirpStack payload")
}

// ExtractRxMetadata extracts gateway metadata from ChirpStack payload
// Location: uplinkEvent.rxInfo or rxInfo at root level
func (h *ChirpStackHandler) ExtractRxMetadata(payload map[string]interface{}) ([]interface{}, error) {
	if payload == nil {
		return nil, fmt.Errorf("payload is nil")
	}

	// Try direct rxInfo at root level (ChirpStack HTTP webhook format)
	if rxInfo, ok := payload["rxInfo"].([]interface{}); ok {
		return rxInfo, nil
	}

	// Try uplinkEvent.rxInfo (ChirpStack MQTT format)
	if uplinkEvent, ok := payload["uplinkEvent"].(map[string]interface{}); ok {
		if rxInfo, ok := uplinkEvent["rxInfo"].([]interface{}); ok {
			return rxInfo, nil
		}
	}

	// Try nested in decoded_raw_data
	if decoded, ok := payload["decoded_raw_data"].(map[string]interface{}); ok {
		return h.ExtractRxMetadata(decoded)
	}

	return nil, fmt.Errorf("rxInfo not found in ChirpStack payload")
}

// ExtractPayloadData extracts the payload data from ChirpStack payload
// Location: uplinkEvent.data
func (h *ChirpStackHandler) ExtractPayloadData(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}

	// Try direct uplinkEvent.data
	if uplinkEvent, ok := payload["uplinkEvent"].(map[string]interface{}); ok {
		if data, ok := uplinkEvent["data"].(string); ok && data != "" {
			return data
		}
	}

	// Try nested in decoded_raw_data
	if decoded, ok := payload["decoded_raw_data"].(map[string]interface{}); ok {
		return h.ExtractPayloadData(decoded)
	}

	// Try direct data field
	if data, ok := payload["data"].(string); ok && data != "" {
		return data
	}

	return ""
}

// ExtractPayloadBytes extracts raw payload bytes from ChirpStack metadata
// ChirpStack format: uplinkEvent.data (base64 encoded)
func (h *ChirpStackHandler) ExtractPayloadBytes(metadata map[string]interface{}) ([]byte, error) {
	// Try uplinkEvent.data
	if uplink, ok := metadata["uplinkEvent"].(map[string]interface{}); ok {
		if data, ok := uplink["data"].(string); ok && data != "" {
			decoded, err := base64.StdEncoding.DecodeString(data)
			if err != nil {
				return nil, fmt.Errorf("failed to decode ChirpStack data: %w", err)
			}
			return decoded, nil
		}
	}

	// Try decoded_raw_data.uplinkEvent.data
	if decoded, ok := metadata["decoded_raw_data"].(map[string]interface{}); ok {
		if uplink, ok := decoded["uplinkEvent"].(map[string]interface{}); ok {
			if data, ok := uplink["data"].(string); ok && data != "" {
				decoded, err := base64.StdEncoding.DecodeString(data)
				if err != nil {
					return nil, fmt.Errorf("failed to decode ChirpStack data: %w", err)
				}
				return decoded, nil
			}
		}
		// Try decoded_raw_data.data (at root of decoded_raw_data)
		if data, ok := decoded["data"].(string); ok && data != "" {
			decoded, err := base64.StdEncoding.DecodeString(data)
			if err != nil {
				return nil, fmt.Errorf("failed to decode ChirpStack data: %w", err)
			}
			return decoded, nil
		}
	}

	// Try data at root level
	if data, ok := metadata["data"].(string); ok && data != "" {
		decoded, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, fmt.Errorf("failed to decode ChirpStack data: %w", err)
		}
		return decoded, nil
	}

	return nil, fmt.Errorf("ChirpStack data not found in metadata")
}

// ExtractGatewayLocations extracts gateway locations from ChirpStack rxInfo
// ChirpStack format: rxInfo[].location.latitude/longitude
func (h *ChirpStackHandler) ExtractGatewayLocations(rxMetadata []interface{}) ([]GatewayMetadata, error) {
	var locations []GatewayMetadata

	for _, gw := range rxMetadata {
		gateway, ok := gw.(map[string]interface{})
		if !ok {
			continue
		}

		locationData, ok := gateway["location"].(map[string]interface{})
		if !ok {
			continue
		}

		lat, latOk := locationData["latitude"].(float64)
		lon, lonOk := locationData["longitude"].(float64)
		rssi, rssiOk := gateway["rssi"].(float64)

		if latOk && lonOk && rssiOk {
			locations = append(locations, GatewayMetadata{
				Latitude:  lat,
				Longitude: lon,
				RSSI:      int(rssi),
			})
		}
	}

	if len(locations) == 0 {
		return nil, fmt.Errorf("no valid gateway locations found in ChirpStack metadata")
	}

	return locations, nil
}
