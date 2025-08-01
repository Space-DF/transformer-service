package parsers

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/Space-DF/transformer-service-go/internal/models"
	"github.com/fxamacker/cbor/v2"
)

// RAK4630Parser handles parsing of RAK4630 device payloads with GPS
type RAK4630Parser struct{}

// SensorFrame represents the CBOR sensor frame structure from firmware
type SensorFrame struct {
	ID     int    `cbor:"id"`
	Type   int    `cbor:"type"`  
	Fmt    string `cbor:"fmt"`
	Sensor string `cbor:"sensor"`
}

// NewRAK4630Parser creates a new RAK4630 parser
func NewRAK4630Parser() *RAK4630Parser {
	return &RAK4630Parser{}
}

// ParsePayload parses RAK4630 device payload and extracts GPS coordinates or sensor data
func (p *RAK4630Parser) ParsePayload(payload map[string]interface{}) (*models.DeviceLocationData, error) {
	// Extract device EUI first
	devEUI := p.extractDevEUI(payload)
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI not found")
	}

	// Try to parse CBOR data from the "data" field first
	if location, err := p.parseFromCBORData(payload); err == nil {
		location.DevEUI = devEUI
		return location, nil
	}

	// Try to extract GPS coordinates from frm_payload
	if location, err := p.parseFromFrmPayload(payload); err == nil {
		location.DevEUI = devEUI
		return location, nil
	}

	// Try to extract from decoded payload if available
	if location, err := p.parseFromDecodedPayload(payload); err == nil {
		location.DevEUI = devEUI
		return location, nil
	}

	return nil, fmt.Errorf("no GPS coordinates or sensor data found in RAK4630 payload")
}

// SupportsGPS returns true since RAK4630 has built-in GPS
func (p *RAK4630Parser) SupportsGPS() bool {
	return true
}

// parseFromCBORData parses CBOR-encoded sensor data from the "data" field
func (p *RAK4630Parser) parseFromCBORData(payload map[string]interface{}) (*models.DeviceLocationData, error) {
	// Look for data field in decoded_raw_data first
	var dataStr string
	var ok bool

	if decodedData, exists := payload["decoded_raw_data"].(map[string]interface{}); exists {
		if dataStr, ok = decodedData["data"].(string); !ok {
			return nil, fmt.Errorf("data field not found in decoded_raw_data")
		}
	} else {
		// Try direct access to data field
		if dataStr, ok = payload["data"].(string); !ok {
			return nil, fmt.Errorf("data field not found in payload")
		}
	}

	// Decode base64 data
	cborData, err := base64.StdEncoding.DecodeString(dataStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 data: %w", err)
	}

	// Parse CBOR data
	var frame SensorFrame
	if err := cbor.Unmarshal(cborData, &frame); err != nil {
		return nil, fmt.Errorf("failed to parse CBOR data: %w", err)
	}

	// Extract sensor data and look for GPS coordinates
	location, err := p.parseSensorData(frame)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sensor data: %w", err)
	}

	return location, nil
}

// parseSensorData extracts GPS coordinates from sensor frame data
func (p *RAK4630Parser) parseSensorData(frame SensorFrame) (*models.DeviceLocationData, error) {
	// Parse sensor string which contains comma-separated values
	// Format example: "xs29.51,41.48,99.96,50,30.20,1.75,0.04,-9.91,-0.06,-0.02,0.03,79.50,185.10,39.90,0.00,0.00,0.00,0,0,00,0,100,3.939692"
	
	sensorData := frame.Sensor
	if sensorData == "" {
		return nil, fmt.Errorf("sensor data is empty")
	}

	// Remove prefix if present (e.g., "xs")
	if strings.HasPrefix(sensorData, "xs") {
		sensorData = sensorData[2:]
	}

	// Split by comma
	values := strings.Split(sensorData, ",")
	if len(values) < 2 {
		return nil, fmt.Errorf("insufficient sensor data values: %d", len(values))
	}

	// Check if first two values could be coordinates
	// Note: This is a heuristic - you may need to adjust based on your actual data format
	lat, err1 := strconv.ParseFloat(strings.TrimSpace(values[0]), 64)
	lon, err2 := strconv.ParseFloat(strings.TrimSpace(values[1]), 64)

	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("failed to parse coordinates from sensor data: lat=%s, lon=%s", values[0], values[1])
	}

	// Validate if these look like GPS coordinates
	if err := p.validateCoordinates(lat, lon); err != nil {
		// If not valid GPS coordinates, this might be sensor data without GPS
		return nil, fmt.Errorf("sensor data does not contain valid GPS coordinates: %w", err)
	}

	return &models.DeviceLocationData{
		Latitude:  lat,
		Longitude: lon,
	}, nil
}

// parseFromFrmPayload extracts GPS coordinates from hex frm_payload
func (p *RAK4630Parser) parseFromFrmPayload(payload map[string]interface{}) (*models.DeviceLocationData, error) {
	var frmPayload string
	var ok bool
	
	// Look for frm_payload in uplink_message
	if uplinkMsg, exists := payload["uplink_message"].(map[string]interface{}); exists {
		if frmPayload, ok = uplinkMsg["frm_payload"].(string); !ok {
			return nil, fmt.Errorf("frm_payload not found in uplink_message")
		}
	} else {
		// Try direct access
		if frmPayload, ok = payload["frm_payload"].(string); !ok {
			return nil, fmt.Errorf("frm_payload not found in payload")
		}
	}

	// Decode hex payload
	payloadBytes, err := hex.DecodeString(frmPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode hex payload: %w", err)
	}

	if len(payloadBytes) < 8 {
		return nil, fmt.Errorf("payload too short for GPS data: %d bytes", len(payloadBytes))
	}

	// Parse GPS coordinates from RAK4630 payload format
	latitude, longitude, err := p.parseGPSCoordinates(payloadBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GPS coordinates: %w", err)
	}

	return &models.DeviceLocationData{
		Latitude:  latitude,
		Longitude: longitude,
	}, nil
}

// parseFromDecodedPayload extracts GPS coordinates from already decoded payload
func (p *RAK4630Parser) parseFromDecodedPayload(payload map[string]interface{}) (*models.DeviceLocationData, error) {
	// Check for decoded_payload or decoded_raw_data
	var decodedData map[string]interface{}
	var ok bool

	if decodedData, ok = payload["decoded_payload"].(map[string]interface{}); !ok {
		if decodedData, ok = payload["decoded_raw_data"].(map[string]interface{}); !ok {
			return nil, fmt.Errorf("no decoded payload data found")
		}
	}

	// Look for GPS coordinates in various formats
	var latitude, longitude float64
	var found bool

	// Try direct lat/lon fields
	if lat, latOk := decodedData["latitude"].(float64); latOk {
		if lon, lonOk := decodedData["longitude"].(float64); lonOk {
			latitude, longitude = lat, lon
			found = true
		}
	}

	// Try lat/lng fields
	if !found {
		if lat, latOk := decodedData["lat"].(float64); latOk {
			if lon, lonOk := decodedData["lng"].(float64); lonOk {
				latitude, longitude = lat, lon
				found = true
			}
		}
	}

	// Try GPS object
	if !found {
		if gps, gpsOk := decodedData["gps"].(map[string]interface{}); gpsOk {
			if lat, latOk := gps["latitude"].(float64); latOk {
				if lon, lonOk := gps["longitude"].(float64); lonOk {
					latitude, longitude = lat, lon
					found = true
				}
			}
		}
	}

	if !found {
		return nil, fmt.Errorf("GPS coordinates not found in decoded payload")
	}

	// Validate coordinates
	if err := p.validateCoordinates(latitude, longitude); err != nil {
		return nil, err
	}

	return &models.DeviceLocationData{
		Latitude:  latitude,
		Longitude: longitude,
	}, nil
}

// parseGPSCoordinates extracts GPS coordinates from RAK4630 payload bytes
func (p *RAK4630Parser) parseGPSCoordinates(payloadBytes []byte) (float64, float64, error) {
	if len(payloadBytes) < 8 {
		return 0, 0, fmt.Errorf("insufficient data for GPS coordinates")
	}

	// RAK4630 GPS format (may need adjustment based on actual format):
	// Bytes 0-3: Latitude (32-bit signed integer, divide by 10000000 for decimal degrees)
	// Bytes 4-7: Longitude (32-bit signed integer, divide by 10000000 for decimal degrees)
	
	latInt := int32(payloadBytes[0]) | int32(payloadBytes[1])<<8 | int32(payloadBytes[2])<<16 | int32(payloadBytes[3])<<24
	lonInt := int32(payloadBytes[4]) | int32(payloadBytes[5])<<8 | int32(payloadBytes[6])<<16 | int32(payloadBytes[7])<<24

	latitude := float64(latInt) / 10000000.0
	longitude := float64(lonInt) / 10000000.0

	if err := p.validateCoordinates(latitude, longitude); err != nil {
		return 0, 0, err
	}

	return latitude, longitude, nil
}

// validateCoordinates validates GPS coordinates
func (p *RAK4630Parser) validateCoordinates(latitude, longitude float64) error {
	if latitude < -90 || latitude > 90 {
		return fmt.Errorf("invalid latitude: %f", latitude)
	}
	if longitude < -180 || longitude > 180 {
		return fmt.Errorf("invalid longitude: %f", longitude)
	}
	return nil
}

// extractDevEUI extracts device EUI from various possible locations in payload
func (p *RAK4630Parser) extractDevEUI(payload map[string]interface{}) string {
	// Try multiple locations for device EUI
	if endDeviceIDs, ok := payload["end_device_ids"].(map[string]interface{}); ok {
		if devEUI, ok := endDeviceIDs["dev_eui"].(string); ok {
			return devEUI
		}
	}
	
	if devEUI, ok := payload["dev_eui"].(string); ok {
		return devEUI
	}
	
	if devEUI, ok := payload["devEui"].(string); ok {
		return devEUI
	}
	
	if deviceInfo, ok := payload["deviceInfo"].(map[string]interface{}); ok {
		if devEUI, ok := deviceInfo["devEui"].(string); ok {
			return devEUI
		}
	}
	
	return ""
}

// ParseTextCoordinates parses coordinates from text format (utility method)
func (p *RAK4630Parser) ParseTextCoordinates(text string) (float64, float64, error) {
	text = strings.TrimSpace(text)
	
	if strings.Contains(text, ",") {
		parts := strings.Split(text, ",")
		if len(parts) != 2 {
			return 0, 0, fmt.Errorf("invalid coordinate format: %s", text)
		}
		
		latStr := strings.TrimSpace(parts[0])
		lonStr := strings.TrimSpace(parts[1])
		
		latStr = strings.TrimPrefix(latStr, "lat:")
		lonStr = strings.TrimPrefix(lonStr, "lon:")
		
		lat, err := strconv.ParseFloat(latStr, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid latitude: %s", latStr)
		}
		
		lon, err := strconv.ParseFloat(lonStr, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid longitude: %s", lonStr)
		}
		
		if err := p.validateCoordinates(lat, lon); err != nil {
			return 0, 0, err
		}
		
		return lat, lon, nil
	}
	
	return 0, 0, fmt.Errorf("unsupported coordinate text format: %s", text)
}