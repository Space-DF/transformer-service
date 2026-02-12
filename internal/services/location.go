package services

import (
	"fmt"
	"math"

	"github.com/Space-DF/transformer-service/internal/models"
)

const (
	EarthRadius              = 6371000.0 // Earth radius in meters
	DefaultTxPower           = 14.0      // Default transmission power in dBm
	DefaultPathLossExponent  = 4.0       // Default path loss exponent
	DefaultReferenceDistance = 1.0       // Default reference distance in meters
)

// LocationService handles device location calculations
type LocationService struct{}

// NewLocationService creates a new location service
func NewLocationService() *LocationService {
	return &LocationService{}
}

// CalculateDeviceLocation calculates device location based on gateway data
func (ls *LocationService) CalculateDeviceLocation(payload map[string]interface{}) (*models.DeviceLocationData, error) {
	var rxMetadata []interface{}
	var frequency float64
	var devEUI string

	// Check if this is TTN format (uplink_message) or ChirpStack format (direct payload)
	if uplinkMessage, ok := payload["uplink_message"].(map[string]interface{}); ok {
		// TTN format
		var rxOk bool
		rxMetadata, rxOk = uplinkMessage["rx_metadata"].([]interface{})
		if !rxOk {
			return nil, fmt.Errorf("rx_metadata not found in uplink_message")
		}

		settings, settingsOk := uplinkMessage["settings"].(map[string]interface{})
		if !settingsOk {
			return nil, fmt.Errorf("settings not found in uplink_message")
		}

		var freqOk bool
		frequency, freqOk = settings["frequency"].(float64)
		if !freqOk {
			return nil, fmt.Errorf("frequency not found in settings")
		}

		endDeviceIDs, deviceOk := payload["end_device_ids"].(map[string]interface{})
		if !deviceOk {
			return nil, fmt.Errorf("end_device_ids not found in payload")
		}

		var euiOk bool
		devEUI, euiOk = endDeviceIDs["dev_eui"].(string)
		if !euiOk {
			return nil, fmt.Errorf("dev_eui not found in end_device_ids")
		}
	} else {
		// ChirpStack/custom format - check for rxInfo in multiple locations
		var rxOk bool

		// First try top-level rxInfo
		rxMetadata, rxOk = payload["rxInfo"].([]interface{})

		// If not found, try uplinkEvent.rxInfo
		if !rxOk {
			if uplinkEvent, uplinkOk := payload["uplinkEvent"].(map[string]interface{}); uplinkOk {
				rxMetadata, rxOk = uplinkEvent["rxInfo"].([]interface{})
			}
		}

		if !rxOk {
			return nil, fmt.Errorf("rxInfo not found in payload")
		}

		// Extract frequency from txInfo (optional for some formats)
		frequency = 868.0 // Default frequency if not found
		if txInfo, txOk := payload["txInfo"].(map[string]interface{}); txOk {
			if freq, freqOk := txInfo["frequency"].(float64); freqOk {
				frequency = freq
			}
		} else if uplinkEvent, uplinkOk := payload["uplinkEvent"].(map[string]interface{}); uplinkOk {
			if txInfo, txOk := uplinkEvent["txInfo"].(map[string]interface{}); txOk {
				if freq, freqOk := txInfo["frequency"].(float64); freqOk {
					frequency = freq
				}
			}
		}

		// Extract device EUI from deviceInfo
		deviceInfo, deviceOk := payload["deviceInfo"].(map[string]interface{})
		if !deviceOk {
			return nil, fmt.Errorf("deviceInfo not found in payload")
		}

		var euiOk bool
		devEUI, euiOk = deviceInfo["devEui"].(string)
		if !euiOk {
			return nil, fmt.Errorf("devEui not found in deviceInfo")
		}
	}

	// Parse gateway locations
	var locations []models.LocationPoint
	for _, gw := range rxMetadata {
		gateway, ok := gw.(map[string]interface{})
		if !ok {
			continue
		}

		locationData, ok := gateway["location"].(map[string]interface{})
		if !ok {
			continue
		}

		lat, latOk := locationData["latitude"].(float64)
		lon, lonOk := locationData["longitude"].(float64)
		rssi, rssiOk := gateway["rssi"].(float64)

		if latOk && lonOk && rssiOk {
			locations = append(locations, models.LocationPoint{
				Latitude:  lat,
				Longitude: lon,
				RSSI:      int(rssi),
			})
		}
	}

	if len(locations) == 0 {
		return nil, fmt.Errorf("no valid gateway locations found")
	}

	// Calculate device location based on number of gateways
	var lat, lon float64
	var err error

	switch len(locations) {
	case 1:
		// Single gateway - return gateway location
		lat, lon = locations[0].Latitude, locations[0].Longitude
	case 2:
		lat, lon, err = ls.calculateLocationTwoGateways(locations, frequency)
	case 3:
		lat, lon, err = ls.calculateLocationThreeGateways(locations, frequency)
	default:
		// More than 3 gateways - use least squares
		lat, lon, err = ls.calculateLocationMultipleGateways(locations, frequency)
	}

	if err != nil {
		return nil, err
	}

	return &models.DeviceLocationData{
		Latitude:  roundToDecimals(lat, 6),
		Longitude: roundToDecimals(lon, 6),
		DevEUI:    devEUI,
	}, nil
}

// findDistance calculates distance based on RSSI and frequency
func (ls *LocationService) findDistance(rssi int, frequency float64) float64 {
	freqMHz := frequency / 1_000_000
	plD0 := 32.45 + 20*math.Log10(freqMHz)
	pl := DefaultTxPower - float64(rssi)
	logD := (pl - plD0) / (10 * DefaultPathLossExponent)
	return DefaultReferenceDistance * math.Pow(10, logD)
}

// latLonToXY converts lat/lon to local XY coordinates
func (ls *LocationService) latLonToXY(lat, lon, refLat, refLon float64) (float64, float64) {
	x := EarthRadius * math.Pi / 180 * (lon - refLon) * math.Cos(math.Pi/180*refLat)
	y := EarthRadius * math.Pi / 180 * (lat - refLat)
	return x, y
}

// xyToLatLon converts local XY coordinates to lat/lon
func (ls *LocationService) xyToLatLon(x, y, refLat, refLon float64) (float64, float64) {
	lat := refLat + (180/math.Pi)*(y/EarthRadius)
	lon := refLon + (180/math.Pi)*(x/(EarthRadius*math.Cos(math.Pi/180*refLat)))
	return lat, lon
}

// calculateLocationTwoGateways calculates location using intersection zone of two circles
func (ls *LocationService) calculateLocationTwoGateways(locations []models.LocationPoint, frequency float64) (float64, float64, error) {
	refLat, refLon := locations[0].Latitude, locations[0].Longitude

	// Convert gateway positions to XY coordinates
	p1X, p1Y := ls.latLonToXY(locations[0].Latitude, locations[0].Longitude, refLat, refLon)
	p2X, p2Y := ls.latLonToXY(locations[1].Latitude, locations[1].Longitude, refLat, refLon)

	// Get initial distances from RSSI
	distances := ls.expandRadiiUntilIntersection(locations, frequency, refLat, refLon)
	d1, d2 := distances[0], distances[1]

	// Calculate intersection zone
	zone := ls.calculateCircleIntersection(p1X, p1Y, d1, p2X, p2Y, d2)

	if !zone.Valid {
		// Fallback: return weighted center based on signal strength
		weight1 := math.Pow(10, float64(locations[0].RSSI)/20) // Convert dBm to linear scale
		weight2 := math.Pow(10, float64(locations[1].RSSI)/20)
		totalWeight := weight1 + weight2

		if totalWeight > 0 {
			x := (p1X*weight1 + p2X*weight2) / totalWeight
			y := (p1Y*weight1 + p2Y*weight2) / totalWeight
			lat, lon := ls.xyToLatLon(x, y, refLat, refLon)
			return lat, lon, nil
		}

		// Final fallback: geometric center
		x := (p1X + p2X) / 2
		y := (p1Y + p2Y) / 2
		lat, lon := ls.xyToLatLon(x, y, refLat, refLon)
		return lat, lon, nil
	}

	// Return the centroid of the intersection zone
	lat, lon := ls.xyToLatLon(zone.CentroidX, zone.CentroidY, refLat, refLon)
	return lat, lon, nil
}

// calculateLocationThreeGateways calculates location using intersection zone of three circles
func (ls *LocationService) calculateLocationThreeGateways(locations []models.LocationPoint, frequency float64) (float64, float64, error) {
	refLat, refLon := locations[0].Latitude, locations[0].Longitude

	// Convert gateway positions to XY coordinates
	p1X, p1Y := ls.latLonToXY(locations[0].Latitude, locations[0].Longitude, refLat, refLon)
	p2X, p2Y := ls.latLonToXY(locations[1].Latitude, locations[1].Longitude, refLat, refLon)
	p3X, p3Y := ls.latLonToXY(locations[2].Latitude, locations[2].Longitude, refLat, refLon)

	// Get expanded distances to ensure intersection
	distances := ls.expandRadiiUntilIntersection(locations, frequency, refLat, refLon)
	d1, d2, d3 := distances[0], distances[1], distances[2]

	// Find pairwise intersections
	zone12 := ls.calculateCircleIntersection(p1X, p1Y, d1, p2X, p2Y, d2)
	zone13 := ls.calculateCircleIntersection(p1X, p1Y, d1, p3X, p3Y, d3)
	zone23 := ls.calculateCircleIntersection(p2X, p2Y, d2, p3X, p3Y, d3)

	validZones := 0
	centroidX, centroidY := 0.0, 0.0

	// Calculate confidence-weighted centroid of all valid intersection zones
	if zone12.Valid {
		weight := zone12.Area * zone12.Confidence
		centroidX += zone12.CentroidX * weight
		centroidY += zone12.CentroidY * weight
		validZones++
	}
	if zone13.Valid {
		weight := zone13.Area * zone13.Confidence
		centroidX += zone13.CentroidX * weight
		centroidY += zone13.CentroidY * weight
		validZones++
	}
	if zone23.Valid {
		weight := zone23.Area * zone23.Confidence
		centroidX += zone23.CentroidX * weight
		centroidY += zone23.CentroidY * weight
		validZones++
	}

	if validZones > 0 {
		totalWeight := 0.0
		if zone12.Valid {
			totalWeight += zone12.Area * zone12.Confidence
		}
		if zone13.Valid {
			totalWeight += zone13.Area * zone13.Confidence
		}
		if zone23.Valid {
			totalWeight += zone23.Area * zone23.Confidence
		}

		if totalWeight > 0 {
			centroidX /= totalWeight
			centroidY /= totalWeight
			lat, lon := ls.xyToLatLon(centroidX, centroidY, refLat, refLon)
			return lat, lon, nil
		}
	}

	// Fallback: weighted center based on signal strength
	weight1 := math.Pow(10, float64(locations[0].RSSI)/20)
	weight2 := math.Pow(10, float64(locations[1].RSSI)/20)
	weight3 := math.Pow(10, float64(locations[2].RSSI)/20)
	totalWeight := weight1 + weight2 + weight3

	if totalWeight > 0 {
		x := (p1X*weight1 + p2X*weight2 + p3X*weight3) / totalWeight
		y := (p1Y*weight1 + p2Y*weight2 + p3Y*weight3) / totalWeight
		lat, lon := ls.xyToLatLon(x, y, refLat, refLon)
		return lat, lon, nil
	}

	// Final fallback: geometric center
	x := (p1X + p2X + p3X) / 3
	y := (p1Y + p2Y + p3Y) / 3
	lat, lon := ls.xyToLatLon(x, y, refLat, refLon)
	return lat, lon, nil
}

// calculateLocationMultipleGateways calculates location using intersection zones of multiple circles
func (ls *LocationService) calculateLocationMultipleGateways(locations []models.LocationPoint, frequency float64) (float64, float64, error) {
	refLat, refLon := locations[0].Latitude, locations[0].Longitude
	n := len(locations)

	// Convert all gateway positions to XY coordinates
	positions := make([][2]float64, n)
	for i, loc := range locations {
		positions[i][0], positions[i][1] = ls.latLonToXY(loc.Latitude, loc.Longitude, refLat, refLon)
	}

	// Get expanded distances to ensure intersections
	distances := ls.expandRadiiUntilIntersection(locations, frequency, refLat, refLon)

	// Find all pairwise intersections and calculate confidence-weighted centroid
	var totalWeightedX, totalWeightedY, totalWeight float64

	for i := 0; i < n-1; i++ {
		for j := i + 1; j < n; j++ {
			zone := ls.calculateCircleIntersection(
				positions[i][0], positions[i][1], distances[i],
				positions[j][0], positions[j][1], distances[j],
			)

			if zone.Valid && zone.Area > 0 {
				// Weight by both area and confidence for better accuracy
				weight := zone.Area * zone.Confidence
				totalWeightedX += zone.CentroidX * weight
				totalWeightedY += zone.CentroidY * weight
				totalWeight += weight
			}
		}
	}

	if totalWeight > 0 {
		centroidX := totalWeightedX / totalWeight
		centroidY := totalWeightedY / totalWeight
		lat, lon := ls.xyToLatLon(centroidX, centroidY, refLat, refLon)
		return lat, lon, nil
	}

	// Fallback: weighted center based on signal strength
	var weightedX, weightedY, signalWeight float64
	for i, loc := range locations {
		weight := math.Pow(10, float64(loc.RSSI)/20) // Convert dBm to linear scale
		weightedX += positions[i][0] * weight
		weightedY += positions[i][1] * weight
		signalWeight += weight
	}

	if signalWeight > 0 {
		x := weightedX / signalWeight
		y := weightedY / signalWeight
		lat, lon := ls.xyToLatLon(x, y, refLat, refLon)
		return lat, lon, nil
	}

	// Final fallback: geometric center
	var sumX, sumY float64
	for i := range positions {
		sumX += positions[i][0]
		sumY += positions[i][1]
	}
	x := sumX / float64(n)
	y := sumY / float64(n)
	lat, lon := ls.xyToLatLon(x, y, refLat, refLon)
	return lat, lon, nil
}

// roundToDecimals rounds a float64 to specified decimal places
func roundToDecimals(val float64, decimals int) float64 {
	multiplier := math.Pow(10, float64(decimals))
	return math.Round(val*multiplier) / multiplier
}

// CircleIntersectionZone represents the intersection area between circles
type CircleIntersectionZone struct {
	CentroidX          float64
	CentroidY          float64
	Area               float64
	Valid              bool
	Confidence         float64      // 0-1 score based on intersection quality
	IntersectionPoints [][2]float64 // Actual intersection points for better centroid calculation
}

// calculateCircleIntersection checks if two circles intersect and returns intersection zone
func (ls *LocationService) calculateCircleIntersection(x1, y1, r1, x2, y2, r2 float64) CircleIntersectionZone {
	// Distance between circle centers
	d := math.Sqrt(math.Pow(x2-x1, 2) + math.Pow(y2-y1, 2))

	// Check if circles intersect
	if d > r1+r2 { // Too far apart
		return CircleIntersectionZone{Valid: false, Confidence: 0.0}
	}
	if d < math.Abs(r1-r2) { // One circle inside the other
		return CircleIntersectionZone{Valid: false, Confidence: 0.0}
	}
	if d == 0 && r1 == r2 { // Same circle
		return CircleIntersectionZone{
			CentroidX:  x1,
			CentroidY:  y1,
			Area:       math.Pi * r1 * r1,
			Valid:      true,
			Confidence: 1.0,
		}
	}

	// Calculate actual intersection points for better accuracy
	intersectionPoints := ls.calculateIntersectionPoints(x1, y1, r1, x2, y2, r2, d)

	// Calculate intersection area using the lens formula
	part1 := r1 * r1 * math.Acos((d*d+r1*r1-r2*r2)/(2*d*r1))
	part2 := r2 * r2 * math.Acos((d*d+r2*r2-r1*r1)/(2*d*r2))
	part3 := 0.5 * math.Sqrt((-d+r1+r2)*(d+r1-r2)*(d-r1+r2)*(d+r1+r2))
	area := part1 + part2 - part3

	// Calculate improved centroid using geometric properties
	centroidX, centroidY := ls.calculateImprovedCentroid(x1, y1, r1, x2, y2, r2, d, intersectionPoints)

	// Calculate confidence based on intersection quality
	confidence := ls.calculateIntersectionConfidence(d, r1, r2, area)

	return CircleIntersectionZone{
		CentroidX:          centroidX,
		CentroidY:          centroidY,
		Area:               area,
		Valid:              true,
		Confidence:         confidence,
		IntersectionPoints: intersectionPoints,
	}
}

// expandRadiiUntilIntersection smartly expands radii for optimal intersections
func (ls *LocationService) expandRadiiUntilIntersection(locations []models.LocationPoint, frequency float64, refLat, refLon float64) []float64 {
	distances := make([]float64, len(locations))
	for i, loc := range locations {
		distances[i] = ls.findDistance(loc.RSSI, frequency)
	}

	// Smart expansion: calculate minimum expansion needed for each pair
	minExpansionNeeded := 1.0

	for i := 0; i < len(locations)-1; i++ {
		for j := i + 1; j < len(locations); j++ {
			x1, y1 := ls.latLonToXY(locations[i].Latitude, locations[i].Longitude, refLat, refLon)
			x2, y2 := ls.latLonToXY(locations[j].Latitude, locations[j].Longitude, refLat, refLon)
			d := math.Sqrt(math.Pow(x2-x1, 2) + math.Pow(y2-y1, 2))

			// Calculate minimum expansion factor needed for intersection
			currentSum := distances[i] + distances[j]
			if d > currentSum {
				requiredExpansion := (d / currentSum) * 1.1 // Add 10% buffer for good overlap
				if requiredExpansion > minExpansionNeeded {
					minExpansionNeeded = requiredExpansion
				}
			}
		}
	}

	// Cap expansion to reasonable limits
	maxExpansion := 3.0
	if minExpansionNeeded > maxExpansion {
		minExpansionNeeded = maxExpansion
	}

	// Apply intelligent expansion: stronger signals get less expansion
	for i := range distances {
		// Weaker signals (lower RSSI) get more expansion as they're less reliable
		signalStrength := math.Abs(float64(locations[i].RSSI))
		adaptiveFactor := minExpansionNeeded

		// Stronger signals (RSSI > -70) get less expansion
		if signalStrength < 70 {
			adaptiveFactor *= 0.8
		} else if signalStrength > 100 {
			adaptiveFactor *= 1.2 // Weaker signals get more expansion
		}

		distances[i] *= adaptiveFactor
	}

	return distances
}

// calculateIntersectionPoints finds the exact intersection points of two circles
func (ls *LocationService) calculateIntersectionPoints(x1, y1, r1, x2, y2, r2, d float64) [][2]float64 {
	// Calculate intersection points using geometry
	a := (r1*r1 - r2*r2 + d*d) / (2 * d)
	h := math.Sqrt(r1*r1 - a*a)

	// Point on line between centers
	px := x1 + a*(x2-x1)/d
	py := y1 + a*(y2-y1)/d

	// Two intersection points
	point1 := [2]float64{
		px + h*(y2-y1)/d,
		py - h*(x2-x1)/d,
	}
	point2 := [2]float64{
		px - h*(y2-y1)/d,
		py + h*(x2-x1)/d,
	}

	return [][2]float64{point1, point2}
}

// calculateImprovedCentroid calculates a more accurate centroid using intersection geometry
func (ls *LocationService) calculateImprovedCentroid(x1, y1, r1, x2, y2, r2, d float64, intersectionPoints [][2]float64) (float64, float64) {
	if len(intersectionPoints) != 2 {
		// Fallback to previous method
		a := (r1*r1 - r2*r2 + d*d) / (2 * d)
		px := x1 + a*(x2-x1)/d
		py := y1 + a*(y2-y1)/d
		return px, py
	}

	// For lens-shaped intersection, the centroid is weighted towards the center of the lens
	// Use the midpoint of intersection points as a better approximation
	p1, p2 := intersectionPoints[0], intersectionPoints[1]

	// Bounds check for individual points
	if len(p1) < 2 || len(p2) < 2 {
		// Fallback to center line calculation
		a := (r1*r1 - r2*r2 + d*d) / (2 * d)
		px := x1 + a*(x2-x1)/d
		py := y1 + a*(y2-y1)/d
		return px, py
	}

	midX := (p1[0] + p2[0]) / 2
	midY := (p1[1] + p2[1]) / 2

	// Weight towards the line connecting circle centers for better accuracy
	a := (r1*r1 - r2*r2 + d*d) / (2 * d)
	centerLineX := x1 + a*(x2-x1)/d
	centerLineY := y1 + a*(y2-y1)/d

	// Weighted average: 70% intersection midpoint, 30% center line point
	centroidX := 0.7*midX + 0.3*centerLineX
	centroidY := 0.7*midY + 0.3*centerLineY

	return centroidX, centroidY
}

// calculateIntersectionConfidence calculates confidence score based on intersection quality
func (ls *LocationService) calculateIntersectionConfidence(d, r1, r2, area float64) float64 {
	// Factors affecting confidence:
	// 1. Overlap ratio - larger overlap = higher confidence
	// 2. Circle size similarity - similar sizes = higher confidence
	// 3. Intersection geometry - avoid tangential intersections

	maxArea := math.Min(math.Pi*r1*r1, math.Pi*r2*r2)
	overlapRatio := area / maxArea

	// Size similarity factor (1.0 when radii are equal, lower when very different)
	sizeFactor := 1.0 - math.Abs(r1-r2)/math.Max(r1, r2)

	// Distance factor - penalize near-tangential intersections
	optimalDistance := (r1 + r2) * 0.7 // Optimal overlap
	distanceFactor := 1.0 - math.Abs(d-optimalDistance)/(r1+r2)
	if distanceFactor < 0 {
		distanceFactor = 0
	}

	// Combined confidence (weighted average)
	confidence := 0.5*overlapRatio + 0.3*sizeFactor + 0.2*distanceFactor

	// Clamp between 0 and 1
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}

	return confidence
}

// findDistanceImproved calculates distance with environmental factors consideration
// func (ls *LocationService) findDistanceImproved(rssi int, frequency float64, environment string) float64 {
// 	freqMHz := frequency / 1_000_000
// 	plD0 := 32.45 + 20*math.Log10(freqMHz)

// 	// Adjust path loss exponent based on environment
// 	pathLossExponent := DefaultPathLossExponent
// 	switch environment {
// 	case "urban":
// 		pathLossExponent = 4.5 // Higher path loss in urban areas
// 	case "suburban":
// 		pathLossExponent = 3.5
// 	case "rural":
// 		pathLossExponent = 2.5 // Lower path loss in open areas
// 	default:
// 		pathLossExponent = DefaultPathLossExponent
// 	}

// 	pl := DefaultTxPower - float64(rssi)
// 	logD := (pl - plD0) / (10 * pathLossExponent)
// 	distance := DefaultReferenceDistance * math.Pow(10, logD)

// 	// Add uncertainty bounds - return base distance for now
// 	return distance
// }
