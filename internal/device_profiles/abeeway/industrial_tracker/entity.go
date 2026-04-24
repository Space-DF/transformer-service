package industrial_tracker

import (
	"fmt"
	"strings"
	"time"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
	"github.com/Space-DF/transformer-service/internal/lns"
)

func (p *AbeewayComponent) ParsePayload(payload *common.RawPayload) (*common.ParsedData, error) {
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

func (p *AbeewayComponent) ParseToEntities(orgSlug, model string, payload *common.RawPayload, deviceLocation *common.Location) ([]common.Entity, error) {
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

	loc := parsed.Location
	if loc == nil {
		loc = deviceLocation
	}
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
			},
			Enabled:   true,
			Timestamp: ts,
		})
	}

	type sensorDef struct {
		key, name, entityType, devClass, unit, icon string
		display                                     []string
	}
	for _, def := range []sensorDef{
		{"battery_voltage", "Battery Voltage", "battery", "battery", "V", "mdi:battery", []string{"chart", "gauge", "value"}},
		{"battery_percent", "Battery Level", "battery", "battery", "%", "mdi:battery", []string{"chart", "gauge", "value", "slider"}},
		{"temperature", "Temperature", "temperature", "temperature", "°C", "mdi:thermometer", []string{"chart", "gauge", "value"}},
		{"speed", "Speed", "sensor", "speed", "km/h", "mdi:speedometer", []string{"gauge", "value"}},
		{"heading", "Heading", "sensor", "heading", "deg", "mdi:compass", []string{"value"}},
		{"sos_alert", "SOS Alert", "binary_sensor", "safety", "", "mdi:alert-circle", []string{"value"}},
		{"motion", "Motion", "binary_sensor", "motion", "", "mdi:run-fast", []string{"value"}},
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
