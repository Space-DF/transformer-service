package netvox_r809ag

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
	var entities []common.Entity

	// Add all entities, including switch, in the loop.
	type sensorDef struct {
		key, name, entityType, devClass, unit string
		display                               []string
	}
	for _, def := range []sensorDef{
		{"switch", "Outlet", "switch", "outlet", "", []string{"toggle"}},
		{"voltage", "Voltage", "voltage", "voltage", "V", []string{"chart", "value"}},
		{"current", "Current", "current", "current", "mA", []string{"chart", "gauge", "value"}},
		{"power", "Power", "power", "power", "W", []string{"chart", "gauge", "value"}},
		{"energy", "Energy", "energy", "energy", "Wh", []string{"chart", "value"}},
		{"overcurrent_alarm", "Over Current Alarm", "switch", "power", "", []string{"switch"}},
		{"dash_current_alarm", "Dash Current Alarm", "switch", "power", "", []string{"switch"}},
		{"power_off_alarm", "Power Off Alarm", "switch", "power", "", []string{"switch"}},
	} {
		val, ok := parsed.SensorData[def.key]
		if !ok {
			continue
		}
		entity := common.Entity{
			UniqueID:    common.GenerateUniqueID(model, devEUI, def.key),
			EntityID:    common.GenerateEntityID(common.GetEntityDomain(def.key), orgSlug, Manufacturer, mdl, devEUI, def.key),
			EntityType:  def.entityType,
			DeviceClass: def.devClass,
			Name:        def.name,
			State:       val,
			DisplayType: def.display,
			UnitOfMeas:  def.unit,
			Enabled:     true,
			Timestamp:   ts,
		}
		entities = append(entities, entity)
	}

	return entities, nil
}
