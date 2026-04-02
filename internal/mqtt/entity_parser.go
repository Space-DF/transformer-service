package mqtt

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Space-DF/transformer-service/internal/components"
	"github.com/Space-DF/transformer-service/internal/components/registry"
	"github.com/Space-DF/transformer-service/internal/models"
)

// parseEntities attempts to parse entities for telemetry and returns the device mapping
func (c *Consumer) parseEntities(orgSlug, devEUI string, payload map[string]interface{}, deviceLocation *components.Location, lnsType models.LNSType) (*components.ParseResult, *models.DeviceMapping, error) {
	if devEUI == "" {
		return nil, nil, fmt.Errorf("dev_eui missing")
	}

	mapping, err := c.deviceProfileService.GetDeviceMapping(orgSlug, devEUI)
	if err != nil || mapping == nil {
		return nil, nil, fmt.Errorf("device mapping not found: %w", err)
	}

	deviceType := components.DeviceType(strings.ToUpper(mapping.Profile))
	raw := &components.RawPayload{
		DeviceEUI: devEUI,
		Timestamp: time.Now(),
		Metadata:  payload,
		LNSType:   lnsType,
	}

	// Extract data field from the most likely location first
	raw.Data = c.extractDataField(payload)

	component := registry.FindComponent(deviceType, raw)
	if component == nil {
		return nil, nil, fmt.Errorf("no component found for device type: %s", deviceType)
	}

	parseResult, err := component.ParseToEntities(context.Background(), orgSlug, mapping.Profile, deviceType, raw, deviceLocation)
	return parseResult, mapping, err
}

// buildTelemetryPayload converts parse result to telemetry payload
func (c *Consumer) buildTelemetryPayload(parseResult *components.ParseResult, orgSlug string, mapping *models.DeviceMapping) (*models.TelemetryPayload, error) {
	if parseResult == nil {
		return nil, fmt.Errorf("parse result is nil")
	}

	// Get device identifiers from mapping (prefer mapping over payload)
	deviceID := "unknown"
	spaceSlug := ""
	if mapping != nil {
		deviceID = mapping.DeviceID
		spaceSlug = mapping.SpaceSlug
	}

	deviceInfo := models.TelemetryDeviceInfo{
		Identifiers:  parseResult.DeviceInfo.Identifiers,
		Name:         parseResult.DeviceInfo.Name,
		Manufacturer: parseResult.DeviceInfo.Manufacturer,
		Model:        parseResult.DeviceInfo.Model,
		ModelID:      parseResult.DeviceInfo.ModelID,
	}

	var telemetryEntities []models.TelemetryEntity
	for _, entity := range parseResult.Entities {
		telemetryEntities = append(telemetryEntities, models.TelemetryEntity{
			UniqueID:    entity.UniqueID,
			EntityID:    entity.EntityID,
			EntityType:  entity.EntityType,
			DeviceClass: entity.DeviceClass,
			Name:        entity.Name,
			State:       entity.State,
			Attributes:  entity.Attributes,
			DisplayType: entity.DisplayType,
			UnitOfMeas:  entity.UnitOfMeas,
			Timestamp:   entity.Timestamp.Format(time.RFC3339),
		})
	}

	return &models.TelemetryPayload{
		Organization: orgSlug,
		DeviceEUI:    parseResult.DeviceEUI,
		DeviceID:     deviceID,
		SpaceSlug:    spaceSlug,
		DeviceInfo:   deviceInfo,
		Entities:     telemetryEntities,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Source:       "transformer-service",
	}, nil
}
