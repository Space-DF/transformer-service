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
// TTN format: rx_metadata[].location.latitude/longitude (when gateway registry is configured)
// Returns gateway locations if available, otherwise returns empty slice (no error)
func (h *TTNHandler) ExtractGatewayLocations(rxMetadata []interface{}) ([]GatewayMetadata, error) {
	if len(rxMetadata) == 0 {
		return nil, fmt.Errorf("no rx_metadata found in TTN payload")
	}

	var gateways []GatewayMetadata

	for _, rx := range rxMetadata {
		rxMap, ok := rx.(map[string]interface{})
		if !ok {
			continue
		}

		// Extract RSSI
		rssi := 0
		if rssiVal, ok := rxMap["rssi"].(float64); ok {
			rssi = int(rssiVal)
		}

		// Extract location if present (from TTN gateway registry)
		var lat, lon float64
		if location, ok := rxMap["location"].(map[string]interface{}); ok {
			if latVal, ok := location["latitude"].(float64); ok {
				lat = latVal
			}
			if lonVal, ok := location["longitude"].(float64); ok {
				lon = lonVal
			}
		}

		// Only add gateways with valid location data
		if lat != 0 || lon != 0 {
			gateways = append(gateways, GatewayMetadata{
				Latitude:  lat,
				Longitude: lon,
				RSSI:      rssi,
			})
		}
	}

	return gateways, nil
}

func (h *TTNHandler) ExtractEventType(payload map[string]interface{}) EventType {
	if payload == nil {
		return EventUnknown
	}

	decoded, ok := payload["decoded_raw_data"].(map[string]interface{})
	if !ok {
		return EventUnknown
	}

	if _, ok := decoded["uplink_message"]; ok {
		return EventUplink
	}
	if _, ok := decoded["join_accept"]; ok {
		return EventJoin
	}
	if _, ok := decoded["downlink_failed"]; ok {
		return EventAlert
	}
	if _, ok := decoded["downlink_nack"]; ok {
		return EventAlert
	}
	if _, ok := decoded["downlink_ack"]; ok {
		return EventAck
	}
	if _, ok := decoded["downlink_queued"]; ok {
		return EventAck
	}
	if _, ok := decoded["downlink_sent"]; ok {
		return EventAck
	}
	if _, ok := decoded["location_solved"]; ok {
		return EventAck
	}
	if _, ok := decoded["service_data"]; ok {
		return EventAck
	}

	return EventUnknown
}

func (h *TTNHandler) ExtractAlert(payload map[string]interface{}) *LNSAlert {
	if payload == nil {
		return nil
	}

	decoded, ok := payload["decoded_raw_data"].(map[string]interface{})
	if !ok {
		return nil
	}

	if df, ok := decoded["downlink_failed"].(map[string]interface{}); ok {
		if errObj, ok := df["error"].(map[string]interface{}); ok {
			return &LNSAlert{
				Code:    getStrField(errObj, "name"),
				Level:   "ERROR",
				Message: getStrField(errObj, "message_format"),
				Source:  "ttn",
			}
		}
	}

	if _, ok := decoded["downlink_nack"]; ok {
		return &LNSAlert{
			Code:    "DOWNLINK_NACK",
			Level:   "WARNING",
			Message: "Downlink was not acknowledged by device",
			Source:  "ttn",
		}
	}

	return nil
}

func getStrField(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
