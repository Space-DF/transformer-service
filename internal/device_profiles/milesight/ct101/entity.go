package ct101

import (
	"fmt"
	"strings"
	"time"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
	"github.com/Space-DF/transformer-service/internal/lns"
)

func (p *CT101Component) ParsePayload(payload *common.RawPayload) (*common.ParsedData, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = lns.ExtractDevEUI(payload.Metadata, payload.LNSType)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI not found")
	}

	sensors := Decode(payload)

	return &common.ParsedData{
		DeviceEUI:  devEUI,
		DeviceType: common.DeviceType(Model),
		Timestamp:  payload.Timestamp,
		SensorData: sensors,
		RawData:    payload.Data,
	}, nil
}

func (p *CT101Component) ParseToEntities(orgSlug, model string, payload *common.RawPayload, deviceLocation *common.Location) ([]common.Entity, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = lns.ExtractDevEUI(payload.Metadata, payload.LNSType)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI is required")
	}

	parsed, err := p.ParsePayload(payload)
	if err != nil {
		return nil, err
	}

	ts := payload.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	mdl := strings.ToLower(model)
	var entities []common.Entity

	// Sensor entities using for loop pattern
	type sensorDef struct {
		key, name, entityType, devClass, unit, icon string
		display                                     []string
		transform                                   func(any) (state any, attributes map[string]any)
	}
	for _, def := range []sensorDef{
		// Simple sensor entities
		{"current", "Current", "current", "current", "A", "current.svg", []string{"chart", "gauge", "value"}, nil},
		{"total_current", "Total Current", "total_current", "current", "A", "total_current.svg", []string{"chart", "gauge", "value"}, nil},
		{"temperature", "Temperature", "temperature", "temperature", "°C", "temperature.svg", []string{"chart", "gauge", "value"}, nil},
		// Alarm entities with transform
		{
			"current_alarm", "Current Alarm", "current_alarm", "problem", "", "current alarm.svg", []string{"indicator"},
			func(v any) (any, map[string]any) {
				val, ok := v.(map[string]any)
				if !ok {
					return nil, nil
				}
				alarmStatus := "off"
				if threshold, ok := val["current_threshold_alarm"].(bool); ok && threshold {
					alarmStatus = "on"
				} else if overRange, ok := val["current_over_range_alarm"].(bool); ok && overRange {
					alarmStatus = "on"
				}
				return alarmStatus, val
			},
		},
		{
			"temperature_alarm", "Temperature Alarm", "temperature_alarm", "problem", "", "temperature_alarm.svg", []string{"indicator"},
			func(v any) (any, map[string]any) {
				val, ok := v.(string)
				if !ok {
					return nil, nil
				}
				alarmStatus := "off"
				if val == "temperature threshold alarm" {
					alarmStatus = "on"
				}
				return alarmStatus, map[string]any{"alarm_type": val}
			},
		},
		// Sensor status entities
		{"current_sensor_status", "Current Sensor Status", "sensor", "problem", "", "current_sensor_status.svg", []string{"text"}, nil},
		{"temperature_sensor_status", "Temperature Sensor Status", "sensor", "problem", "", "tempt_sensor_status.svg", []string{"text"}, nil},
	} {
		val, ok := parsed.SensorData[def.key]
		if !ok {
			continue
		}

		// Skip empty string values for status sensors
		if strVal, ok := val.(string); ok && strVal == "" {
			continue
		}

		state := val
		attributes := map[string]any{}

		if def.transform != nil {
			var attrs map[string]any
			state, attrs = def.transform(val)
			if state == nil {
				continue
			}
			if attrs != nil {
				attributes = attrs
			}
		}

		entity := common.Entity{
			UniqueID:    common.GenerateUniqueID(model, devEUI, def.key),
			EntityID:    common.GenerateEntityID(common.GetEntityDomain(def.key), orgSlug, Manufacturer, mdl, devEUI, def.key),
			EntityType:  def.entityType,
			DeviceClass: def.devClass,
			Name:        def.name,
			State:       state,
			DisplayType: def.display,
			UnitOfMeas:  def.unit,
			Icon:        def.icon,
			Enabled:     true,
			Timestamp:   ts,
		}

		if len(attributes) > 0 {
			entity.Attributes = attributes
		}

		entities = append(entities, entity)
	}

	// Add device metadata as attributes to first entity if available
	if len(entities) > 0 {
		metadata := make(map[string]any)
		if hwVersion, ok := parsed.SensorData["hardware_version"].(string); ok {
			metadata["hardware_version"] = hwVersion
		}
		if fwVersion, ok := parsed.SensorData["firmware_version"].(string); ok {
			metadata["firmware_version"] = fwVersion
		}
		if sn, ok := parsed.SensorData["serial_number"].(string); ok {
			metadata["serial_number"] = sn
		}
		if len(metadata) > 0 {
			if entities[0].Attributes == nil {
				entities[0].Attributes = make(map[string]any)
			}
			for k, v := range metadata {
				entities[0].Attributes[k] = v
			}
		}
	}

	return entities, nil
}
