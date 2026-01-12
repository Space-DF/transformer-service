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

	payload = rawPayload
	locationPayload = payload

	// Check if payload has nested JSON string (MQTT format)
	if payloadStr, ok := payload["payload"].(string); ok && payloadStr != "" {
		var nestedPayload map[string]interface{}
		if err := json.Unmarshal([]byte(payloadStr), &nestedPayload); err == nil {
			payload = nestedPayload
		}
	}

	// Decode raw_data if present (nested format)
	if rawData, ok := payload["raw_data"].(string); ok {
		if decoded, err := base64.StdEncoding.DecodeString(rawData); err == nil {
			var jsonData map[string]interface{}
			if json.Unmarshal(decoded, &jsonData) == nil {
				payload["decoded_raw_data"] = jsonData
				locationPayload = jsonData
			}
		}
	}

	return payload, locationPayload, nil
}

func (p *Parser) ExtractDevEUI(payload map[string]interface{}, locationPayload map[string]interface{}) string {
	// For your data format, devEUI is consistently in decoded_raw_data.deviceInfo.devEui
	if locationPayload == nil {
		return ""
	}

	if deviceInfo, ok := locationPayload["deviceInfo"].(map[string]interface{}); ok {
		if eui, ok := deviceInfo["devEui"].(string); ok && eui != "" {
			return eui
		}
	}

	return ""
}
