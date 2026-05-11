package cubicmeter

import (
	"fmt"
	"strings"
	"time"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
	"github.com/Space-DF/transformer-service/internal/lns"
)

func (p *CubicMeterComponent) ParsePayload(payload *common.RawPayload) (*common.ParsedData, error) {
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

func (p *CubicMeterComponent) ParseToEntities(orgSlug, model string, payload *common.RawPayload, deviceLocation *common.Location) ([]common.Entity, error) {
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

	type sensorDef struct {
		key, name, entityType, devClass, unit, icon string
		display                                     []string
	}

	for _, def := range []sensorDef{
		{"total_volume", "Total Volume", "total_volume", "water", "L", "total_volume.svg", []string{"chart", "value"}},
		{"total_heat", "Total Heat", "total_heat", "energy", "kCal", "total_heat.svg", []string{"chart", "value"}},
		{"ambient_temperature", "Ambient Temperature", "temperature", "temperature", "°C", "ambient_temperature.svg", []string{"chart", "gauge", "value"}},
		{"water_temperature_min", "Water Temp Min", "temperature", "temperature", "°C", "water_temperature_min.svg", []string{"chart", "value"}},
		{"water_temperature_max", "Water Temp Max", "temperature", "temperature", "°C", "water_temperature_max.svg", []string{"chart", "value"}},
		{"battery_active", "Battery Voltage (Active)", "battery_voltage", "battery_voltage", "mV", "battery_active.svg", []string{"chart", "gauge", "value", "slider"}},
		{"battery_recovered", "Battery Voltage (Recovered)", "battery_voltage", "battery_voltage", "mV", "battery_recovered.svg", []string{"chart", "gauge", "value", "slider"}},
		{"error_code", "Error Code", "error_code", "problem", "", "data_code.svg", []string{"value"}},
		{"leak_state", "Leak State", "leak_state", "problem", "", "water_depth.svg", []string{"value"}},
		{"is_sensing", "Is Sensing", "is_sensing", "problem", "", "sensor_model.svg", []string{"value"}},
	} {
		val, ok := parsed.SensorData[def.key]
		if !ok {
			continue
		}
		entities = append(entities, common.Entity{
			UniqueID:    common.GenerateUniqueID(model, devEUI, def.key),
			EntityID:    common.GenerateEntityID(common.GetEntityDomain(def.entityType), orgSlug, Manufacturer, mdl, devEUI, def.key),
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
