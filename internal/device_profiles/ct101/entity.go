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

	// Current Entity
	if val, ok := parsed.SensorData["current"].(float64); ok {
		entities = append(entities, common.Entity{
			UniqueID:    common.GenerateUniqueID(model, devEUI, "current"),
			EntityID:    common.GenerateEntityID(common.GetEntityDomain("current"), orgSlug, Manufacturer, mdl, devEUI, "current"),
			EntityType:  "current",
			DeviceClass: "current",
			Name:        "Current",
			State:       val,
			DisplayType: []string{"chart", "gauge", "value"},
			UnitOfMeas:  "A",
			Icon:        "mdi:current-ac",
			Enabled:     true,
			Timestamp:   ts,
		})
	}

	// Total Current Entity (cumulative)
	if val, ok := parsed.SensorData["total_current"].(float64); ok {
		entities = append(entities, common.Entity{
			UniqueID:    common.GenerateUniqueID(model, devEUI, "total_current"),
			EntityID:    common.GenerateEntityID(common.GetEntityDomain("total_current"), orgSlug, Manufacturer, mdl, devEUI, "total_current"),
			EntityType:  "total_current",
			DeviceClass: "current",
			Name:        "Total Current",
			State:       val,
			DisplayType: []string{"chart", "value"},
			UnitOfMeas:  "A",
			Icon:        "mdi:current-ac",
			Enabled:     true,
			Timestamp:   ts,
		})
	}

	// Temperature Entity
	if val, ok := parsed.SensorData["temperature"].(float64); ok {
		entities = append(entities, common.Entity{
			UniqueID:    common.GenerateUniqueID(model, devEUI, "temperature"),
			EntityID:    common.GenerateEntityID(common.GetEntityDomain("temperature"), orgSlug, Manufacturer, mdl, devEUI, "temperature"),
			EntityType:  "temperature",
			DeviceClass: "temperature",
			Name:        "Temperature",
			State:       val,
			DisplayType: []string{"chart", "gauge", "value"},
			UnitOfMeas:  "°C",
			Icon:        "mdi:thermometer",
			Enabled:     true,
			Timestamp:   ts,
		})
	}

	// Current Alarm Status (binary sensor)
	if val, ok := parsed.SensorData["current_alarm"].(map[string]any); ok {
		alarmStatus := "off"
		if threshold, ok := val["current_threshold_alarm"].(bool); ok && threshold {
			alarmStatus = "on"
		} else if overRange, ok := val["current_over_range_alarm"].(bool); ok && overRange {
			alarmStatus = "on"
		}

		entities = append(entities, common.Entity{
			UniqueID:    common.GenerateUniqueID(model, devEUI, "current_alarm"),
			EntityID:    common.GenerateEntityID("binary_sensor", orgSlug, Manufacturer, mdl, devEUI, "current_alarm"),
			EntityType:  "current_alarm",
			DeviceClass: "problem",
			Name:        "Current Alarm",
			State:       alarmStatus,
			DisplayType: []string{"indicator"},
			Attributes:  val,
			Icon:        "mdi:alert",
			Enabled:     true,
			Timestamp:   ts,
		})
	}

	// Temperature Alarm Status (binary sensor)
	if val, ok := parsed.SensorData["temperature_alarm"].(string); ok && val != "" {
		alarmStatus := "off"
		if val == "temperature threshold alarm" {
			alarmStatus = "on"
		}

		entities = append(entities, common.Entity{
			UniqueID:    common.GenerateUniqueID(model, devEUI, "temperature_alarm"),
			EntityID:    common.GenerateEntityID("binary_sensor", orgSlug, Manufacturer, mdl, devEUI, "temperature_alarm"),
			EntityType:  "temperature_alarm",
			DeviceClass: "problem",
			Name:        "Temperature Alarm",
			State:       alarmStatus,
			DisplayType: []string{"indicator"},
			Attributes: map[string]any{
				"alarm_type": val,
			},
			Icon:      "mdi:alert",
			Enabled:   true,
			Timestamp: ts,
		})
	}

	// Current Sensor Status (if sensor has issues)
	if val, ok := parsed.SensorData["current_sensor_status"].(string); ok && val != "" {
		entities = append(entities, common.Entity{
			UniqueID:    common.GenerateUniqueID(model, devEUI, "current_sensor_status"),
			EntityID:    common.GenerateEntityID("sensor", orgSlug, Manufacturer, mdl, devEUI, "current_sensor_status"),
			EntityType:  "sensor",
			DeviceClass: "problem",
			Name:        "Current Sensor Status",
			State:       val,
			DisplayType: []string{"text"},
			Icon:        "mdi:sensor-alert",
			Enabled:     true,
			Timestamp:   ts,
		})
	}

	// Temperature Sensor Status (if sensor has issues)
	if val, ok := parsed.SensorData["temperature_sensor_status"].(string); ok && val != "" {
		entities = append(entities, common.Entity{
			UniqueID:    common.GenerateUniqueID(model, devEUI, "temperature_sensor_status"),
			EntityID:    common.GenerateEntityID("sensor", orgSlug, Manufacturer, mdl, devEUI, "temperature_sensor_status"),
			EntityType:  "sensor",
			DeviceClass: "problem",
			Name:        "Temperature Sensor Status",
			State:       val,
			DisplayType: []string{"text"},
			Icon:        "mdi:sensor-alert",
			Enabled:     true,
			Timestamp:   ts,
		})
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
