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
	return common.BuildEntitiesFromState(orgSlug, model, Manufacturer, mdl, devEUI, entityDefs(), parsed.SensorData, ts), nil
}

func (p *LSN50v2S31Component) GetEntityTemplates(model, devEUI string) []common.Entity {
	mdl := strings.ToLower(model)
	return common.BuildEntityTemplates("", model, Manufacturer, mdl, devEUI, entityDefs())
}

func entityDefs() []common.EntityDef {
	return []common.EntityDef{
		{Key: "battery_voltage", Name: "Battery Voltage", EntityType: "battery", DeviceClass: "voltage", UnitOfMeas: "V", Icon: "battery_voltage.svg", DisplayType: []string{"gauge", "value"}},
		{Key: "exti_trigger", Name: "EXTI Trigger", EntityType: "binary_sensor", Icon: "exti_trigger.svg", DisplayType: []string{"value"}},
		{Key: "door_status", Name: "Door Status", EntityType: "binary_sensor", DeviceClass: "door", Icon: "door_status.svg", DisplayType: []string{"value"}},
		{Key: "work_mode", Name: "Work Mode", EntityType: "work_mode", Icon: "work_mode.svg", DisplayType: []string{"value"}},
		{Key: "temperature_sht31", Name: "SHT31 Temperature", EntityType: "temperature", DeviceClass: "temperature", UnitOfMeas: "°C", Icon: "temperature.svg", DisplayType: []string{"chart", "gauge", "value"}},
		{Key: "humidity_sht31", Name: "SHT31 Humidity", EntityType: "humidity", DeviceClass: "humidity", UnitOfMeas: "%", Icon: "humidity.svg", DisplayType: []string{"chart", "gauge", "value"}},
		{Key: "data_time", Name: "Data Time", EntityType: "timestamp", Icon: "data_time.svg", DisplayType: []string{"value"}},
		{Key: "sht_temp_min", Name: "SHT Temp Min", EntityType: "temperature", DeviceClass: "temperature", UnitOfMeas: "°C", Icon: "sht_temp_min.svg", DisplayType: []string{"value"}},
		{Key: "sht_temp_max", Name: "SHT Temp Max", EntityType: "temperature", DeviceClass: "temperature", UnitOfMeas: "°C", Icon: "sht_temp_max.svg", DisplayType: []string{"value"}},
		{Key: "sht_hum_min", Name: "SHT Humidity Min", EntityType: "humidity", DeviceClass: "humidity", UnitOfMeas: "%", Icon: "sht_hum_min.svg", DisplayType: []string{"value"}},
		{Key: "sht_hum_max", Name: "SHT Humidity Max", EntityType: "humidity", DeviceClass: "humidity", UnitOfMeas: "%", Icon: "sht_hum_max.svg", DisplayType: []string{"value"}},
		{Key: "firmware_version", Name: "Firmware Version", EntityType: "firmware", Icon: "firmware_version.svg", DisplayType: []string{"value"}},
		{Key: "freq_band", Name: "Frequency Band", EntityType: "freq_band", Icon: "freq_band.svg", DisplayType: []string{"value"}},
		{Key: "tdc_sec", Name: "TDC Interval", EntityType: "duration", DeviceClass: "duration", UnitOfMeas: "s", Icon: "tdc_sec.svg", DisplayType: []string{"value"}},
	}
}
