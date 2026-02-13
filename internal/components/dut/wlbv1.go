package dut

import (
	"encoding/hex"
	"fmt"

	"github.com/Space-DF/transformer-service/internal/components"
)

const coordScaleWLB = 1e7

// WLBV1Parser handles parsing of WLB V1 device payloads
type WLBV1Parser struct{}

// NewWLBV1Parser creates a new WLB V1 parser
func NewWLBV1Parser() *WLBV1Parser {
	return &WLBV1Parser{}
}

// ParsePayload parses WLB V1 device payload and extracts GPS coordinates
func (p *WLBV1Parser) ParsePayload(payload *components.RawPayload) (*components.ParsedData, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = components.ExtractDevEUI(payload.Metadata)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI not found")
	}

	// Try multiple parsing strategies
	var err error
	var location *components.Location

	location, err = p.parseFromFrmPayload(payload.Metadata)
	if err != nil {
		location, err = p.parseFromDecodedPayload(payload.Metadata)
	}
	if err != nil {
		location, err = p.parseFromObjectField(payload.Metadata)
	}
	if err != nil {
		return nil, fmt.Errorf("WLB V1 parsing not yet implemented: %w", err)
	}

	sensorData := p.extractObjectData(payload.Metadata)

	return &components.ParsedData{
		DeviceEUI:  devEUI,
		DeviceType: components.DeviceTypeWLBV1,
		Timestamp:  payload.Timestamp,
		Location:   location,
		SensorData: sensorData,
		RawData:    payload.Data,
	}, nil
}

// SupportsGPS returns true since WLB V1 can have GPS capability depending on configuration
func (p *WLBV1Parser) SupportsGPS() bool {
	return true // WLB V1 can support GPS
}

// GetSupportedPorts returns the fPorts typically used by WLB V1
func (p *WLBV1Parser) GetSupportedPorts() []int {
	return []int{1, 2, 3, 4, 5} // WLB V1 supports multiple fPorts
}

// GetSupportedEntityTypes returns entity types supported by WLB V1
func (p *WLBV1Parser) GetSupportedEntityTypes() []string {
	return []string{"location", "battery", "water_depth"} // WLB V1 environmental sensor
}

// ParseToEntities creates entities for WLB V1 device
func (p *WLBV1Parser) ParseToEntities(orgSlug, model string, payload *components.RawPayload, deviceLocation *components.Location) ([]components.Entity, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = components.ExtractDevEUI(payload.Metadata)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI is required")
	}

	parsedData, err := p.ParsePayload(payload)
	if err != nil {
		return nil, err
	}

	objectData := p.extractObjectData(payload.Metadata)
	var entities []components.Entity
	timestamp := payload.Timestamp

	// Location Entity
	if parsedData.Location != nil {
		locationEntity := components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "location"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("location"),
				orgSlug, "dut", "wlb_v1", devEUI, "location",
			),
			EntityType:  "location",
			DeviceClass: "location",
			Name:        "Location",
			State:       "home",
			DisplayType: []string{"map"},
			Attributes: map[string]interface{}{
				"source":       "gps",
				"gps_capable":  true,
				"device_model": "WLB_V1",
				"latitude":     parsedData.Location.Latitude,
				"longitude":    parsedData.Location.Longitude,
			},
			Enabled:   true,
			Timestamp: timestamp,
		}
		entities = append(entities, locationEntity)
	}

	// Battery Entity (vBat from object)
	if vBat, ok := objectData["vBat"].(float64); ok {
		batteryEntity := components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "battery"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("battery"),
				orgSlug, "dut", "wlb_v1", devEUI, "battery",
			),
			EntityType:  "battery",
			DeviceClass: "battery",
			Name:        "Battery Voltage",
			State:       vBat,
			DisplayType: []string{"chart", "gauge", "value", "slider"},
			UnitOfMeas:  "V",
			Timestamp:   timestamp,
			Enabled:     true,
		}
		entities = append(entities, batteryEntity)
	}

	// Water Level Entity (waterlevel_cm from object)
	if waterDepth, ok := objectData["waterlevel_cm"].(float64); ok {
		waterLevelEntity := components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "water_depth"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("water_depth"),
				orgSlug, "dut", "wlb_v1", devEUI, "water_depth",
			),
			EntityType:  "water_depth",
			DeviceClass: "distance",
			Name:        "Water Depth",
			State:       waterDepth,
			DisplayType: []string{"chart", "gauge", "value", "slider"},
			Attributes: map[string]interface{}{
				"sensor_height_from_ground": 200,
			},
			UnitOfMeas: "cm",
			Timestamp:  timestamp,
			Enabled:    true,
		}
		entities = append(entities, waterLevelEntity)
	}

	return entities, nil
}

// parseFromFrmPayload extracts GPS coordinates from hex frm_payload
func (p *WLBV1Parser) parseFromFrmPayload(metadata map[string]interface{}) (*components.Location, error) {
	var frmPayload string

	if uplink, ok := metadata["uplink_message"].(map[string]interface{}); ok {
		frmPayload, _ = uplink["frm_payload"].(string)
	}
	if frmPayload == "" {
		frmPayload, _ = metadata["frm_payload"].(string)
	}

	if frmPayload == "" {
		return nil, fmt.Errorf("frm_payload not found")
	}

	payloadBytes, err := hex.DecodeString(frmPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode hex payload: %w", err)
	}

	if len(payloadBytes) < 8 {
		return nil, fmt.Errorf("payload too short for GPS data: %d bytes", len(payloadBytes))
	}

	lat, lng, err := p.parseGPSCoordinates(payloadBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GPS coordinates: %w", err)
	}

	return &components.Location{
		Latitude:  lat,
		Longitude: lng,
	}, nil
}

// parseFromDecodedPayload extracts GPS coordinates from already decoded payload
func (p *WLBV1Parser) parseFromDecodedPayload(metadata map[string]interface{}) (*components.Location, error) {
	var decoded map[string]interface{}
	var ok bool

	if decoded, ok = metadata["decoded_payload"].(map[string]interface{}); !ok {
		if decoded, ok = metadata["decoded_raw_data"].(map[string]interface{}); !ok {
			return nil, fmt.Errorf("no decoded payload data found")
		}
	}

	var lat, lng float64
	var found bool

	if v, ok := decoded["latitude"].(float64); ok {
		if w, ok := decoded["longitude"].(float64); ok {
			lat, lng = v, w
			found = true
		}
	}
	if !found {
		if v, ok := decoded["lat"].(float64); ok {
			if w, ok := decoded["lng"].(float64); ok {
				lat, lng = v, w
				found = true
			}
		}
	}
	if !found {
		if gps, ok := decoded["gps"].(map[string]interface{}); ok {
			if v, ok := gps["latitude"].(float64); ok {
				if w, ok := gps["longitude"].(float64); ok {
					lat, lng = v, w
					found = true
				}
			}
		}
	}

	if !found {
		return nil, fmt.Errorf("GPS coordinates not found in decoded payload")
	}

	if err := p.validateCoordinates(lat, lng); err != nil {
		return nil, err
	}

	return &components.Location{
		Latitude:  lat,
		Longitude: lng,
	}, nil
}

// parseGPSCoordinates extracts GPS coordinates from WLB V1 payload bytes
func (p *WLBV1Parser) parseGPSCoordinates(payloadBytes []byte) (float64, float64, error) {
	if len(payloadBytes) < 8 {
		return 0, 0, fmt.Errorf("insufficient data for GPS coordinates")
	}

	latInt := int32(payloadBytes[0]) | int32(payloadBytes[1])<<8 | int32(payloadBytes[2])<<16 | int32(payloadBytes[3])<<24
	lonInt := int32(payloadBytes[4]) | int32(payloadBytes[5])<<8 | int32(payloadBytes[6])<<16 | int32(payloadBytes[7])<<24

	lat := float64(latInt) / coordScaleWLB
	lng := float64(lonInt) / coordScaleWLB

	if err := p.validateCoordinates(lat, lng); err != nil {
		return 0, 0, err
	}

	return lat, lng, nil
}

// validateCoordinates validates GPS coordinates
func (p *WLBV1Parser) validateCoordinates(latitude, longitude float64) error {
	if latitude == 0.0 && longitude == 0.0 {
		return fmt.Errorf("GPS coordinates are 0,0 - no GPS fix available")
	}
	if latitude < -90 || latitude > 90 {
		return fmt.Errorf("invalid latitude: %f", latitude)
	}
	if longitude < -180 || longitude > 180 {
		return fmt.Errorf("invalid longitude: %f", longitude)
	}
	return nil
}

// parseFromObjectField extracts GPS coordinates from decoded_raw_data.object field
func (p *WLBV1Parser) parseFromObjectField(metadata map[string]interface{}) (*components.Location, error) {
	var objectData map[string]interface{}

	// Try decoded_raw_data.object first
	if decodedRaw, ok := metadata["decoded_raw_data"].(map[string]interface{}); ok {
		if obj, ok := decodedRaw["object"].(map[string]interface{}); ok {
			objectData = obj
		}
	}

	// Try object directly
	if objectData == nil {
		if obj, ok := metadata["object"].(map[string]interface{}); ok {
			objectData = obj
		}
	}

	if objectData == nil {
		return nil, fmt.Errorf("object field not found")
	}

	// Extract latitude and longitude
	lat, latOk := objectData["latitude"].(float64)
	lng, lngOk := objectData["longitude"].(float64)

	if !latOk || !lngOk {
		return nil, fmt.Errorf("latitude or longitude not found in object")
	}

	if err := p.validateCoordinates(lat, lng); err != nil {
		return nil, err
	}

	return &components.Location{
		Latitude:  lat,
		Longitude: lng,
	}, nil
}

// extractObjectData extracts all data from the "object" field
func (p *WLBV1Parser) extractObjectData(metadata map[string]interface{}) map[string]interface{} {
	var objectData map[string]interface{}

	// Try decoded_raw_data.object first
	if decodedRaw, ok := metadata["decoded_raw_data"].(map[string]interface{}); ok {
		if obj, ok := decodedRaw["object"].(map[string]interface{}); ok {
			objectData = obj
		}
	}

	// Try object directly
	if objectData == nil {
		if obj, ok := metadata["object"].(map[string]interface{}); ok {
			objectData = obj
		}
	}

	if objectData == nil {
		return make(map[string]interface{})
	}

	return objectData
}
