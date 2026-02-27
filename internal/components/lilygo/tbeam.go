package lilygo

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/Space-DF/transformer-service/internal/components"
)

// Cayenne LPP Data Type constants
const (
	LPPDigitalInput     = 0x00
	LPPDigitalOutput    = 0x01
	LPPAnalogInput      = 0x02
	LPPAnalogOutput     = 0x03
	LPPIlluminance      = 0x65
	LPPPresence         = 0x66
	LPPTemperature      = 0x67
	LPPHumidity         = 0x68
	LPPAccelerometer    = 0x71
	LPPBarometer        = 0x73
	LPPGPSLocation      = 0x88 // GPS location: 9 bytes (lat 3, lon 3, alt 3)
	LPPUnixTime         = 0x95
)

func readInt24BE(data []byte) int32 {
	if len(data) < 3 {
		return 0
	}
	value := int32(data[0])<<16 | int32(data[1])<<8 | int32(data[2])
	// Sign extend 24-bit to 32-bit
	if value&0x800000 != 0 {
		value |= ^0xFFFFFF
	}
	return value
}

type TBeamParser struct{}

func NewTBeamParser() *TBeamParser {
	return &TBeamParser{}
}

// GetSupportedPorts returns the fPorts this device type uses
func (c *TBeamParser) GetSupportedPorts() []int {
	return []int{1, 2, 5}
}

func (c *TBeamParser) SupportsGPS() bool {
	return true
}

// GetSupportedEntityTypes returns the entity types this device supports
// T-Beam has built-in GPS and battery management.
// Temperature, humidity, pressure, etc. are only available via external sensors (e.g. BME280 on I2C)
// and are parsed dynamically from Cayenne LPP payloads when present.
func (c *TBeamParser) GetSupportedEntityTypes() []string {
	return []string{"location", "battery"}
}

func (c *TBeamParser) ParsePayload(payload *components.RawPayload) (*components.ParsedData, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = components.ExtractDevEUI(payload.Metadata)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI not found")
	}

	// Decode payload bytes
	encoded := extractPayloadData(payload.Data)
	if encoded == "" {
		encoded = extractPayloadData(payload.Metadata)
	}
	if encoded == "" {
		return nil, fmt.Errorf("no payload data found")
	}

	bytes, err := decodePayloadBytes(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	// Parse Cayenne LPP payload
	sensorData := make(map[string]interface{})
	var location *components.Location
	var batteryLevel *float64

	// Parse Cayenne LPP format
	// Format: [Channel(1)][Type(1)][Data...]
	i := 0
	for i < len(bytes) {
		if i+2 > len(bytes) {
			break
		}

		channel := bytes[i]
		dataType := bytes[i+1]
		i += 2

		switch dataType {
		case LPPTemperature:
			// 2 bytes, 0.1°C signed
			if i+2 > len(bytes) {
				break
			}
			rawTemp := int16(binary.BigEndian.Uint16(bytes[i : i+2])) //#nosec G115
			temp := float64(rawTemp) / 10.0
			sensorData[fmt.Sprintf("temperature_ch%d", channel)] = temp
			sensorData["temperature"] = temp
			i += 2

		case LPPHumidity:
			// 1 byte, 0.5% unsigned
			if i+1 > len(bytes) {
				break
			}
			humidity := float64(bytes[i]) / 2.0
			sensorData[fmt.Sprintf("humidity_ch%d", channel)] = humidity
			sensorData["humidity"] = humidity
			i += 1

		case LPPDigitalInput, LPPDigitalOutput:
			// 1 byte, digital value
			if i+1 > len(bytes) {
				break
			}
			value := bytes[i]
			sensorData[fmt.Sprintf("digital_ch%d", channel)] = value > 0
			i += 1

		case LPPAnalogInput, LPPAnalogOutput:
			// 2 bytes, 0.01 signed
			if i+2 > len(bytes) {
				break
			}
			rawAnalog := int16(binary.BigEndian.Uint16(bytes[i : i+2])) //#nosec G115
			analog := float64(rawAnalog) / 100.0
			sensorData[fmt.Sprintf("analog_ch%d", channel)] = analog
			i += 2

		case LPPIlluminance:
			// 2 bytes, lux unsigned
			if i+2 > len(bytes) {
				break
			}
			illuminance := float64(binary.BigEndian.Uint16(bytes[i : i+2]))
			sensorData[fmt.Sprintf("illuminance_ch%d", channel)] = illuminance
			sensorData["illuminance"] = illuminance
			i += 2

		case LPPPresence:
			// 1 byte presence
			if i+1 > len(bytes) {
				break
			}
			value := bytes[i] > 0
			sensorData[fmt.Sprintf("presence_ch%d", channel)] = value
			sensorData["presence"] = value
			i += 1

		case LPPBarometer:
			// 2 bytes, 0.1 hPa unsigned
			if i+2 > len(bytes) {
				break
			}
			rawPressure := binary.BigEndian.Uint16(bytes[i : i+2])
			pressure := float64(rawPressure) / 10.0
			sensorData[fmt.Sprintf("pressure_ch%d", channel)] = pressure
			sensorData["pressure"] = pressure
			i += 2

		case LPPGPSLocation:
			// 9 bytes: lat(3), lon(3), alt(3) - STANDARD CAYENNE LPP
			if i+9 > len(bytes) {
				break
			}
			// Use helper function to read 3-byte signed integers
			latRaw := readInt24BE(bytes[i : i+3])
			lonRaw := readInt24BE(bytes[i+3 : i+6])
			altRaw := readInt24BE(bytes[i+6 : i+9])

			latitude := float64(latRaw) / 10000.0
			longitude := float64(lonRaw) / 10000.0
			altitude := float64(altRaw) / 100.0

			sensorData["latitude"] = latitude
			sensorData["longitude"] = longitude
			sensorData["altitude"] = altitude

			// Only set location if coordinates are valid
			if latitude != 0 || longitude != 0 {
				if components.ValidateCoordinates(latitude, longitude) == nil {
					location = &components.Location{
						Latitude:  latitude,
						Longitude: longitude,
						Altitude:  altitude,
					}
				}
			}
			i += 9

		case LPPUnixTime:
			// 4 bytes, Unix timestamp
			if i+4 > len(bytes) {
				break
			}
			rawTime := binary.BigEndian.Uint32(bytes[i : i+4])
			sensorData["unix_time"] = rawTime
			sensorData["timestamp"] = time.Unix(int64(rawTime), 0).UTC()
			i += 4

		case LPPAccelerometer:
			// 6 bytes: X(2), Y(2), Z(2), signed, /1000 g
			if i+6 > len(bytes) {
				break
			}
			x := float64(int16(binary.BigEndian.Uint16(bytes[i:i+2]))) / 1000.0
			y := float64(int16(binary.BigEndian.Uint16(bytes[i+2:i+4]))) / 1000.0
			z := float64(int16(binary.BigEndian.Uint16(bytes[i+4:i+6]))) / 1000.0
			accel := map[string]float64{"x": x, "y": y, "z": z}
			sensorData[fmt.Sprintf("accelerometer_ch%d", channel)] = accel
			sensorData["accelerometer"] = accel
			i += 6

		default:
			// For safety, break to avoid parsing errors
			break
		}
	}

	return &components.ParsedData{
		DeviceEUI:    devEUI,
		DeviceType:   DeviceTypeTBeam,
		Timestamp:    payload.Timestamp,
		Location:     location,
		SensorData:   sensorData,
		BatteryLevel: batteryLevel,
		RawData:      encoded,
	}, nil
}

// ParseToEntities creates entities for T-Beam device
func (p *TBeamParser) ParseToEntities(orgSlug, model string, payload *components.RawPayload, deviceLocation *components.Location) ([]components.Entity, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = components.ExtractDevEUI(payload.Metadata)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI is required")
	}

	// Parse the payload first
	parsed, err := p.ParsePayload(payload)
	if err != nil {
		return nil, err
	}

	var entities []components.Entity
	timestamp := payload.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	modelID := "lilygo_tbeam"

	// Location Entity
	if parsed.Location != nil {
		locationAttrs := map[string]interface{}{
			"source":       "gps",
			"gps_capable":  true,
			"device_model": "LILYGO T-Beam",
			"latitude":     parsed.Location.Latitude,
			"longitude":    parsed.Location.Longitude,
		}
		if parsed.Location.Altitude != 0 {
			locationAttrs["altitude"] = parsed.Location.Altitude
		}

		entities = append(entities, components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "location"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("location"),
				orgSlug, "lilygo", modelID, devEUI, "location",
			),
			EntityType:  "location",
			DeviceClass: "location",
			Name:        "Location",
			State:       "home",
			DisplayType: []string{"map"},
			Attributes:  locationAttrs,
			Enabled:     true,
			Timestamp:   timestamp,
		})
	}

	// Temperature Entity
	if temp, ok := parsed.SensorData["temperature"].(float64); ok {
		entities = append(entities, components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "temperature"),
			EntityID: components.GenerateEntityID(
				"sensor",
				orgSlug, "lilygo", modelID, devEUI, "temperature",
			),
			EntityType:  "sensor",
			DeviceClass: "temperature",
			Name:        "Temperature",
			State:       temp,
			UnitOfMeas:  "°C",
			DisplayType: []string{"chart", "gauge", "value"},
			Attributes: map[string]interface{}{
				"device_model": "LILYGO T-Beam",
				"sensor_type":  "external",
			},
			Enabled:   true,
			Timestamp: timestamp,
		})
	}

	// Humidity Entity
	if humidity, ok := parsed.SensorData["humidity"].(float64); ok {
		entities = append(entities, components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "humidity"),
			EntityID: components.GenerateEntityID(
				"sensor",
				orgSlug, "lilygo", modelID, devEUI, "humidity",
			),
			EntityType:  "sensor",
			DeviceClass: "humidity",
			Name:        "Humidity",
			State:       humidity,
			UnitOfMeas:  "%",
			DisplayType: []string{"chart", "gauge", "value"},
			Attributes: map[string]interface{}{
				"device_model": "LILYGO T-Beam",
				"sensor_type":  "external",
			},
			Enabled:   true,
			Timestamp: timestamp,
		})
	}

	// Pressure Entity
	if pressure, ok := parsed.SensorData["pressure"].(float64); ok {
		entities = append(entities, components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "pressure"),
			EntityID: components.GenerateEntityID(
				"sensor",
				orgSlug, "lilygo", modelID, devEUI, "pressure",
			),
			EntityType:  "sensor",
			DeviceClass: "pressure",
			Name:        "Pressure",
			State:       pressure,
			UnitOfMeas:  "hPa",
			DisplayType: []string{"chart", "gauge", "value"},
			Attributes: map[string]interface{}{
				"device_model": "LILYGO T-Beam",
				"sensor_type":  "external",
			},
			Enabled:   true,
			Timestamp: timestamp,
		})
	}

	// Illuminance Entity
	if illuminance, ok := parsed.SensorData["illuminance"].(float64); ok {
		entities = append(entities, components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "illuminance"),
			EntityID: components.GenerateEntityID(
				"sensor",
				orgSlug, "lilygo", modelID, devEUI, "illuminance",
			),
			EntityType:  "sensor",
			DeviceClass: "illuminance",
			Name:        "Illuminance",
			State:       illuminance,
			UnitOfMeas:  "lx",
			DisplayType: []string{"chart", "gauge", "value"},
			Attributes: map[string]interface{}{
				"device_model": "LILYGO T-Beam",
				"sensor_type":  "external",
			},
			Enabled:   true,
			Timestamp: timestamp,
		})
	}

	// Digital Input Entity (if present)
	for key, value := range parsed.SensorData {
		if strings.HasPrefix(key, "digital_ch") {
			channel := strings.TrimPrefix(key, "digital_ch")
			if val, ok := value.(bool); ok {
				entities = append(entities, components.Entity{
					UniqueID: components.GenerateUniqueID(model, devEUI, fmt.Sprintf("digital_%s", channel)),
					EntityID: components.GenerateEntityID(
						"binary_sensor",
						orgSlug, "lilygo", modelID, devEUI, fmt.Sprintf("digital_%s", channel),
					),
					EntityType:  "binary_sensor",
					DeviceClass: "signal",
					Name:        fmt.Sprintf("Digital Input %s", channel),
					State:       val,
					Attributes: map[string]interface{}{
						"device_model": "LILYGO T-Beam",
						"channel":      channel,
					},
					Enabled:   true,
					Timestamp: timestamp,
				})
			}
		}
	}

	return entities, nil
}

func extractPayloadData(payload interface{}) string {
	switch v := payload.(type) {
	case string:
		return v
	case map[string]interface{}:
		if uplink, ok := v["uplinkEvent"].(map[string]interface{}); ok {
			if data, ok := uplink["data"].(string); ok && data != "" {
				return data
			}
		}

		if decoded, ok := v["decoded_raw_data"].(map[string]interface{}); ok {
			if uplink, ok := decoded["uplinkEvent"].(map[string]interface{}); ok {
				if data, ok := uplink["data"].(string); ok && data != "" {
					return data
				}
			}
		}

		for _, key := range []string{"data", "payload", "frm_payload", "frmPayload", "payload_hex"} {
			if val, ok := v[key].(string); ok && val != "" {
				trimmed := strings.TrimSpace(val)
				if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
					continue
				}
				return val
			}
		}
	}
	return ""
}

func decodePayloadBytes(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, fmt.Errorf("empty payload data")
	}

	// Try hex decode first
	if decoded, err := hex.DecodeString(encoded); err == nil && len(decoded) > 0 {
		return decoded, nil
	}

	// Try base64 decode
	if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil && len(decoded) > 0 {
		return decoded, nil
	}

	return nil, fmt.Errorf("failed to decode payload as hex or base64")
}
