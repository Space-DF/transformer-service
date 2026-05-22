package sensecap_t1000

import (
	"fmt"
	"strings"
	"time"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
	"github.com/Space-DF/transformer-service/internal/lns"
)

func (p *SenseCapT1000Component) ParsePayload(payload *common.RawPayload) (*common.ParsedData, error) {
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

func (p *SenseCapT1000Component) ParseToEntities(orgSlug, model string, payload *common.RawPayload, deviceLocation *common.Location) ([]common.Entity, error) {
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

func (p *SenseCapT1000Component) GetEntityTemplates(model, devEUI string) []common.Entity {
	mdl := strings.ToLower(model)
	entities := []common.Entity{
		common.BuildLocationTemplate("", model, Manufacturer, mdl, devEUI, true, false),
	}
	entities = append(entities, common.BuildEntityTemplates("", model, Manufacturer, mdl, devEUI, entityDefs())...)
	return entities
}

func entityDefs() []common.EntityDef {
	return []common.EntityDef{
		{Key: "battery_level", Name: "Battery Level", EntityType: "battery", DeviceClass: "battery", UnitOfMeas: "%", Icon: "battery_percent.svg", DisplayType: []string{"chart", "gauge", "value", "slider"}},
		{Key: "temperature", Name: "Temperature", EntityType: "temperature", DeviceClass: "temperature", UnitOfMeas: "°C", Icon: "temperature.svg", DisplayType: []string{"chart", "gauge", "value"}},
		{Key: "light", Name: "Light Level", EntityType: "sensor", DeviceClass: "illuminance", UnitOfMeas: "%", Icon: "light_level.svg", DisplayType: []string{"chart", "gauge", "value"}},
		{Key: "motion", Name: "Motion", EntityType: "binary_sensor", DeviceClass: "motion", Icon: "motion.svg", DisplayType: []string{"value"}},
		{Key: "shock_event", Name: "Shock Event", EntityType: "binary_sensor", DeviceClass: "vibration", Icon: "light_shock_event.svg", DisplayType: []string{"value"}},
		{Key: "sos_alert", Name: "SOS Alert", EntityType: "binary_sensor", DeviceClass: "safety", Icon: "sos_alert.svg", DisplayType: []string{"value"}},
		{Key: "temperature_event", Name: "Temperature Event", EntityType: "binary_sensor", DeviceClass: "heat", Icon: "temperature_event.svg", DisplayType: []string{"value"}},
		{Key: "light_event", Name: "Light Event", EntityType: "binary_sensor", DeviceClass: "light", Icon: "light_event.svg", DisplayType: []string{"value"}},
		{Key: "press_once_event", Name: "Press Once Event", EntityType: "binary_sensor", DeviceClass: "button", Icon: "press_once_event.svg", DisplayType: []string{"value"}},
	}
}
