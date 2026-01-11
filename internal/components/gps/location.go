package gps

import (
	"time"

	"github.com/Space-DF/transformer-service/internal/components"
)

// BuildLocationEntity creates a location entity with proper entity_id and all attributes
func BuildLocationEntity(orgSlug, manufacturer, model, devEUI string,
	location *Location, positionType PositionType, timestamp time.Time) components.Entity {

	attrs := map[string]interface{}{
		"source":       string(positionType),
		"gps_capable":  true,
		"device_model": model,
		"latitude":     location.Latitude,
		"longitude":    location.Longitude,
	}

	// Add optional fields if present
	if location.Altitude != 0 {
		attrs["altitude"] = location.Altitude
	}
	if location.Accuracy != 0 {
		attrs["accuracy"] = location.Accuracy
	}
	if location.Speed != 0 {
		attrs["speed"] = location.Speed
	}
	if location.Heading != 0 {
		attrs["heading"] = location.Heading
	}

	return components.Entity{
		UniqueID: components.GenerateUniqueID(model, devEUI, "location"),
		EntityID: components.GenerateEntityID(
			components.GetEntityDomain("location"),
			orgSlug, manufacturer, model, devEUI, "location",
		),
		EntityType:  "location",
		DeviceClass: "location",
		Name:        "Location",
		State:       "home",
		DisplayType:  []string{"map"},
		Attributes:  attrs,
		Enabled:     true,
		Timestamp:   timestamp,
	}
}
