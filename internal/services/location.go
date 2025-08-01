package services

import (
	"fmt"
	"math"

	"github.com/Space-DF/transformer-service-go/internal/models"
	"gonum.org/v1/gonum/mat"
)

const (
	EarthRadius = 6371000.0 // Earth radius in meters
	DefaultTxPower = 14.0   // Default transmission power in dBm
	DefaultPathLossExponent = 4.0 // Default path loss exponent
	DefaultReferenceDistance = 1.0 // Default reference distance in meters
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
		// ChirpStack format - check for rxInfo array
		var rxOk bool
		rxMetadata, rxOk = payload["rxInfo"].([]interface{})
		if !rxOk {
			return nil, fmt.Errorf("rxInfo not found in payload")
		}

		// Extract frequency from txInfo
		txInfo, txOk := payload["txInfo"].(map[string]interface{})
		if !txOk {
			return nil, fmt.Errorf("txInfo not found in payload")
		}

		var freqOk bool
		frequency, freqOk = txInfo["frequency"].(float64)
		if !freqOk {
			return nil, fmt.Errorf("frequency not found in txInfo")
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
	freqMHz := frequency / 1e6
	plD0 := 32.45 + 20*math.Log10(freqMHz)
	pl := DefaultTxPower - float64(rssi)
	logD := (pl - plD0) / (10 * DefaultPathLossExponent)
	return DefaultReferenceDistance * math.Pow(10, logD)
}

// latLonToXY converts lat/lon to local XY coordinates
func (ls *LocationService) latLonToXY(lat, lon, refLat, refLon float64) (float64, float64) {
	x := EarthRadius * math.Pi/180 * (lon - refLon) * math.Cos(math.Pi/180*refLat)
	y := EarthRadius * math.Pi/180 * (lat - refLat)
	return x, y
}

// xyToLatLon converts local XY coordinates to lat/lon
func (ls *LocationService) xyToLatLon(x, y, refLat, refLon float64) (float64, float64) {
	lat := refLat + (180/math.Pi)*(y/EarthRadius)
	lon := refLon + (180/math.Pi)*(x/(EarthRadius*math.Cos(math.Pi/180*refLat)))
	return lat, lon
}

// calculateLocationTwoGateways calculates location using two gateways
func (ls *LocationService) calculateLocationTwoGateways(locations []models.LocationPoint, frequency float64) (float64, float64, error) {
	refLat, refLon := locations[0].Latitude, locations[0].Longitude

	p1X, p1Y := ls.latLonToXY(locations[0].Latitude, locations[0].Longitude, refLat, refLon)
	p2X, p2Y := ls.latLonToXY(locations[1].Latitude, locations[1].Longitude, refLat, refLon)

	d1 := ls.findDistance(locations[0].RSSI, frequency)
	d2 := ls.findDistance(locations[1].RSSI, frequency)

	D := math.Sqrt(math.Pow(p2X-p1X, 2) + math.Pow(p2Y-p1Y, 2))
	a := (math.Pow(d1, 2) - math.Pow(d2, 2) + math.Pow(D, 2)) / (2 * D)
	hSq := math.Pow(d1, 2) - math.Pow(a, 2)

	var h float64
	if math.Abs(hSq) < 1e-10 {
		h = 0.0
	} else {
		h = math.Sqrt(math.Abs(hSq))
	}

	x2 := p1X + a*(p2X-p1X)/D
	y2 := p1Y + a*(p2Y-p1Y)/D

	var x, y float64
	if h == 0.0 {
		x, y = x2, y2
	} else {
		intersec1X := x2 + h*(p2Y-p1Y)/D
		intersec1Y := y2 - h*(p2X-p1X)/D

		intersec2X := x2 - h*(p2Y-p1Y)/D
		intersec2Y := y2 + h*(p2X-p1X)/D

		// Choose intersection point based on RSSI signal strength
		rssiDiff := math.Abs(float64(locations[0].RSSI - locations[1].RSSI))
		
		if rssiDiff > 10.0 { // Significant signal difference (>10 dB)
			// Choose point closer to the gateway with stronger signal
			strongerGateway := 0
			if locations[1].RSSI > locations[0].RSSI {
				strongerGateway = 1
			}
			
			// Calculate distances from each intersection to each gateway
			dist1ToGw0 := math.Sqrt(math.Pow(intersec1X-p1X, 2) + math.Pow(intersec1Y-p1Y, 2))
			dist1ToGw1 := math.Sqrt(math.Pow(intersec1X-p2X, 2) + math.Pow(intersec1Y-p2Y, 2))
			dist2ToGw0 := math.Sqrt(math.Pow(intersec2X-p1X, 2) + math.Pow(intersec2Y-p1Y, 2))
			dist2ToGw1 := math.Sqrt(math.Pow(intersec2X-p2X, 2) + math.Pow(intersec2Y-p2Y, 2))
			
			if strongerGateway == 0 {
				// Choose intersection closer to gateway 0
				if dist1ToGw0 < dist2ToGw0 {
					x, y = intersec1X, intersec1Y
				} else {
					x, y = intersec2X, intersec2Y
				}
			} else {
				// Choose intersection closer to gateway 1
				if dist1ToGw1 < dist2ToGw1 {
					x, y = intersec1X, intersec1Y
				} else {
					x, y = intersec2X, intersec2Y
				}
			}
		} else if rssiDiff > 3.0 { // Moderate signal difference (3-10 dB)
			// Use weighted positioning based on signal strength
			weight1 := float64(locations[0].RSSI + 120) // Normalize RSSI to positive values
			weight2 := float64(locations[1].RSSI + 120)
			totalWeight := weight1 + weight2
			
			if totalWeight > 0 {
				x = (intersec1X*weight1 + intersec2X*weight2) / totalWeight
				y = (intersec1Y*weight1 + intersec2Y*weight2) / totalWeight
			} else {
				// Fallback to averaging
				x = (intersec1X + intersec2X) / 2
				y = (intersec1Y + intersec2Y) / 2
			}
		} else {
			// Similar signal strengths - use traditional averaging
			x = (intersec1X + intersec2X) / 2
			y = (intersec1Y + intersec2Y) / 2
		}
	}

	lat, lon := ls.xyToLatLon(x, y, refLat, refLon)
	return lat, lon, nil
}

// calculateLocationThreeGateways calculates location using three gateways
func (ls *LocationService) calculateLocationThreeGateways(locations []models.LocationPoint, frequency float64) (float64, float64, error) {
	refLat, refLon := locations[0].Latitude, locations[0].Longitude

	p1X, p1Y := ls.latLonToXY(locations[0].Latitude, locations[0].Longitude, refLat, refLon)
	p2X, p2Y := ls.latLonToXY(locations[1].Latitude, locations[1].Longitude, refLat, refLon)
	p3X, p3Y := ls.latLonToXY(locations[2].Latitude, locations[2].Longitude, refLat, refLon)

	d1 := ls.findDistance(locations[0].RSSI, frequency)
	d2 := ls.findDistance(locations[1].RSSI, frequency)
	d3 := ls.findDistance(locations[2].RSSI, frequency)

	// Set up matrix equation Ax = b
	A := mat.NewDense(2, 2, []float64{
		2*(p2X-p1X), 2*(p2Y-p1Y),
		2*(p3X-p1X), 2*(p3Y-p1Y),
	})

	b := mat.NewVecDense(2, []float64{
		math.Pow(d1, 2) - math.Pow(d2, 2) - math.Pow(p1X, 2) + math.Pow(p2X, 2) - math.Pow(p1Y, 2) + math.Pow(p2Y, 2),
		math.Pow(d1, 2) - math.Pow(d3, 2) - math.Pow(p1X, 2) + math.Pow(p3X, 2) - math.Pow(p1Y, 2) + math.Pow(p3Y, 2),
	})

	var x mat.VecDense
	err := x.SolveVec(A, b)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to solve linear system: %v", err)
	}

	lat, lon := ls.xyToLatLon(x.AtVec(0), x.AtVec(1), refLat, refLon)
	return lat, lon, nil
}

// calculateLocationMultipleGateways calculates location using multiple gateways (least squares)
func (ls *LocationService) calculateLocationMultipleGateways(locations []models.LocationPoint, frequency float64) (float64, float64, error) {
	refLat, refLon := locations[0].Latitude, locations[0].Longitude

	n := len(locations)
	A := mat.NewDense(n-1, 2, nil)
	b := mat.NewVecDense(n-1, nil)

	p1X, p1Y := ls.latLonToXY(locations[0].Latitude, locations[0].Longitude, refLat, refLon)
	d1 := ls.findDistance(locations[0].RSSI, frequency)

	for i := 1; i < n; i++ {
		piX, piY := ls.latLonToXY(locations[i].Latitude, locations[i].Longitude, refLat, refLon)
		di := ls.findDistance(locations[i].RSSI, frequency)

		A.Set(i-1, 0, 2*(piX-p1X))
		A.Set(i-1, 1, 2*(piY-p1Y))

		bVal := math.Pow(d1, 2) - math.Pow(di, 2) - math.Pow(p1X, 2) + math.Pow(piX, 2) - math.Pow(p1Y, 2) + math.Pow(piY, 2)
		b.SetVec(i-1, bVal)
	}

	var x mat.VecDense
	err := x.SolveVec(A, b)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to solve least squares: %v", err)
	}

	lat, lon := ls.xyToLatLon(x.AtVec(0), x.AtVec(1), refLat, refLon)
	return lat, lon, nil
}

// roundToDecimals rounds a float64 to specified decimal places
func roundToDecimals(val float64, decimals int) float64 {
	multiplier := math.Pow(10, float64(decimals))
	return math.Round(val*multiplier) / multiplier
}