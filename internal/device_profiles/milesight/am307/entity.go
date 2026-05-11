package am307

import (
	"fmt"
	"strings"
	"time"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
	"github.com/Space-DF/transformer-service/internal/lns"
)

func (p *AM307Component) ParsePayload(payload *common.RawPayload) (*common.ParsedData, error) {
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

func (p *AM307Component) ParseToEntities(orgSlug, model string, payload *common.RawPayload, deviceLocation *common.Location) ([]common.Entity, error) {
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

	type sensorDef struct {
		key, name, entityType, devClass, unit, icon string
		display                                     []string
	}
	for _, def := range []sensorDef{
		{"temperature", "Temperature", "temperature", "temperature", "°C", "temperature.svg", []string{"chart", "gauge", "value"}},
		{"humidity", "Humidity", "humidity", "humidity", "%", "humidity.svg", []string{"chart", "gauge", "value"}},
		{"battery", "Battery", "battery", "battery", "%", "battery_percent.svg", []string{"gauge", "value"}},
		{"occupancy", "Occupancy", "occupancy", "occupancy", "", "occupancy.svg", []string{"value"}},
		{"pir_sensor_value", "PIR Sensor Value", "pir_sensor_value", "", "", "pir_sensor_value.svg", []string{"value"}},
		{"pir_sensor_status", "PIR Sensor Status", "pir_sensor_status", "", "", "pir_trigger_status.svg", []string{"value"}},
		{"light_level", "Light Level", "light_level", "illuminance", "lx", "light_level.svg", []string{"chart", "gauge", "value"}},
		{"co2", "CO2", "co2", "carbon_dioxide", "ppm", "co2.svg", []string{"chart", "gauge", "value"}},
		{"tvoc", "TVOC", "tvoc", "volatile_organic_compounds", "", "volatile_organic_compounds.svg", []string{"chart", "gauge", "value"}},
		{"pressure", "Pressure", "pressure", "pressure", "hPa", "pressure.svg", []string{"chart", "gauge", "value"}},
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
