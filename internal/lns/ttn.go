package lns

import (
	"encoding/base64"
	"fmt"
	"strconv"
)

// TTNHandler handles data from The Things Network (TTN) webhook payloads
// TTN payload structure:
//
//	{
//	  "end_device_ids": {"dev_eui": "..."},
//	  "uplink_message": {
//	    "f_port": 1,
//	    "f_cnt": 123,
//	    "frm_payload": "...",
//	    "settings": {"frequency": 868300000},
//	    "rx_metadata": [...]
//	  }
//	}
type TTNHandler struct{}

func (h *TTNHandler) Name() string {
	return "ttn"
}

// ExtractDevEUI extracts DevEUI from TTN payload
// Location: end_device_ids.dev_eui
func (h *TTNHandler) ExtractDevEUI(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}

	// Try direct end_device_ids.dev_eui
	if endDeviceIDs, ok := payload["end_device_ids"].(map[string]interface{}); ok {
		if devEUI, ok := endDeviceIDs["dev_eui"].(string); ok && devEUI != "" {
			return devEUI
		}
	}

	// Try nested in decoded_raw_data
	if decoded, ok := payload["decoded_raw_data"].(map[string]interface{}); ok {
		return h.ExtractDevEUI(decoded)
	}

	return ""
}

// ExtractFPort extracts fPort from TTN payload
// Location: uplink_message.f_port
func (h *TTNHandler) ExtractFPort(payload map[string]interface{}) int {
	if payload == nil {
		return 0
	}

	// Try direct uplink_message.f_port
	if uplinkMsg, ok := payload["uplink_message"].(map[string]interface{}); ok {
		if fPort, ok := uplinkMsg["f_port"].(float64); ok {
			return int(fPort)
		}
	}

	// Try nested in decoded_raw_data
	if decoded, ok := payload["decoded_raw_data"].(map[string]interface{}); ok {
		return h.ExtractFPort(decoded)
	}

	return 0
}

// ExtractFrequency extracts frequency from TTN payload
// Location: uplink_message.settings.frequency
func (h *TTNHandler) ExtractFrequency(payload map[string]interface{}) (float64, error) {
	if payload == nil {
		return 0, fmt.Errorf("payload is nil")
	}

	// Try direct uplink_message.settings.frequency
	if uplinkMsg, ok := payload["uplink_message"].(map[string]interface{}); ok {
		if settings, ok := uplinkMsg["settings"].(map[string]interface{}); ok {
			// TTN sends frequency as a string (e.g., "921400000")
			if freqStr, ok := settings["frequency"].(string); ok && freqStr != "" {
				if freq, err := strconv.ParseFloat(freqStr, 64); err == nil {
					return freq, nil
				}
			}
			if freq, ok := settings["frequency"].(float64); ok {
				return freq, nil
			}
		}
	}

	// Try nested in decoded_raw_data
	if decoded, ok := payload["decoded_raw_data"].(map[string]interface{}); ok {
		return h.ExtractFrequency(decoded)
	}

	return 0, fmt.Errorf("frequency not found in TTN payload")
}

// ExtractRxMetadata extracts gateway metadata from TTN payload
// Location: uplink_message.rx_metadata
func (h *TTNHandler) ExtractRxMetadata(payload map[string]interface{}) ([]interface{}, error) {
	if payload == nil {
		return nil, fmt.Errorf("payload is nil")
	}

	// Try direct uplink_message.rx_metadata
	if uplinkMsg, ok := payload["uplink_message"].(map[string]interface{}); ok {
		if rxMetadata, ok := uplinkMsg["rx_metadata"].([]interface{}); ok {
			return rxMetadata, nil
		}
	}

	// Try nested in decoded_raw_data
	if decoded, ok := payload["decoded_raw_data"].(map[string]interface{}); ok {
		return h.ExtractRxMetadata(decoded)
	}

	return nil, fmt.Errorf("rx_metadata not found in TTN payload")
}

// ExtractPayloadData extracts the payload data from TTN payload
// Location: uplink_message.frm_payload
func (h *TTNHandler) ExtractPayloadData(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}

	// Try direct uplink_message.frm_payload
	if uplinkMsg, ok := payload["uplink_message"].(map[string]interface{}); ok {
		if frmPayload, ok := uplinkMsg["frm_payload"].(string); ok && frmPayload != "" {
			return frmPayload
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

// ExtractPayloadBytes extracts raw payload bytes from TTN metadata
// TTN format: uplink_message.frm_payload (base64 encoded)
func (h *TTNHandler) ExtractPayloadBytes(metadata map[string]interface{}) ([]byte, error) {
	// Try uplink_message.frm_payload
	if uplink, ok := metadata["uplink_message"].(map[string]interface{}); ok {
		if frmPayload, ok := uplink["frm_payload"].(string); ok && frmPayload != "" {
			decoded, err := base64.StdEncoding.DecodeString(frmPayload)
			if err != nil {
				return nil, fmt.Errorf("failed to decode TTN frm_payload: %w", err)
			}
			return decoded, nil
		}
	}

	// Try decoded_raw_data.uplink_message.frm_payload
	if decoded, ok := metadata["decoded_raw_data"].(map[string]interface{}); ok {
		if uplink, ok := decoded["uplink_message"].(map[string]interface{}); ok {
			if frmPayload, ok := uplink["frm_payload"].(string); ok && frmPayload != "" {
				decoded, err := base64.StdEncoding.DecodeString(frmPayload)
				if err != nil {
					return nil, fmt.Errorf("failed to decode TTN frm_payload: %w", err)
				}
				return decoded, nil
			}
		}
		// Try decoded_raw_data.data (at root of decoded_raw_data)
		if data, ok := decoded["data"].(string); ok && data != "" {
			decoded, err := base64.StdEncoding.DecodeString(data)
			if err != nil {
				return nil, fmt.Errorf("failed to decode TTN data: %w", err)
			}
			return decoded, nil
		}
	}

	// Try data at root level
	if data, ok := metadata["data"].(string); ok && data != "" {
		decoded, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, fmt.Errorf("failed to decode TTN data: %w", err)
		}
		return decoded, nil
	}

	return nil, fmt.Errorf("TTN frm_payload not found in metadata")
}

// ExtractGatewayLocations extracts gateway locations from TTN rx_metadata
// TTN format: gateways don't include location, only gateway IDs
// Returns error indicating gateway registry lookup or device GPS is required
func (h *TTNHandler) ExtractGatewayLocations(rxMetadata []interface{}) ([]GatewayMetadata, error) {
	return nil, fmt.Errorf("TTN gateway location requires gateway registry lookup or device GPS")
}
