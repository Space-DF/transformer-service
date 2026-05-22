package ht

import (
	"fmt"
	"strings"
	"time"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
	"github.com/Space-DF/transformer-service/internal/lns"
)

func (p *MclimateHTComponent) ParsePayload(payload *common.RawPayload) (*common.ParsedData, error) {
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

func (p *MclimateHTComponent) ParseToEntities(orgSlug, model string, payload *common.RawPayload, deviceLocation *common.Location) ([]common.Entity, error) {
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

	// Location via trilateration (no built-in GPS).
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

func (p *MclimateHTComponent) GetEntityTemplates(model, devEUI string) []common.Entity {
	mdl := strings.ToLower(model)
	entities := []common.Entity{
		common.BuildLocationTemplate("", model, Manufacturer, mdl, devEUI, false, true),
	}
	entities = append(entities, common.BuildEntityTemplates("", model, Manufacturer, mdl, devEUI, entityDefs())...)
	return entities
}

func entityDefs() []common.EntityDef {
	return []common.EntityDef{
		{Key: "temperature", Name: "Temperature", EntityType: "temperature", DeviceClass: "temperature", UnitOfMeas: "°C", Icon: "temperature.svg", DisplayType: []string{"chart", "gauge", "value"}},
		{Key: "humidity", Name: "Humidity", EntityType: "humidity", DeviceClass: "humidity", UnitOfMeas: "%", Icon: "humidity.svg", DisplayType: []string{"chart", "gauge", "value"}},
		{Key: "battery_voltage", Name: "Battery Voltage", EntityType: "battery_voltage", DeviceClass: "battery_voltage", UnitOfMeas: "V", Icon: "battery_voltage.svg", DisplayType: []string{"chart", "gauge", "value"}},
		{Key: "battery", Name: "Battery", EntityType: "battery", DeviceClass: "battery", UnitOfMeas: "%", Icon: "battery_percent.svg", DisplayType: []string{"gauge", "value"}},
		{Key: "occupancy", Name: "Occupancy", EntityType: "occupancy", DeviceClass: "occupancy", Icon: "occupancy.svg", DisplayType: []string{"value"}},
		{Key: "pir_trigger_count", Name: "PIR Trigger Count", EntityType: "pir_trigger_count", Icon: "pir_trigger_count.svg", DisplayType: []string{"value"}},
	}
}
