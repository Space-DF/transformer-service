package r809ag

import (
	"fmt"
	"strings"
	"time"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
	"github.com/Space-DF/transformer-service/internal/lns"
)

// ParsePayload decodes the raw R809AG uplink into a ParsedData struct.
func (p *R809AGComponent) ParsePayload(payload *common.RawPayload) (*common.ParsedData, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = lns.ExtractDevEUI(payload.Metadata, payload.LNSType)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI not found")
	}

	sensors := Decode(payload)

	return &common.ParsedData{
		DeviceEUI:  devEUI,
		DeviceType: common.DeviceType(Model),
		Timestamp:  payload.Timestamp,
		SensorData: sensors,
		RawData:    payload.Data,
	}, nil
}

// ParseToEntities converts a raw R809AG uplink into Home-Assistant-style entities.
func (p *R809AGComponent) ParseToEntities(orgSlug, model string, payload *common.RawPayload, _ *common.Location) ([]common.Entity, error) {
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

func (p *R809AGComponent) GetEntityTemplates(model, devEUI string) []common.Entity {
	mdl := strings.ToLower(model)
	return common.BuildEntityTemplates("", model, Manufacturer, mdl, devEUI, entityDefs())
}

func entityDefs() []common.EntityDef {
	return []common.EntityDef{
		{Key: "switch", Name: "Outlet", EntityType: "switch", DeviceClass: "outlet", Icon: "switch.svg", DisplayType: []string{"toggle"}},
		{Key: "voltage", Name: "Voltage", EntityType: "voltage", DeviceClass: "voltage", UnitOfMeas: "V", Icon: "external_voltage.svg", DisplayType: []string{"chart", "value"}},
		{Key: "current", Name: "Current", EntityType: "current", DeviceClass: "current", UnitOfMeas: "mA", Icon: "current.svg", DisplayType: []string{"chart", "gauge", "value"}},
		{Key: "power", Name: "Power", EntityType: "power", DeviceClass: "power", UnitOfMeas: "W", Icon: "power.svg", DisplayType: []string{"chart", "gauge", "value"}},
		{Key: "energy", Name: "Energy", EntityType: "energy", DeviceClass: "energy", UnitOfMeas: "Wh", Icon: "energy.svg", DisplayType: []string{"chart", "value"}},
		{Key: "overcurrent_alarm", Name: "Over Current Alarm", EntityType: "switch", DeviceClass: "power", Icon: "over_current_alarm.svg", DisplayType: []string{"switch"}},
		{Key: "dash_current_alarm", Name: "Dash Current Alarm", EntityType: "switch", DeviceClass: "power", Icon: "dash_current_alarm.svg", DisplayType: []string{"switch"}},
		{Key: "power_off_alarm", Name: "Power Off Alarm", EntityType: "switch", DeviceClass: "power", Icon: "power_off_alarm.svg", DisplayType: []string{"switch"}},
	}
}
