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
		{"heading", "Heading", "heading", "", "°", "mdi:compass", []string{"value"}},
		{"speed", "Speed", "speed", "speed", "km/h", "mdi:speedometer", []string{"chart", "gauge", "value"}},
		{"battery_voltage", "Battery Voltage", "battery", "voltage", "V", "mdi:battery", []string{"chart", "gauge", "value"}},
		{"external_voltage", "External Voltage", "sensor", "voltage", "V", "mdi:flash", []string{"chart", "gauge", "value"}},
		{"analog_input", "Analog Input", "sensor", "voltage", "V", "mdi:analog", []string{"chart", "value"}},
		{"temperature", "Temperature", "temperature", "temperature", "°C", "mdi:thermometer", []string{"chart", "gauge", "value"}},
		{"gps_accuracy", "GPS Accuracy", "sensor", "distance", "m", "mdi:crosshairs-gps", []string{"value"}},
		{"trip_type", "Trip Type", "sensor", "", "", "mdi:car", []string{"value"}},
		{"ignition", "Ignition", "binary_sensor", "power", "", "mdi:engine", []string{"value"}},
		{"dig_in_1", "Digital Input 1", "binary_sensor", "", "", "mdi:electric-switch", []string{"value"}},
		{"dig_in_2", "Digital Input 2", "binary_sensor", "", "", "mdi:electric-switch", []string{"value"}},
		{"dig_out", "Digital Output", "switch", "", "", "mdi:electric-switch-closed", []string{"toggle"}},
		{"odometer", "Odometer", "sensor", "distance", "km", "mdi:counter", []string{"chart", "value"}},
		{"runtime", "Runtime", "sensor", "duration", "", "mdi:timer", []string{"value"}},
		{"firmware", "Firmware Version", "sensor", "firmware", "", "mdi:information", []string{"value"}},
		{"downlink_ack", "Downlink ACK", "sensor", "status", "", "mdi:check-circle", []string{"value"}},
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
