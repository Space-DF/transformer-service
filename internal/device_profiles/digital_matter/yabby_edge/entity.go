package yabby_edge

import (
	"fmt"
	"strings"
	"time"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
	"github.com/Space-DF/transformer-service/internal/lns"
)

func (p *YabbyEdgeComponent) ParsePayload(payload *common.RawPayload) (*common.ParsedData, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = lns.ExtractDevEUI(payload.Metadata, payload.LNSType)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI not found")
	}

	sensors, location := Decode(payload)

	return &common.ParsedData{
		DeviceEUI:  devEUI,
		DeviceType: common.DeviceType(Model),
		Timestamp:  payload.Timestamp,
		Location:   location,
		SensorData: sensors,
		RawData:    payload.Data,
	}, nil
}

func (p *YabbyEdgeComponent) ParseToEntities(orgSlug, model string, payload *common.RawPayload, deviceLocation *common.Location) ([]common.Entity, error) {
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

	loc := common.ResolveLocationBearing(parsed.Location, deviceLocation, parsed.SensorData)
	if loc != nil {
		entities = append(entities, common.Entity{
			UniqueID: common.GenerateUniqueID(model, devEUI, "location"),
			EntityID: common.GenerateEntityID(
				common.GetEntityDomain("location"),
				orgSlug, Manufacturer, mdl, devEUI, "location",
			),
			EntityType:  "location",
			DeviceClass: "location",
			Name:        "Location",
			State:       "home",
			DisplayType: []string{"map"},
			Attributes: map[string]interface{}{
				"source":       common.LocationSource(parsed.Location),
				"gps_capable":  true,
				"device_model": model,
				"latitude":     loc.Latitude,
				"longitude":    loc.Longitude,
				"bearing":      loc.Bearing,
			},
			Enabled:   true,
			Timestamp: ts,
		})
	}

	entities = append(entities, common.BuildEntitiesFromState(orgSlug, model, Manufacturer, mdl, devEUI, entityDefs(), parsed.SensorData, ts)...)

	return entities, nil
}

func (p *YabbyEdgeComponent) GetEntityTemplates(model, devEUI string) []common.Entity {
	mdl := strings.ToLower(model)
	entities := []common.Entity{
		common.BuildLocationTemplate("", model, Manufacturer, mdl, devEUI, true, false),
	}
	entities = append(entities, common.BuildEntityTemplates("", model, Manufacturer, mdl, devEUI, entityDefs())...)
	return entities
}

func entityDefs() []common.EntityDef {
	return []common.EntityDef{
		{Key: "battery_voltage", Name: "Battery Voltage", EntityType: "battery", DeviceClass: "voltage", UnitOfMeas: "V", DisplayType: []string{"chart", "gauge", "value"}},
		{Key: "battery_level", Name: "Battery Level", EntityType: "battery", DeviceClass: "battery", UnitOfMeas: "%", DisplayType: []string{"chart", "gauge", "value", "slider"}},
		{Key: "trip_status", Name: "Trip Status", EntityType: "binary_sensor", DeviceClass: "moving", Icon: "trip_status.svg", DisplayType: []string{"value"}},
		{Key: "inactivity", Name: "Inactivity", EntityType: "binary_sensor", DeviceClass: "problem", Icon: "inactivity.svg", DisplayType: []string{"value"}},
		{Key: "trip_count", Name: "Trip Count", EntityType: "sensor", DeviceClass: "trip_count", Icon: "trip_count.svg", DisplayType: []string{"chart", "value"}},
		{Key: "firmware", Name: "Firmware Version", EntityType: "sensor", DeviceClass: "firmware", Icon: "firmware.svg", DisplayType: []string{"value"}},
		{Key: "battery_stats", Name: "Battery Statistics", EntityType: "sensor", DeviceClass: "battery_stats", Icon: "battery_stats.svg", DisplayType: []string{"value"}},
		{Key: "downlink_ack", Name: "Downlink ACK", EntityType: "sensor", DeviceClass: "status", Icon: "data_code.svg", DisplayType: []string{"value"}},
		{Key: "connect", Name: "Connection Status", EntityType: "sensor", DeviceClass: "connectivity", Icon: "connect.svg", DisplayType: []string{"value"}},
	}
}
