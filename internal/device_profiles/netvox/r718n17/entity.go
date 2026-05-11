package r718n17

import (
	"fmt"
	"strings"
	"time"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
	"github.com/Space-DF/transformer-service/internal/lns"
)

func (p *NetvoxR718N17Component) ParsePayload(payload *common.RawPayload) (*common.ParsedData, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = lns.ExtractDevEUI(payload.Metadata, payload.LNSType)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI not found")
	}

	return &common.ParsedData{
		DeviceEUI:  devEUI,
		DeviceType: common.DeviceType(Model),
		Timestamp:  payload.Timestamp,
		SensorData: Decode(payload),
		RawData:    payload.Data,
	}, nil
}

func (p *NetvoxR718N17Component) ParseToEntities(orgSlug, model string, payload *common.RawPayload, deviceLocation *common.Location) ([]common.Entity, error) {
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

	// Location via trilateration (no built-in GPS)
	locationState := "unknown"
	attrs := map[string]interface{}{
		"source":               "trilateration",
		"requires_calculation": true,
		"gps_capable":          false,
		"device_model":         model,
	}
	if deviceLocation != nil {
		locationState = fmt.Sprintf("%f,%f", deviceLocation.Latitude, deviceLocation.Longitude)
		attrs["latitude"] = deviceLocation.Latitude
		attrs["longitude"] = deviceLocation.Longitude
	}
	entities = append(entities, common.Entity{
		UniqueID: common.GenerateUniqueID(model, devEUI, "location"),
		EntityID: common.GenerateEntityID(
			common.GetEntityDomain("location"),
			orgSlug, Manufacturer, mdl, devEUI, "location",
		),
		EntityType:  "location",
		DeviceClass: "location",
		Name:        "Location",
		State:       locationState,
		DisplayType: []string{"map"},
		Attributes:  attrs,
		Enabled:     true,
		Timestamp:   ts,
	})

	// Define sensor entities for 1-phase current sensor
	type sensorDef struct {
		key, name, entityType, devClass, unit, icon string
		display                                     []string
	}
	for _, def := range []sensorDef{
		{"battery_voltage", "Battery Voltage", "battery_voltage", "voltage", "V", "battery_voltage.svg", []string{"gauge", "value"}},
		{"low_battery", "Low Battery", "binary_sensor", "battery", "", "low_battery.svg", []string{"value"}},
		{"current_ma", "Current", "current", "current", "mA", "current.svg", []string{"chart", "gauge", "value"}},
		{"current_raw_ma", "Current (Raw)", "current_raw", "current", "mA", "current.svg", []string{"value"}},
		{"multiplier", "Multiplier", "multiplier", "", "", "multiplier.svg", []string{"value"}},
		{"config_status", "Config Status", "config_status", "", "", "config_status .svg", []string{"value"}},
		{"min_time_s", "Min Time Interval", "min_time_s", "duration", "s", "min_time_s.svg", []string{"value"}},
		{"max_time_s", "Max Time Interval", "max_time_s", "duration", "s", "max_time_s.svg", []string{"value"}},
		{"current_change_ma", "Current Change Threshold", "current_change_ma", "current", "mA", "current_change.svg", []string{"value"}},
		{"sw_version", "Software Version", "sw_version", "", "", "sw_version.svg", []string{"value"}},
		{"hw_version", "Hardware Version", "hw_version", "", "", "hw_version.svg", []string{"value"}},
		{"date_code", "Date Code", "date_code", "", "", "data_code.svg", []string{"value"}},
	} {
		val, ok := parsed.SensorData[def.key]
		if !ok {
			continue
		}
		entities = append(entities, common.Entity{
			UniqueID:    common.GenerateUniqueID(model, devEUI, def.key),
			EntityID:    common.GenerateEntityID(common.GetEntityDomain(def.key), orgSlug, Manufacturer, mdl, devEUI, def.key),
			EntityType:  def.entityType,
			DeviceClass: def.devClass,
			Name:        def.name,
			State:       val,
			DisplayType: def.display,
			UnitOfMeas:  def.unit,
			Icon:        def.icon,
			Enabled:     true,
			Timestamp:   ts,
		})
	}

	return entities, nil
}
