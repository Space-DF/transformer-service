package common

import (
	"encoding/base64"
	"fmt"
	"math"
	"strings"
	"time"
)

// ValidateCoordinates checks that lat/lon are within valid ranges and not null-island.
func ValidateCoordinates(lat, lon float64) error {
	if math.Abs(lat) > 90 || math.Abs(lon) > 180 {
		return fmt.Errorf("coordinates out of valid range")
	}
	if lat == 0 && lon == 0 {
		return fmt.Errorf("null island coordinates")
	}
	return nil
}

// DecodePayloadBytes decodes a base64-encoded payload string to bytes.
func DecodePayloadBytes(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, fmt.Errorf("empty payload data")
	}
	if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil && len(decoded) > 0 {
		return decoded, nil
	}
	return nil, fmt.Errorf("failed to decode payload as base64")
}

// GenerateUniqueID creates a simple registry key for an entity.
func GenerateUniqueID(model, devEUI, entityType string) string {
	return fmt.Sprintf("%s_%s_%s", model, normalizeDevEUI(devEUI), entityType)
}

// GenerateEntityID creates a descriptive entity ID.
func GenerateEntityID(domain, orgSlug, manufacturer, model, devEUI, entityType string) string {
	return fmt.Sprintf("%s.%s_%s_%s_%s_%s",
		domain, orgSlug, manufacturer, model, normalizeDevEUI(devEUI), entityType)
}

func normalizeDevEUI(devEUI string) string {
	return strings.ToUpper(strings.TrimSpace(devEUI))
}

// GetEntityDomain returns the HA domain for a given entity type key.
func GetEntityDomain(entityType string) string {
	switch entityType {
	case "location":
		return "device_tracker"
	case "battery", "temperature", "humidity", "pressure", "water_depth":
		return "sensor"
	case "motion", "door", "window":
		return "binary_sensor"
	case "light", "switch":
		return entityType
	default:
		return "sensor"
	}
}

// CreateDeviceInfo builds a DeviceInfo struct.
func CreateDeviceInfo(devEUI, name, manufacturer, model, modelID string) DeviceInfo {
	return DeviceInfo{
		Identifiers:  []string{devEUI},
		Name:         name,
		Manufacturer: manufacturer,
		Model:        model,
		ModelID:      modelID,
	}
}

// ExtractGPS looks for latitude/longitude in a decoded field map using common key names.
// Returns nil if no valid coordinates are found.
func ExtractGPS(src map[string]interface{}) *Location {
	for _, latKey := range []string{"latitude", "lat"} {
		for _, lonKey := range []string{"longitude", "lon", "lng"} {
			lat, latOK := src[latKey].(float64)
			lon, lonOK := src[lonKey].(float64)
			if latOK && lonOK && ValidateCoordinates(lat, lon) == nil {
				return &Location{Latitude: lat, Longitude: lon}
			}
		}
	}
	return nil
}

// LocationSource returns "gps" when the device itself provided a valid fix,
// or "gateway" when the location was computed from gateway trilateration as a fallback.
func LocationSource(deviceGPS *Location) string {
	if deviceGPS != nil {
		return "gps"
	}
	return "gateway"
}

// ResolveLocationBearing returns the preferred bearing for a location, using a device-reported
// heading when present and otherwise falling back to the computed device location bearing.
func ResolveLocationBearing(parsedLocation, deviceLocation *Location, sensorData map[string]interface{}) *Location {
	var source *Location
	switch {
	case parsedLocation != nil:
		source = parsedLocation
	case deviceLocation != nil:
		source = deviceLocation
	default:
		return nil
	}

	resolved := *source

	if heading, ok := extractBearingValue(sensorData, "heading"); ok {
		resolved.Bearing = normalizeBearing(heading)
		return &resolved
	}

	if bearing, ok := extractBearingValue(sensorData, "bearing"); ok {
		resolved.Bearing = normalizeBearing(bearing)
		return &resolved
	}

	if deviceLocation != nil {
		resolved.Bearing = deviceLocation.Bearing
	}

	return &resolved
}

func extractBearingValue(sensorData map[string]interface{}, key string) (float64, bool) {
	if sensorData == nil {
		return 0, false
	}

	raw, exists := sensorData[key]
	if !exists {
		return 0, false
	}

	value, ok := raw.(float64)
	if !ok {
		return 0, false
	}

	return value, true
}

func normalizeBearing(value float64) float64 {
	normalized := math.Mod(value, 360)
	if normalized < 0 {
		normalized += 360
	}
	return normalized

}

func BuildLocationTemplate(orgSlug, model, manufacturer, modelKey, devEUI string, gpsCapable bool, requiresCalculation bool) Entity {
	source := "gps"
	if requiresCalculation {
		source = "trilateration"
	}

	return Entity{
		Key:      "location",
		UniqueID: GenerateUniqueID(model, devEUI, "location"),
		EntityID: GenerateEntityID(
			GetEntityDomain("location"),
			orgSlug, manufacturer, modelKey, devEUI, "location",
		),
		EntityType:  "location",
		DeviceClass: "location",
		Name:        "Location",
		State:       "unknown",
		DisplayType: []string{"map"},
		Attributes: map[string]interface{}{
			"source":               source,
			"gps_capable":          gpsCapable,
			"requires_calculation": requiresCalculation,
			"device_model":         model,
		},
		Enabled:   true,
		Timestamp: time.Time{},
	}
}

func BuildEntityTemplates(orgSlug, model, manufacturer, modelKey, devEUI string, defs []EntityDef) []Entity {
	entities := make([]Entity, 0, len(defs))
	for _, def := range defs {
		entities = append(entities, BuildEntityFromDef(orgSlug, model, manufacturer, modelKey, devEUI, def, nil, nil, time.Time{}))
	}
	return entities
}

func BuildEntitiesFromState(orgSlug, model, manufacturer, modelKey, devEUI string, defs []EntityDef, sensorData map[string]interface{}, ts time.Time) []Entity {
	entities := make([]Entity, 0, len(defs))
	for _, def := range defs {
		value, ok := sensorData[def.Key]
		if !ok {
			continue
		}
		if def.Skip != nil && def.Skip(value) {
			continue
		}

		state := value
		attributes := cloneAttributes(def.Attributes)
		if def.Transform != nil {
			var transformed map[string]any
			state, transformed, ok = def.Transform(value)
			if !ok {
				continue
			}
			for k, v := range transformed {
				if attributes == nil {
					attributes = make(map[string]interface{}, len(transformed))
				}
				attributes[k] = v
			}
		}

		entities = append(entities, BuildEntityFromDef(
			orgSlug, model, manufacturer, modelKey, devEUI, def, state, attributes, ts,
		))
	}
	return entities
}

func BuildEntityFromDef(orgSlug, model, manufacturer, modelKey, devEUI string, def EntityDef, state interface{}, attributes map[string]interface{}, ts time.Time) Entity {
	domainKey := def.DomainKey
	if domainKey == "" {
		domainKey = def.Key
	}
	if attributes == nil {
		attributes = cloneAttributes(def.Attributes)
	}

	return Entity{
		Key:         def.Key,
		UniqueID:    GenerateUniqueID(model, devEUI, def.Key),
		EntityID:    GenerateEntityID(GetEntityDomain(domainKey), orgSlug, manufacturer, modelKey, devEUI, def.Key),
		EntityType:  def.EntityType,
		DeviceClass: def.DeviceClass,
		Name:        def.Name,
		State:       state,
		DisplayType: def.DisplayType,
		UnitOfMeas:  def.UnitOfMeas,
		Icon:        def.Icon,
		Attributes:  attributes,
		Enabled:     true,
		Timestamp:   ts,
	}
}

func cloneAttributes(attributes map[string]interface{}) map[string]interface{} {
	if len(attributes) == 0 {
		return nil
	}

	cloned := make(map[string]interface{}, len(attributes))
	for key, value := range attributes {
		cloned[key] = value
	}
	return cloned
}
