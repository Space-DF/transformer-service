package mqtt

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Space-DF/transformer-service/internal/models"
	"github.com/Space-DF/transformer-service/internal/mqtt/logging"
	segmentjson "github.com/segmentio/encoding/json"
)

var apiEntityKeyPattern = regexp.MustCompile(`[^a-z0-9_]+`)

type apiPayload struct {
	SerialNumber string
	Payload      string
	Metadata     map[string]interface{}
}

func (c *Consumer) isAPIMessage(payload map[string]interface{}) bool {
	if payload == nil {
		return false
	}

	if metadata, ok := payload["metadata"].(map[string]interface{}); ok {
		if source, ok := metadata["api_source"].(string); ok && source == "api" {
			return true
		}
	}

	_, hasDeviceID := payload["serialNumber"].(string)
	_, hasPayload := payload["payload"].(string)
	if hasDeviceID && hasPayload {
		return true
	}

	if decoded, ok := payload["decoded_raw_data"].(map[string]interface{}); ok {
		_, hasDeviceID = decoded["serialNumber"].(string)
		_, hasPayload = decoded["payload"].(string)
		return hasDeviceID && hasPayload
	}

	return false
}

func (c *Consumer) handleAPIMessage(tenant *TenantConsumer, payload map[string]interface{}) error {
	message, err := extractAPIPayload(payload)
	if err != nil {
		return err
	}

	if c.deviceProfileService == nil {
		return fmt.Errorf("device profile service is required for API device messages")
	}

	mapping, err := c.deviceProfileService.GetAPIDeviceMapping(tenant.OrgSlug, message.SerialNumber)
	if err != nil {
		return err
	}
	if mapping == nil || strings.TrimSpace(mapping.DeviceID) == "" {
		return fmt.Errorf("api device mapping for %s did not include device id", message.SerialNumber)
	}

	rawBytes, err := base64.StdEncoding.DecodeString(message.Payload)
	if err != nil {
		return fmt.Errorf("invalid api payload base64: %w", err)
	}

	timestamp := apiMessageTimestamp(message.Metadata)
	entities := buildAPIEntities(tenant.OrgSlug, message.SerialNumber, rawBytes, timestamp)
	if len(entities) == 0 {
		return fmt.Errorf("api payload produced no telemetry entities")
	}

	telemetryPayload := &models.TelemetryPayload{
		Organization:  tenant.OrgSlug,
		DeviceEUI:     message.SerialNumber,
		DeviceID:      mapping.DeviceID,
		SpaceSlug:     mapping.SpaceSlug,
		IsPublished:   mapping.IsPublished,
		IsDeactivated: mapping.IsDeactivated,
		DeviceInfo: models.TelemetryDeviceInfo{
			Identifiers:  []string{message.SerialNumber},
			Name:         firstNonEmpty(mapping.DeviceName, message.SerialNumber),
			Manufacturer: firstNonEmpty(mapping.Manufacture, "api"),
			Model:        firstNonEmpty(mapping.Profile, "api"),
			ModelID:      firstNonEmpty(mapping.Profile, "api"),
		},
		Entities:  entities,
		Timestamp: timestamp,
		Source:    "transformer-service",
		Metadata: map[string]interface{}{
			"api_source":         "api",
			"serial_number":      message.SerialNumber,
			"raw_payload_base64": message.Payload,
			"raw_payload_hex":    hex.EncodeToString(rawBytes),
		},
	}

	for key, value := range message.Metadata {
		telemetryPayload.Metadata[key] = value
	}

	if err := c.publishTelemetry(tenant.Channel, telemetryPayload, tenant); err != nil {
		return fmt.Errorf("failed to publish api telemetry: %w", err)
	}

	logging.Tenant(tenant.OrgSlug, tenant.Vhost, "✅", "Processed API message for serial number %s with %d entities", message.SerialNumber, len(entities))
	return nil
}

func extractAPIPayload(payload map[string]interface{}) (*apiPayload, error) {
	source := payload
	if decoded, ok := payload["decoded_raw_data"].(map[string]interface{}); ok {
		source = decoded
	}

	serialNumber := strings.TrimSpace(stringFromMap(source, "serialNumber"))
	if serialNumber == "" {
		return nil, fmt.Errorf("api serialNumber is required")
	}

	encodedPayload := strings.TrimSpace(stringFromMap(source, "payload"))
	if encodedPayload == "" {
		return nil, fmt.Errorf("api payload is required")
	}

	metadata := map[string]interface{}{}
	if envelopeMetadata, ok := payload["metadata"].(map[string]interface{}); ok {
		for key, value := range envelopeMetadata {
			metadata[key] = value
		}
	}

	return &apiPayload{
		SerialNumber: serialNumber,
		Payload:      encodedPayload,
		Metadata:     metadata,
	}, nil
}

func buildAPIEntities(orgSlug, deviceID string, rawBytes []byte, timestamp string) []models.TelemetryEntity {
	var decoded map[string]interface{}
	if err := segmentjson.Unmarshal(rawBytes, &decoded); err == nil {
		return buildAPIJSONEntities(orgSlug, deviceID, decoded, timestamp)
	}

	return []models.TelemetryEntity{
		newAPIEntity(orgSlug, deviceID, "raw_payload", "Raw Payload", "sensor", "", "", "", []string{"value"}, base64.StdEncoding.EncodeToString(rawBytes), timestamp),
	}
}

func buildAPIJSONEntities(orgSlug, deviceID string, decoded map[string]interface{}, timestamp string) []models.TelemetryEntity {
	keys := make([]string, 0, len(decoded))
	for key := range decoded {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	entities := make([]models.TelemetryEntity, 0, len(keys))
	for _, key := range keys {
		value := decoded[key]
		if !isAPIScalar(value) {
			continue
		}

		entityKey := normalizeAPIEntityKey(key)
		entityType, deviceClass, unitOfMeas, icon, displayType := classifyAPIEntity(entityKey, value)
		entities = append(entities, newAPIEntity(orgSlug, deviceID, entityKey, titleFromKey(key), entityType, deviceClass, unitOfMeas, icon, displayType, value, timestamp))
	}

	if len(entities) == 0 {
		entities = append(entities, newAPIEntity(orgSlug, deviceID, "raw_payload", "Raw Payload", "sensor", "", "", "", []string{"value"}, mustMarshalString(decoded), timestamp))
	}

	return entities
}

func newAPIEntity(orgSlug, deviceID, key, name, entityType, deviceClass, unitOfMeas, icon string, displayType []string, state interface{}, timestamp string) models.TelemetryEntity {
	uniqueID := fmt.Sprintf("%s_%s_%s", normalizeAPIEntityKey(orgSlug), normalizeAPIEntityKey(deviceID), key)
	return models.TelemetryEntity{
		UniqueID:    uniqueID,
		EntityID:    fmt.Sprintf("%s.%s", entityType, uniqueID),
		EntityType:  entityType,
		DeviceClass: deviceClass,
		Name:        name,
		State:       state,
		Attributes: map[string]interface{}{
			"source": "api",
		},
		DisplayType: displayType,
		UnitOfMeas:  unitOfMeas,
		Icon:        icon,
		Timestamp:   timestamp,
	}
}

func normalizeAPIEntityKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "_")
	value = apiEntityKeyPattern.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_")
	if value == "" {
		return "value"
	}
	return value
}

func titleFromKey(value string) string {
	value = strings.ReplaceAll(normalizeAPIEntityKey(value), "_", " ")
	words := strings.Fields(value)
	for i, word := range words {
		if word == "" {
			continue
		}
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	if len(words) == 0 {
		return "Value"
	}
	return strings.Join(words, " ")
}

func classifyAPIEntity(key string, value interface{}) (entityType, deviceClass, unitOfMeas, icon string, displayType []string) {
	entityType = "sensor"
	displayType = []string{"value"}
	if _, ok := value.(bool); ok {
		entityType = "binary_sensor"
	}

	switch {
	case key == "distance" || strings.Contains(key, "distance"):
		return "distance", "distance", "cm", "water_depth.svg", []string{"chart", "gauge", "value"}
	case strings.Contains(key, "temperature"):
		deviceClass = "temperature"
	case strings.Contains(key, "humidity"):
		deviceClass = "humidity"
	case strings.Contains(key, "battery"):
		deviceClass = "battery"
	case strings.Contains(key, "pressure"):
		deviceClass = "pressure"
	case strings.Contains(key, "voltage"):
		deviceClass = "voltage"
	case strings.Contains(key, "current"):
		deviceClass = "current"
	}

	return entityType, deviceClass, unitOfMeas, icon, displayType
}

func isAPIScalar(value interface{}) bool {
	switch value.(type) {
	case string, bool, float64, int, int64, uint64, nil:
		return true
	default:
		return false
	}
}

func apiMessageTimestamp(metadata map[string]interface{}) string {
	if value := strings.TrimSpace(stringFromMap(metadata, "received_at")); value != "" {
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			return parsed.UTC().Format(time.RFC3339)
		}
	}
	return time.Now().UTC().Format(time.RFC3339)
}

func stringFromMap(values map[string]interface{}, key string) string {
	if values == nil {
		return ""
	}
	if value, ok := values[key].(string); ok {
		return value
	}
	return ""
}

func boolFromMap(values map[string]interface{}, key string) bool {
	if values == nil {
		return false
	}
	if value, ok := values[key].(bool); ok {
		return value
	}
	if value, ok := values[key].(string); ok {
		parsed, _ := strconv.ParseBool(value)
		return parsed
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func mustMarshalString(value interface{}) string {
	body, err := segmentjson.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(body)
}
