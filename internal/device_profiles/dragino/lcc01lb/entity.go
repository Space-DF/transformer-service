package lcc01lb

import (
	"fmt"
	"strings"
	"time"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
	"github.com/Space-DF/transformer-service/internal/lns"
)

func (p *LCC01LBComponent) ParsePayload(payload *common.RawPayload) (*common.ParsedData, error) {
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

func (p *LCC01LBComponent) ParseToEntities(orgSlug, model string, payload *common.RawPayload, deviceLocation *common.Location) ([]common.Entity, error) {
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

func (p *LCC01LBComponent) GetEntityTemplates(model, devEUI string) []common.Entity {
	mdl := strings.ToLower(model)
	return common.BuildEntityTemplates("", model, Manufacturer, mdl, devEUI, entityDefs())
}

func entityDefs() []common.EntityDef {
	return []common.EntityDef{
		{Key: "battery_voltage", DomainKey: "battery_voltage", Name: "Battery Voltage", EntityType: "battery_voltage", DeviceClass: "battery", UnitOfMeas: "V", Icon: "battery_voltage.svg", DisplayType: []string{"chart", "gauge", "value", "slider"}},
		{Key: "actual_weight_g", DomainKey: "weight", Name: "Actual Weight", EntityType: "weight", DeviceClass: "weight", UnitOfMeas: "g", Icon: "actual_weight.svg", DisplayType: []string{"chart", "gauge", "value"}},
		{Key: "weight_reading", DomainKey: "weight_reading", Name: "Weight Reading", EntityType: "weight_reading", DeviceClass: "weight", Icon: "weight_reading.svg", DisplayType: []string{"chart", "value"}},
		{Key: "weight_state", DomainKey: "weight_state", Name: "Weight State", EntityType: "weight_state", DeviceClass: "enum", Icon: "weight_state.svg", DisplayType: []string{"value"}},
		{Key: "scale_factor", DomainKey: "scale_factor", Name: "Scale Factor", EntityType: "scale_factor", DeviceClass: "scale_factor", Icon: "scale_factor.svg", DisplayType: []string{"value"}},
		{Key: "weight_flag", DomainKey: "weight_flag", Name: "Weight Flag", EntityType: "weight_flag", DeviceClass: "weight_flag", Icon: "weight_flag.svg", DisplayType: []string{"value"}},
		{Key: "mod", DomainKey: "mod", Name: "MOD", EntityType: "mod", Icon: "mod.svg", DisplayType: []string{"value"}},
		{Key: "sensor_model", DomainKey: "sensor_model", Name: "Sensor Model", EntityType: "sensor_model", Icon: "sensor_model.svg", DisplayType: []string{"value"}},
		{Key: "firmware_version", DomainKey: "firmware", Name: "Firmware Version", EntityType: "firmware", Icon: "firmware_version.svg", DisplayType: []string{"value"}},
		{Key: "frequency_band", DomainKey: "freq_band", Name: "Frequency Band", EntityType: "freq_band", Icon: "frequency_band.svg", DisplayType: []string{"value"}},
		{Key: "sub_band", DomainKey: "sub_band", Name: "Sub Band", EntityType: "sub_band", Icon: "sub_band.svg", DisplayType: []string{"value"}},
	}
}
