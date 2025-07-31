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
	// Try to extract uplink message from different possible locations
	var uplinkMessage map[string]interface{}
	
	// First, try to get uplink_message from payload
	if msg, ok := payload["uplink_message"].(map[string]interface{}); ok {
		uplinkMessage = msg
	} else if payloadData, ok := payload["payload"].(map[string]interface{}); ok {
		// Try to get uplink_message from nested payload
		if msg, ok := payloadData["uplink_message"].(map[string]interface{}); ok {
			uplinkMessage = msg
		} else {
			// Use the payload data directly
			uplinkMessage = payloadData
		}
	} else {
		// Use the root payload directly
		uplinkMessage = payload
	}

	// Extract gateways - try multiple possible locations
	var rxMetadata []interface{}
	var ok bool
	
	// Try rx_metadata in uplink message
	if rxMetadata, ok = uplinkMessage["rx_metadata"].([]interface{}); !ok {
		// Try gateways field
		if rxMetadata, ok = uplinkMessage["gateways"].([]interface{}); !ok {
			// Try gateway_info or similar
			if rxMetadata, ok = uplinkMessage["gateway_info"].([]interface{}); !ok {
				// Try rxInfo field (LoRaWAN format)
				if rxMetadata, ok = uplinkMessage["rxInfo"].([]interface{}); !ok {
					return nil, fmt.Errorf("rx_metadata, gateways, gateway_info, or rxInfo not found in message. Available keys: %v payload.raw_data is encoded by base64", getMapKeys(uplinkMessage))
				}
			}
		}
	}

	// Extract frequency - try multiple possible locations
	var frequency float64
	var freqOk bool
	
	if settings, ok := uplinkMessage["settings"].(map[string]interface{}); ok {
		if freq, ok := settings["frequency"].(float64); ok {
			frequency = freq
			freqOk = true
		}
	}
	
	// Try direct frequency field if not found in settings
	if !freqOk {
		if freq, ok := uplinkMessage["frequency"].(float64); ok {
			frequency = freq
			freqOk = true
		}
	}
	
	// Try txInfo for LoRaWAN format
	if !freqOk {
		if txInfo, ok := uplinkMessage["txInfo"].(map[string]interface{}); ok {
			if freq, ok := txInfo["frequency"].(float64); ok {
				frequency = freq
				freqOk = true
			}
		}
	}
	
	// Use default frequency if not found
	if !freqOk {
		frequency = 868000000.0 // Default LoRaWAN frequency (868 MHz)
	}

	// Extract device EUI - try both payload and uplinkMessage
	var devEUI string
	if endDeviceIDs, ok := payload["end_device_ids"].(map[string]interface{}); ok {
		if eui, ok := endDeviceIDs["dev_eui"].(string); ok {
			devEUI = eui
		}
	}
	
	// If not found in payload, try uplinkMessage or check for dev_eui directly
	if devEUI == "" {
		if eui, ok := uplinkMessage["dev_eui"].(string); ok {
			devEUI = eui
		} else if eui, ok := payload["dev_eui"].(string); ok {
			devEUI = eui
		} else if eui, ok := uplinkMessage["devEui"].(string); ok {
			// LoRaWAN format uses devEui
			devEUI = eui
		} else if deviceInfo, ok := uplinkMessage["deviceInfo"].(map[string]interface{}); ok {
			// Try deviceInfo.devEui
			if eui, ok := deviceInfo["devEui"].(string); ok {
				devEUI = eui
			}
		}
		
		if devEUI == "" {
			return nil, fmt.Errorf("dev_eui/devEui not found in payload")
		}
	}

	// Parse gateway locations
	var locations []models.LocationPoint
	for _, gw := range rxMetadata {
		gateway, ok := gw.(map[string]interface{})
		if !ok {
			continue
		}

		var lat, lon, rssi float64
		var latOk, lonOk, rssiOk bool

		// Try standard format first
		if locationData, ok := gateway["location"].(map[string]interface{}); ok {
			lat, latOk = locationData["latitude"].(float64)
			lon, lonOk = locationData["longitude"].(float64)
			rssi, rssiOk = gateway["rssi"].(float64)
		} else {
			// Try LoRaWAN format - location might be directly in gateway object
			lat, latOk = gateway["latitude"].(float64)
			lon, lonOk = gateway["longitude"].(float64)
			rssi, rssiOk = gateway["rssi"].(float64)
			
			// If not found, try alternative field names
			if !latOk || !lonOk || !rssiOk {
				// Check for altitude field that might contain lat/lon
				if alt, ok := gateway["altitude"].(float64); ok && !latOk {
					lat = alt
					latOk = true
				}
				
				// Try different RSSI field names
				if !rssiOk {
					if rssiVal, ok := gateway["loRaSNR"].(float64); ok {
						rssi = rssiVal
						rssiOk = true
					}
				}
			}
		}

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
		Latitude:     roundToDecimals(lat, 6),
		Longitude:    roundToDecimals(lon, 6),
		DevEUI:       devEUI,
		Organization: "unknown", // This will be set by the consumer using device profile
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

		x = (intersec1X + intersec2X) / 2
		y = (intersec1Y + intersec2Y) / 2
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

// getMapKeys returns the keys of a map for debugging
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}