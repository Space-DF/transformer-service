package common

import (
	"encoding/base64"
	"fmt"
	"math"
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
	return fmt.Sprintf("%s_%s_%s", model, devEUI, entityType)
}

// GenerateEntityID creates a descriptive entity ID.
func GenerateEntityID(domain, orgSlug, manufacturer, model, devEUI, entityType string) string {
	return fmt.Sprintf("%s.%s_%s_%s_%s_%s",
		domain, orgSlug, manufacturer, model, devEUI, entityType)
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
