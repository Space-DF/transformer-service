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
	entities := common.BuildEntitiesFromState(orgSlug, model, Manufacturer, mdl, devEUI, entityDefs(), parsed.SensorData, ts)

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

func (p *CT101Component) GetEntityTemplates(model, devEUI string) []common.Entity {
	mdl := strings.ToLower(model)
	return common.BuildEntityTemplates("", model, Manufacturer, mdl, devEUI, entityDefs())
}

func entityDefs() []common.EntityDef {
	return []common.EntityDef{
		{Key: "current", Name: "Current", EntityType: "current", DeviceClass: "current", UnitOfMeas: "A", Icon: "current.svg", DisplayType: []string{"chart", "gauge", "value"}},
		{Key: "total_current", Name: "Total Current", EntityType: "total_current", DeviceClass: "current", UnitOfMeas: "A", Icon: "total_current.svg", DisplayType: []string{"chart", "gauge", "value"}},
		{Key: "temperature", Name: "Temperature", EntityType: "temperature", DeviceClass: "temperature", UnitOfMeas: "°C", Icon: "temperature.svg", DisplayType: []string{"chart", "gauge", "value"}},
		{
			Key:         "current_alarm",
			Name:        "Current Alarm",
			EntityType:  "current_alarm",
			DeviceClass: "problem",
			Icon:        "current_alarm.svg",
			DisplayType: []string{"indicator"},
			Transform: func(v any) (any, map[string]any, bool) {
				val, ok := v.(map[string]any)
				if !ok {
					return nil, nil, false
				}
				alarmStatus := "off"
				if threshold, ok := val["current_threshold_alarm"].(bool); ok && threshold {
					alarmStatus = "on"
				} else if overRange, ok := val["current_over_range_alarm"].(bool); ok && overRange {
					alarmStatus = "on"
				}
				return alarmStatus, val, true
			},
		},
		{
			Key:         "temperature_alarm",
			Name:        "Temperature Alarm",
			EntityType:  "temperature_alarm",
			DeviceClass: "problem",
			Icon:        "temperature_alarm.svg",
			DisplayType: []string{"indicator"},
			Transform: func(v any) (any, map[string]any, bool) {
				val, ok := v.(string)
				if !ok {
					return nil, nil, false
				}
				alarmStatus := "off"
				if val == "temperature threshold alarm" {
					alarmStatus = "on"
				}
				return alarmStatus, map[string]any{"alarm_type": val}, true
			},
		},
		{
			Key:         "current_sensor_status",
			Name:        "Current Sensor Status",
			EntityType:  "sensor",
			DeviceClass: "problem",
			Icon:        "current_sensor_status.svg",
			DisplayType: []string{"text"},
			Skip: func(v any) bool {
				strVal, ok := v.(string)
				return ok && strVal == ""
			},
		},
		{
			Key:         "temperature_sensor_status",
			Name:        "Temperature Sensor Status",
			EntityType:  "sensor",
			DeviceClass: "problem",
			Icon:        "tempt_sensor_status.svg",
			DisplayType: []string{"text"},
			Skip: func(v any) bool {
				strVal, ok := v.(string)
				return ok && strVal == ""
			},
		},
	}
}
