package extractors

import (
	"encoding/base64"
	"fmt"
	"strconv"

	"github.com/Space-DF/transformer-service/internal/models"
)

// HeliumHandler handles data from Helium LNS webhook payloads
// Helium payload structure:
//
//	{
//	  "dev_eui": "...",
//	  "fport": 1,
//	  "fcnt": 123,
//	  "payload": "...",
//	  "frequency": 868300000,
//	  "hotspots": [...]
//	}
type HeliumHandler struct{}

func (h *HeliumHandler) Name() string {
	return "helium"
}

// ExtractDevEUI extracts DevEUI from Helium payload
// Location: dev_eui (at root level)
func (h *HeliumHandler) ExtractDevEUI(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}

	// Try direct dev_eui at root
	for _, key := range []string{"dev_eui", "device_eui", "devEui", "deviceEui"} {
		if devEUI, ok := payload[key].(string); ok && devEUI != "" {
			return devEUI
		}
	}

	// Try nested in decoded_raw_data
	if decoded, ok := payload["decoded_raw_data"].(map[string]interface{}); ok {
		return h.ExtractDevEUI(decoded)
	}

	return ""
}

// ExtractFPort extracts fPort from Helium payload
// Location: fport (at root level)
func (h *HeliumHandler) ExtractFPort(payload map[string]interface{}) int {
	if payload == nil {
		return 0
	}

	// Try various fport field names
	for _, key := range []string{"fport", "f_port", "fPort", "port"} {
		if val, ok := payload[key]; ok {
			switch v := val.(type) {
			case float64:
				return int(v)
			case int:
				return v
			case int64:
				return int(v)
			}
		}
	}

	// Try nested in decoded_raw_data
	if decoded, ok := payload["decoded_raw_data"].(map[string]interface{}); ok {
		return h.ExtractFPort(decoded)
	}

	return 0
}

// ExtractFrequency extracts frequency from Helium payload
// Location: frequency (at root level)
func (h *HeliumHandler) ExtractFrequency(payload map[string]interface{}) (float64, error) {
	if payload == nil {
		return 0, fmt.Errorf("payload is nil")
	}

	// Try direct frequency at root
	if freq, ok := payload["frequency"].(float64); ok {
		return freq, nil
	}

	// Try as string and convert
	if freqStr, ok := payload["frequency"].(string); ok {
		if freq, err := strconv.ParseFloat(freqStr, 64); err == nil && freq > 0 {
			return freq, nil
		}
	}

	// Try nested in decoded_raw_data
	if decoded, ok := payload["decoded_raw_data"].(map[string]interface{}); ok {
		return h.ExtractFrequency(decoded)
	}

	return 0, fmt.Errorf("frequency not found in Helium payload")
}

// ExtractRxMetadata extracts hotspot/gateway metadata from Helium payload
// Location: hotspots (at root level)
func (h *HeliumHandler) ExtractRxMetadata(payload map[string]interface{}) ([]interface{}, error) {
	if payload == nil {
		return nil, fmt.Errorf("payload is nil")
	}

	// Try direct hotspots at root
	if hotspots, ok := payload["hotspots"].([]interface{}); ok {
		return hotspots, nil
	}

	// Try alternate names
	for _, key := range []string{"rx_metadata", "rxInfo", "gateways"} {
		if metadata, ok := payload[key].([]interface{}); ok {
			return metadata, nil
		}
	}

	// Try nested in decoded_raw_data
	if decoded, ok := payload["decoded_raw_data"].(map[string]interface{}); ok {
		return h.ExtractRxMetadata(decoded)
	}

	return nil, fmt.Errorf("hotspots/rx_metadata not found in Helium payload")
}

// ExtractPayloadData extracts the payload data from Helium payload
// Location: payload (at root level)
func (h *HeliumHandler) ExtractPayloadData(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}

	// Try direct payload at root
	for _, key := range []string{"payload", "data", "frm_payload"} {
		if data, ok := payload[key].(string); ok && data != "" {
			return data
		}
	}

	// Try nested in decoded_raw_data
	if decoded, ok := payload["decoded_raw_data"].(map[string]interface{}); ok {
		return h.ExtractPayloadData(decoded)
	}

	return ""
}

// ExtractPayloadBytes extracts payload bytes from Helium metadata
// Helium format: payload (at root level, base64 encoded)
func (h *HeliumHandler) ExtractPayloadBytes(metadata map[string]interface{}) ([]byte, error) {
	// Try payload at root level
	if payload, ok := metadata["payload"].(string); ok && payload != "" {
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to decode Helium payload: %w", err)
		}
		return decoded, nil
	}

	// Try decoded_raw_data.payload
	if decoded, ok := metadata["decoded_raw_data"].(map[string]interface{}); ok {
		if payload, ok := decoded["payload"].(string); ok && payload != "" {
			decoded, err := base64.StdEncoding.DecodeString(payload)
			if err != nil {
				return nil, fmt.Errorf("failed to decode Helium payload: %w", err)
			}
			return decoded, nil
		}
		// Try decoded_raw_data.data (at root of decoded_raw_data)
		if data, ok := decoded["data"].(string); ok && data != "" {
			decoded, err := base64.StdEncoding.DecodeString(data)
			if err != nil {
				return nil, fmt.Errorf("failed to decode Helium data: %w", err)
			}
			return decoded, nil
		}
	}

	// Try data at root level
	if data, ok := metadata["data"].(string); ok && data != "" {
		decoded, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, fmt.Errorf("failed to decode Helium data: %w", err)
		}
		return decoded, nil
	}

	return nil, fmt.Errorf("Helium payload not found in metadata")
}

// ExtractGatewayLocations extracts gateway locations from Helium hotspots
// Helium format: hotspots[].location.latitude/longitude
func (h *HeliumHandler) ExtractGatewayLocations(rxMetadata []interface{}) ([]models.GatewayMetadata, error) {
	var locations []models.GatewayMetadata

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
			locations = append(locations, models.GatewayMetadata{
				Latitude:  lat,
				Longitude: lon,
				RSSI:      int(rssi),
			})
		}
	}

	if len(locations) == 0 {
		return nil, fmt.Errorf("no valid hotspot locations found in Helium metadata")
	}

	return locations, nil
}
