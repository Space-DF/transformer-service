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

	entities = append(entities, common.BuildEntitiesFromState(orgSlug, model, Manufacturer, mdl, devEUI, entityDefs(), parsed.SensorData, ts)...)

	return entities, nil
}

func (p *NetvoxR718N17Component) GetEntityTemplates(model, devEUI string) []common.Entity {
	mdl := strings.ToLower(model)
	entities := []common.Entity{
		common.BuildLocationTemplate("", model, Manufacturer, mdl, devEUI, false, true),
	}
	entities = append(entities, common.BuildEntityTemplates("", model, Manufacturer, mdl, devEUI, entityDefs())...)
	return entities
}

func entityDefs() []common.EntityDef {
	return []common.EntityDef{
		{Key: "battery_voltage", Name: "Battery Voltage", EntityType: "battery_voltage", DeviceClass: "voltage", UnitOfMeas: "V", Icon: "battery_voltage.svg", DisplayType: []string{"gauge", "value"}},
		{Key: "low_battery", Name: "Low Battery", EntityType: "binary_sensor", DeviceClass: "battery", Icon: "low_battery.svg", DisplayType: []string{"value"}},
		{Key: "current_ma", Name: "Current", EntityType: "current", DeviceClass: "current", UnitOfMeas: "mA", Icon: "current.svg", DisplayType: []string{"chart", "gauge", "value"}},
		{Key: "current_raw_ma", Name: "Current (Raw)", EntityType: "current_raw", DeviceClass: "current", UnitOfMeas: "mA", Icon: "current.svg", DisplayType: []string{"value"}},
		{Key: "multiplier", Name: "Multiplier", EntityType: "multiplier", Icon: "multiplier.svg", DisplayType: []string{"value"}},
		{Key: "config_status", Name: "Config Status", EntityType: "config_status", Icon: "config_status.svg", DisplayType: []string{"value"}},
		{Key: "min_time_s", Name: "Min Time Interval", EntityType: "min_time_s", DeviceClass: "duration", UnitOfMeas: "s", Icon: "min_time_s.svg", DisplayType: []string{"value"}},
		{Key: "max_time_s", Name: "Max Time Interval", EntityType: "max_time_s", DeviceClass: "duration", UnitOfMeas: "s", Icon: "max_time_s.svg", DisplayType: []string{"value"}},
		{Key: "current_change_ma", Name: "Current Change Threshold", EntityType: "current_change_ma", DeviceClass: "current", UnitOfMeas: "mA", Icon: "current_change.svg", DisplayType: []string{"value"}},
		{Key: "sw_version", Name: "Software Version", EntityType: "sw_version", Icon: "sw_version.svg", DisplayType: []string{"value"}},
		{Key: "hw_version", Name: "Hardware Version", EntityType: "hw_version", Icon: "hw_version.svg", DisplayType: []string{"value"}},
		{Key: "date_code", Name: "Date Code", EntityType: "date_code", Icon: "data_code.svg", DisplayType: []string{"value"}},
	}
}
