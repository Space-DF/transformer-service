package payload

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Parser struct {
}

func NewParser() *Parser {
	return &Parser{}
}

func (p *Parser) Parse(msg amqp.Delivery) (payload map[string]interface{}, locationPayload map[string]interface{}, err error) {
	// Parse the incoming message
	var rawPayload map[string]interface{}
	if err := json.Unmarshal(msg.Body, &rawPayload); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	// Use the orgSlug passed as parameter (we already know it from the queue name)
	// Check if there's a base64 encoded payload that needs to be decoded
	var encodedPayload string
	var found bool

	// Check for payload key (JSON stringified)
	if payloadStr, ok := rawPayload["payload"].(string); ok {
		encodedPayload = payloadStr
		found = true
	}

	if found {
		// First try to parse as JSON string directly
		var jsonPayload map[string]interface{}
		if err := json.Unmarshal([]byte(encodedPayload), &jsonPayload); err == nil {

			payload = jsonPayload
		} else {
			// If JSON parsing fails, use raw payload
			payload = rawPayload
		}
	} else {
		// No base64 encoded payload, use raw payload directly
		payload = rawPayload
	}

	// Debug: Print final payload structure and decode raw_data if found
	locationPayload = payload // Default to original payload

	if rawData, ok := payload["raw_data"]; ok {
		if rawStr, ok := rawData.(string); ok {
			if decoded, err := base64.StdEncoding.DecodeString(rawStr); err == nil {
				var jsonData interface{}
				if json.Unmarshal(decoded, &jsonData) == nil {
					if jsonMap, ok := jsonData.(map[string]interface{}); ok {
						payload["decoded_raw_data"] = jsonMap
						locationPayload = jsonMap
					} else {
						payload["decoded_raw_data"] = jsonData
					}
				}
			}
		}
	}

	return payload, locationPayload, nil
}

func (p *Parser) ExtractDevEUI(payload map[string]interface{}, locationPayload map[string]interface{}) string {
	var devEUI string

	if endDeviceIDs, ok := payload["end_device_ids"].(map[string]interface{}); ok {
		if eui, ok := endDeviceIDs["dev_eui"].(string); ok {
			devEUI = eui
		}
	}

	// If not found in payload, try locationPayload or check for dev_eui directly
	if devEUI == "" {
		if eui, ok := locationPayload["dev_eui"].(string); ok {
			devEUI = eui
		} else if eui, ok := payload["dev_eui"].(string); ok {
			devEUI = eui
		} else if eui, ok := locationPayload["devEui"].(string); ok {
			// LoRaWAN format uses devEui
			devEUI = eui
		} else if deviceInfo, ok := locationPayload["deviceInfo"].(map[string]interface{}); ok {
			// Try deviceInfo.devEui
			if eui, ok := deviceInfo["devEui"].(string); ok {
				devEUI = eui
			}
		}
	}

	return devEUI
}
