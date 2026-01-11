package gps

import (
	"fmt"
	"math"

	"github.com/Space-DF/transformer-service/internal/components"
)

// Location represents GPS coordinates with metadata
type Location struct {
	Latitude  float64
	Longitude float64
	Altitude  float64
	Accuracy  float64
	Speed     float64
	Heading   float64
}

// PositionType represents the type of position fix
type PositionType string

const (
	PositionTypeGPS         PositionType = "GPS"
	PositionTypeWiFi        PositionType = "WiFi"
	PositionTypeBLE         PositionType = "BLE"
	PositionTypeGPSTimeout  PositionType = "GPS_TIMEOUT"
	PositionTypeWiFiFailure PositionType = "WIFI_FAILURE"
	PositionTypeBLEFailure  PositionType = "BLE_FAILURE"
	PositionTypeUnknown     PositionType = "UNKNOWN"
)

// ValidateCoordinates validates GPS coordinates
func ValidateCoordinates(latitude, longitude float64) error {
	if latitude == 0.0 && longitude == 0.0 {
		return fmt.Errorf("GPS coordinates are 0,0 - no GPS fix available")
	}
	if latitude < -90 || latitude > 90 {
		return fmt.Errorf("invalid latitude: %f (must be between -90 and 90)", latitude)
	}
	if longitude < -180 || longitude > 180 {
		return fmt.Errorf("invalid longitude: %f (must be between -180 and 180)", longitude)
	}
	return nil
}

// ValidateAltitude validates altitude value
func ValidateAltitude(altitude float64) error {
	// Altitude can be negative (below sea level) or positive
	// Reasonable range: -500m (Dead Sea) to 8848m (Mount Everest)
	if altitude < -500 || altitude > 10000 {
		return fmt.Errorf("invalid altitude: %f (must be between -500 and 10000 meters)", altitude)
	}
	return nil
}

// ValidateAccuracy validates horizontal accuracy value
func ValidateAccuracy(accuracy float64) error {
	// Accuracy should be positive (in meters)
	// GPS accuracy can be up to several kilometers in poor conditions
	if accuracy < 0 {
		return fmt.Errorf("invalid accuracy: %f (must be non-negative)", accuracy)
	}
	if accuracy > 10000 {
		return fmt.Errorf("invalid accuracy: %f (must be less than 10000 meters)", accuracy)
	}
	return nil
}

// ValidateSpeed validates speed value
func ValidateSpeed(speed float64) error {
	if speed < 0 {
		return fmt.Errorf("invalid speed: %f (must be non-negative)", speed)
	}
	// Max reasonable speed: 1000 m/s (~3600 km/h, faster than most aircraft)
	if speed > 1000 {
		return fmt.Errorf("invalid speed: %f (must be less than 1000 m/s)", speed)
	}
	return nil
}

// ValidateHeading validates heading value
func ValidateHeading(heading float64) error {
	// Heading is 0-360 degrees
	if heading < 0 || heading >= 360 {
		return fmt.Errorf("invalid heading: %f (must be between 0 and 360 degrees)", heading)
	}
	return nil
}

// CalculateDistance calculates distance between two coordinates in meters using Haversine formula
func CalculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000 // meters

	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLat := (lat2 - lat1) * math.Pi / 180
	deltaLon := (lon2 - lon1) * math.Pi / 180

	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadius * c
}

// CalculateBearing calculates bearing between two coordinates in degrees
func CalculateBearing(lat1, lon1, lat2, lon2 float64) float64 {
	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLon := (lon2 - lon1) * math.Pi / 180

	y := math.Sin(deltaLon) * math.Cos(lat2Rad)
	x := math.Cos(lat1Rad)*math.Sin(lat2Rad) -
		math.Sin(lat1Rad)*math.Cos(lat2Rad)*math.Cos(deltaLon)

	bearing := math.Atan2(y, x) * 180 / math.Pi
	if bearing < 0 {
		bearing += 360
	}

	return bearing
}

// ParseGPSCoordinates extracts GPS coordinates from payload bytes.
// Parses int32 values (latitude/longitude scaled by 10^7) from 8-byte payload.
func ParseGPSCoordinates(payloadBytes []byte) (float64, float64, error) {
	if len(payloadBytes) < 8 {
		return 0, 0, fmt.Errorf("insufficient data for GPS coordinates")
	}

	latInt := int32(payloadBytes[0]) | int32(payloadBytes[1])<<8 | int32(payloadBytes[2])<<16 | int32(payloadBytes[3])<<24
	lonInt := int32(payloadBytes[4]) | int32(payloadBytes[5])<<8 | int32(payloadBytes[6])<<16 | int32(payloadBytes[7])<<24

	lat := float64(latInt) / 10000000.0
	lon := float64(lonInt) / 10000000.0

	if err := ValidateCoordinates(lat, lon); err != nil {
		return 0, 0, err
	}

	return lat, lon, nil
}

// ExtractGPSFromDecodedPayload extracts GPS from decoded_payload metadata.
// Handles multiple field name variations: latitude/longitude, lat/lng, gps.latitude/gps.longitude.
func ExtractGPSFromDecodedPayload(metadata map[string]interface{}) (*components.Location, error) {
	var decoded map[string]interface{}
	var ok bool

	if decoded, ok = metadata["decoded_payload"].(map[string]interface{}); !ok {
		if decoded, ok = metadata["decoded_raw_data"].(map[string]interface{}); !ok {
			return nil, fmt.Errorf("no decoded payload data found")
		}
	}

	var lat, lon float64
	var found bool

	// Try latitude/longitude
	if v, ok := decoded["latitude"].(float64); ok {
		if w, ok := decoded["longitude"].(float64); ok {
			lat, lon = v, w
			found = true
		}
	}
	// Try lat/lng
	if !found {
		if v, ok := decoded["lat"].(float64); ok {
			if w, ok := decoded["lng"].(float64); ok {
				lat, lon = v, w
				found = true
			}
		}
	}
	// Try gps.latitude/gps.longitude
	if !found {
		if gps, ok := decoded["gps"].(map[string]interface{}); ok {
			if v, ok := gps["latitude"].(float64); ok {
				if w, ok := gps["longitude"].(float64); ok {
					lat, lon = v, w
					found = true
				}
			}
		}
	}

	if !found {
		return nil, fmt.Errorf("GPS coordinates not found in decoded payload")
	}

	if err := ValidateCoordinates(lat, lon); err != nil {
		return nil, err
	}

	return &components.Location{
		Latitude:  lat,
		Longitude: lon,
	}, nil
}
