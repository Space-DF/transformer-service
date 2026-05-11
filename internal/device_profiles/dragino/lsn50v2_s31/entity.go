package lsn50v2_s31

import (
	"fmt"
	"strings"
	"time"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
	"github.com/Space-DF/transformer-service/internal/lns"
)

func (p *LSN50v2S31Component) ParsePayload(payload *common.RawPayload) (*common.ParsedData, error) {
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

func (p *LSN50v2S31Component) ParseToEntities(orgSlug, model string, payload *common.RawPayload, _ *common.Location) ([]common.Entity, error) {
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
		{"battery_voltage", "Battery Voltage", "battery", "voltage", "V", "battery_voltage.svg", []string{"gauge", "value"}},
		{"exti_trigger", "EXTI Trigger", "binary_sensor", "", "", "exti_trigger.svg", []string{"value"}},
		{"door_status", "Door Status", "binary_sensor", "door", "", "door_status.svg", []string{"value"}},
		{"work_mode", "Work Mode", "work_mode", "", "", "work_mode.svg", []string{"value"}},
		{"temperature_sht31", "SHT31 Temperature", "temperature", "temperature", "°C", "temperature.svg", []string{"chart", "gauge", "value"}},
		{"humidity_sht31", "SHT31 Humidity", "humidity", "humidity", "%", "humidity.svg", []string{"chart", "gauge", "value"}},
		{"data_time", "Data Time", "timestamp", "", "", "data_time.svg", []string{"value"}},
		{"sht_temp_min", "SHT Temp Min", "temperature", "temperature", "°C", "sht_temp_min.svg", []string{"value"}},
		{"sht_temp_max", "SHT Temp Max", "temperature", "temperature", "°C", "sht_temp_max.svg", []string{"value"}},
		{"sht_hum_min", "SHT Humidity Min", "humidity", "humidity", "%", "sht_hum_min.svg", []string{"value"}},
		{"sht_hum_max", "SHT Humidity Max", "humidity", "humidity", "%", "sht_hum_max.svg", []string{"value"}},
		{"firmware_version", "Firmware Version", "firmware", "", "", "firmware_version.svg", []string{"value"}},
		{"freq_band", "Frequency Band", "freq_band", "", "", "freq_band.svg", []string{"value"}},
		{"tdc_sec", "TDC Interval", "duration", "duration", "s", "tdc_sec.svg", []string{"value"}},
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
