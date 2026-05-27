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
	return common.BuildEntitiesFromState(orgSlug, model, Manufacturer, mdl, devEUI, entityDefs(), parsed.SensorData, ts), nil
}

func (p *CubicMeterComponent) GetEntityTemplates(model, devEUI string) []common.Entity {
	mdl := strings.ToLower(model)
	return common.BuildEntityTemplates("", model, Manufacturer, mdl, devEUI, entityDefs())
}

func entityDefs() []common.EntityDef {
	return []common.EntityDef{
		{Key: "total_volume", DomainKey: "total_volume", Name: "Total Volume", EntityType: "total_volume", DeviceClass: "water", UnitOfMeas: "L", Icon: "total_volume.svg", DisplayType: []string{"chart", "value"}},
		{Key: "total_heat", DomainKey: "total_heat", Name: "Total Heat", EntityType: "total_heat", DeviceClass: "energy", UnitOfMeas: "kCal", Icon: "total_heat.svg", DisplayType: []string{"chart", "value"}},
		{Key: "ambient_temperature", DomainKey: "temperature", Name: "Ambient Temperature", EntityType: "temperature", DeviceClass: "temperature", UnitOfMeas: "°C", Icon: "ambient_temperature.svg", DisplayType: []string{"chart", "gauge", "value"}},
		{Key: "water_temperature_min", DomainKey: "temperature", Name: "Water Temp Min", EntityType: "temperature", DeviceClass: "temperature", UnitOfMeas: "°C", Icon: "water_temperature_min.svg", DisplayType: []string{"chart", "value"}},
		{Key: "water_temperature_max", DomainKey: "temperature", Name: "Water Temp Max", EntityType: "temperature", DeviceClass: "temperature", UnitOfMeas: "°C", Icon: "water_temperature_max.svg", DisplayType: []string{"chart", "value"}},
		{Key: "battery_active", DomainKey: "battery_voltage", Name: "Battery Voltage (Active)", EntityType: "battery_voltage", DeviceClass: "battery_voltage", UnitOfMeas: "mV", Icon: "battery_active.svg", DisplayType: []string{"chart", "gauge", "value", "slider"}},
		{Key: "battery_recovered", DomainKey: "battery_voltage", Name: "Battery Voltage (Recovered)", EntityType: "battery_voltage", DeviceClass: "battery_voltage", UnitOfMeas: "mV", Icon: "battery_recovered.svg", DisplayType: []string{"chart", "gauge", "value", "slider"}},
		{Key: "error_code", DomainKey: "error_code", Name: "Error Code", EntityType: "error_code", DeviceClass: "problem", Icon: "data_code.svg", DisplayType: []string{"value"}},
		{Key: "leak_state", DomainKey: "leak_state", Name: "Leak State", EntityType: "leak_state", DeviceClass: "problem", Icon: "water_depth.svg", DisplayType: []string{"value"}},
		{Key: "is_sensing", DomainKey: "is_sensing", Name: "Is Sensing", EntityType: "is_sensing", DeviceClass: "problem", Icon: "sensor_model.svg", DisplayType: []string{"value"}},
	}
}
