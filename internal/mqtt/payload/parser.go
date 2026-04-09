package payload

import (
	"encoding/base64"
	"fmt"

	"github.com/Space-DF/transformer-service/internal/lns"
	amqp "github.com/rabbitmq/amqp091-go"
	segmentjson "github.com/segmentio/encoding/json"
)

type Parser struct {
}

func NewParser() *Parser {
	return &Parser{}
}

func (p *Parser) Parse(msg amqp.Delivery) (payload map[string]interface{}, locationPayload map[string]interface{}, err error) {
	// Parse the incoming message
	var rawPayload map[string]interface{}
	if err := segmentjson.Unmarshal(msg.Body, &rawPayload); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	payload = rawPayload
	locationPayload = payload

	// Check if payload has nested JSON string (MQTT format)
	if payloadStr, ok := payload["payload"].(string); ok && payloadStr != "" {
		var nestedPayload map[string]interface{}
		if err := segmentjson.Unmarshal([]byte(payloadStr), &nestedPayload); err == nil {
			payload = nestedPayload
		}
	}

	// Decode raw_data if present (nested format)
	if rawData, ok := payload["raw_data"].(string); ok {
		if decoded, err := base64.StdEncoding.DecodeString(rawData); err == nil {
			var jsonData map[string]interface{}
			if segmentjson.Unmarshal(decoded, &jsonData) == nil {
				payload["decoded_raw_data"] = jsonData
				locationPayload = jsonData
			}
		}
	}

	return payload, locationPayload, nil
}

// ExtractLNSSource extracts the LNS type from the payload.
func (p *Parser) ExtractLNSSource(payload map[string]interface{}) lns.LNSType {
	return lns.ExtractLNSSource(payload)
}
