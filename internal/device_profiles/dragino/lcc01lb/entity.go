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
	var entities []common.Entity

	type sensorDef struct {
		key, name, entityType, devClass, unit, icon string
		display                                     []string
	}

	for _, def := range []sensorDef{
		{"battery_voltage", "Battery Voltage", "battery_voltage", "battery", "V", "battery_voltage.svg", []string{"chart", "gauge", "value", "slider"}},
		{"actual_weight_g", "Actual Weight", "weight", "weight", "g", "actual_weight.svg", []string{"chart", "gauge", "value"}},
		{"weight_reading", "Weight Reading", "weight_reading", "weight", "", "weight_reading.svg", []string{"chart", "value"}},
		{"weight_state", "Weight State", "weight_state", "enum", "", "weight_state.svg", []string{"value"}},
		{"scale_factor", "Scale Factor", "scale_factor", "scale_factor", "", "scale_factor.svg", []string{"value"}},
		{"weight_flag", "Weight Flag", "weight_flag", "weight_flag", "", "weight_flag.svg", []string{"value"}},
		{"mod", "MOD", "mod", "", "", "mod.svg", []string{"value"}},
		{"sensor_model", "Sensor Model", "sensor_model", "", "", "sensor_model.svg", []string{"value"}},
		{"firmware_version", "Firmware Version", "firmware", "", "", "firmware_version.svg", []string{"value"}},
		{"frequency_band", "Frequency Band", "freq_band", "", "", "frequency_band.svg", []string{"value"}},
		{"sub_band", "Sub Band", "sub_band", "", "", "sub_band.svg", []string{"value"}},
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
