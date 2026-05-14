package g62

import (
	"fmt"
	"strings"
	"time"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
	"github.com/Space-DF/transformer-service/internal/lns"
)

func (p *G62Component) ParsePayload(payload *common.RawPayload) (*common.ParsedData, error) {
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

func (p *G62Component) ParseToEntities(orgSlug, model string, payload *common.RawPayload, deviceLocation *common.Location) ([]common.Entity, error) {
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

func (p *G62Component) GetEntityTemplates(model, devEUI string) []common.Entity {
	mdl := strings.ToLower(model)
	entities := []common.Entity{
		common.BuildLocationTemplate("", model, Manufacturer, mdl, devEUI, true, false),
	}
	entities = append(entities, common.BuildEntityTemplates("", model, Manufacturer, mdl, devEUI, entityDefs())...)
	return entities
}

func entityDefs() []common.EntityDef {
	return []common.EntityDef{
		{Key: "heading", Name: "Heading", EntityType: "heading", UnitOfMeas: "°", Icon: "direction.svg", DisplayType: []string{"value"}},
		{Key: "speed", Name: "Speed", EntityType: "speed", DeviceClass: "speed", UnitOfMeas: "km/h", Icon: "speed.svg", DisplayType: []string{"chart", "gauge", "value"}},
		{Key: "battery_voltage", Name: "Battery Voltage", EntityType: "battery", DeviceClass: "voltage", UnitOfMeas: "V", Icon: "battery_voltage.svg", DisplayType: []string{"chart", "gauge", "value"}},
		{Key: "external_voltage", Name: "External Voltage", EntityType: "sensor", DeviceClass: "voltage", UnitOfMeas: "V", Icon: "external_voltage.svg", DisplayType: []string{"chart", "gauge", "value"}},
		{Key: "analog_input", Name: "Analog Input", EntityType: "sensor", DeviceClass: "voltage", UnitOfMeas: "V", Icon: "analog_input.svg", DisplayType: []string{"chart", "value"}},
		{Key: "temperature", Name: "Temperature", EntityType: "temperature", DeviceClass: "temperature", UnitOfMeas: "°C", Icon: "temperature.svg", DisplayType: []string{"chart", "gauge", "value"}},
		{Key: "gps_accuracy", Name: "GPS Accuracy", EntityType: "sensor", DeviceClass: "distance", UnitOfMeas: "m", Icon: "gps_accuracy.svg", DisplayType: []string{"value"}},
		{Key: "trip_type", Name: "Trip Type", EntityType: "sensor", Icon: "trip_type.svg", DisplayType: []string{"value"}},
		{Key: "ignition", Name: "Ignition", EntityType: "binary_sensor", DeviceClass: "power", Icon: "ignition.svg", DisplayType: []string{"value"}},
		{Key: "dig_in_1", Name: "Digital Input 1", EntityType: "binary_sensor", Icon: "dig_out.svg", DisplayType: []string{"value"}},
		{Key: "dig_in_2", Name: "Digital Input 2", EntityType: "binary_sensor", Icon: "dig_out.svg", DisplayType: []string{"value"}},
		{Key: "dig_out", Name: "Digital Output", EntityType: "switch", Icon: "dig_out.svg", DisplayType: []string{"toggle"}},
		{Key: "odometer", Name: "Odometer", EntityType: "sensor", DeviceClass: "distance", UnitOfMeas: "km", Icon: "odometer.svg", DisplayType: []string{"chart", "value"}},
		{Key: "runtime", Name: "Runtime", EntityType: "sensor", DeviceClass: "duration", Icon: "runtime.svg", DisplayType: []string{"value"}},
		{Key: "firmware", Name: "Firmware Version", EntityType: "sensor", DeviceClass: "firmware", Icon: "firmware.svg", DisplayType: []string{"value"}},
		{Key: "downlink_ack", Name: "Downlink ACK", EntityType: "sensor", DeviceClass: "status", Icon: "data_code.svg", DisplayType: []string{"value"}},
	}
}
