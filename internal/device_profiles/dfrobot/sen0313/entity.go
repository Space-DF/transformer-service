package sen0313

import (
	"fmt"
	"strings"
	"time"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
	"github.com/Space-DF/transformer-service/internal/lns"
)

func (p *SEN0313Component) ParsePayload(payload *common.RawPayload) (*common.ParsedData, error) {
	deviceID := extractDeviceID(payload)
	if deviceID == "" {
		return nil, fmt.Errorf("device identifier not found")
	}

	return &common.ParsedData{
		DeviceEUI:  deviceID,
		DeviceType: common.DeviceType(Model),
		Timestamp:  payload.Timestamp,
		SensorData: Decode(payload),
		RawData:    payload.Data,
	}, nil
}

func (p *SEN0313Component) ParseToEntities(orgSlug, model string, payload *common.RawPayload, _ *common.Location) ([]common.Entity, error) {
	deviceID := extractDeviceID(payload)
	if deviceID == "" {
		return nil, fmt.Errorf("device identifier is required")
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
	return common.BuildEntitiesFromState(orgSlug, model, Manufacturer, mdl, deviceID, entityDefs(), parsed.SensorData, ts), nil
}

func (p *SEN0313Component) GetEntityTemplates(model, devEUI string) []common.Entity {
	mdl := strings.ToLower(model)
	return common.BuildEntityTemplates("", model, Manufacturer, mdl, devEUI, entityDefs())
}

func entityDefs() []common.EntityDef {
	return []common.EntityDef{
		{Key: "distance", DomainKey: "distance", Name: "Distance", EntityType: "distance", DeviceClass: "distance", UnitOfMeas: "cm", Icon: "water_depth.svg", DisplayType: []string{"chart", "gauge", "value"}},
	}
}

func extractDeviceID(payload *common.RawPayload) string {
	if payload == nil {
		return ""
	}
	if payload.DeviceEUI != "" {
		return payload.DeviceEUI
	}
	if devEUI := lns.ExtractDevEUI(payload.Metadata, payload.LNSType); devEUI != "" {
		return devEUI
	}

	for _, key := range []string{"device_id", "serial_number"} {
		if value, ok := payload.Metadata[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}

	return ""
}
