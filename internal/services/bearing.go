package services

import (
	"math"
)

const minBearingDistanceMeters = 5.0

// Coordinate represents a geographic coordinate
type Coordinate struct {
	Latitude  float64
	Longitude float64
}

// CalculateBearing calculates the bearing from point A to point B
// Returns bearing in degrees (0-360), where 0 is North, 90 is East, etc.
func calculateBearing(from, to Coordinate) float64 {
	// Convert to radians
	lat1 := from.Latitude * math.Pi / 180
	lon1 := from.Longitude * math.Pi / 180
	lat2 := to.Latitude * math.Pi / 180
	lon2 := to.Longitude * math.Pi / 180

	dLon := lon2 - lon1

	y := math.Sin(dLon) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(dLon)

	bearing := math.Atan2(y, x)

	// Convert to degrees
	bearing = bearing * 180 / math.Pi

	// Normalize to 0-360
	if bearing < 0 {
		bearing += 360
	}

	return bearing
}

func CalculateBearingFromPoints(points []LocationEntry) float64 {
	if len(points) < 2 {
		return 0
	}

	var sumX, sumY float64

	for i := 1; i < len(points); i++ {
		b := calculateBearing(
			Coordinate{
				Latitude:  points[i].Latitude,
				Longitude: points[i].Longitude,
			},
			Coordinate{
				Latitude:  points[i-1].Latitude,
				Longitude: points[i-1].Longitude,
			},
		)

		rad := b * math.Pi / 180
		sumX += math.Cos(rad)
		sumY += math.Sin(rad)
	}

	avg := math.Atan2(sumY, sumX) * 180 / math.Pi
	return math.Mod(avg+360, 360)
}

func FilterPoints(points []LocationEntry) []LocationEntry {
	if len(points) < 2 {
		return points
	}

	var result []LocationEntry
	result = append(result, points[0])

	for i := 1; i < len(points); i++ {
		if distanceMeters(result[len(result)-1], points[i]) >= minBearingDistanceMeters {
			result = append(result, points[i])
		}
	}

	return result
}

func distanceMeters(a, b LocationEntry) float64 {
	const earthRadiusMeters = 6371000.0

	lat1 := a.Latitude * math.Pi / 180
	lon1 := a.Longitude * math.Pi / 180
	lat2 := b.Latitude * math.Pi / 180
	lon2 := b.Longitude * math.Pi / 180

	dLat := lat2 - lat1
	dLon := lon2 - lon1

	sinLat := math.Sin(dLat / 2)
	sinLon := math.Sin(dLon / 2)

	h := sinLat*sinLat + math.Cos(lat1)*math.Cos(lat2)*sinLon*sinLon
	return 2 * earthRadiusMeters * math.Atan2(math.Sqrt(h), math.Sqrt(1-h))
}
