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
		key, name, entityType, devClass, unit string
		display                               []string
	}

	for _, def := range []sensorDef{
		{"total_volume", "Total Volume", "total_volume", "water", "L", []string{"chart", "value"}},
		{"total_heat", "Total Heat", "total_heat", "energy", "kCal", []string{"chart", "value"}},
		{"ambient_temperature", "Ambient Temperature", "temperature", "temperature", "°C", []string{"chart", "gauge", "value"}},
		{"water_temperature_min", "Water Temp Min", "temperature", "temperature", "°C", []string{"chart", "value"}},
		{"water_temperature_max", "Water Temp Max", "temperature", "temperature", "°C", []string{"chart", "value"}},
		{"battery_active", "Battery Voltage (Active)", "battery_voltage", "battery_voltage", "mV", []string{"chart", "gauge", "value", "slider"}},
		{"battery_recovered", "Battery Voltage (Recovered)", "battery_voltage", "battery_voltage", "mV", []string{"chart", "gauge", "value", "slider"}},
		{"error_code", "Error Code", "error_code", "problem", "", []string{"value"}},
		{"leak_state", "Leak State", "leak_state", "problem", "", []string{"value"}},
		{"is_sensing", "Is Sensing", "is_sensing", "problem", "", []string{"value"}},
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
			Enabled:     true,
			Timestamp:   ts,
		})
	}

	return entities, nil
}
