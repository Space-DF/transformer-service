package tbeam

import (
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

// Decode extracts sensor readings and location from a T-Beam Cayenne LPP uplink.
// Cayenne LPP frame: [Channel(1), Type(1), Data(N)] repeated.
func Decode(payload *common.RawPayload) (map[string]interface{}, *common.Location) {
	sensors := make(map[string]interface{})
	var location *common.Location

	// Parse the raw binary payload.
	b := common.ExtractBytes(payload)
	if len(b) < 2 {
		return sensors, location
	}

	i := 0
	for i+1 < len(b) {
		dataType := b[i+1] // skip channel byte at b[i]
		i += 2

		switch dataType {
		case 0x67: // Temperature: int16 BE / 10 °C
			if i+2 > len(b) {
				return sensors, location
			}
			raw := int(b[i])<<8 | int(b[i+1])
			if raw > 0x7FFF {
				raw -= 0x10000
			}
			sensors["temperature"] = float64(raw) / 10.0
			i += 2

		case 0x68: // Humidity: uint8 / 2 %
			if i+1 > len(b) {
				return sensors, location
			}
			sensors["humidity"] = float64(b[i]) / 2.0
			i++

		case 0x73: // Barometer: uint16 BE / 10 hPa
			if i+2 > len(b) {
				return sensors, location
			}
			sensors["pressure"] = float64(int(b[i])<<8|int(b[i+1])) / 10.0
			i += 2

		case 0x65: // Illuminance: uint16 BE lux
			if i+2 > len(b) {
				return sensors, location
			}
			sensors["illuminance"] = float64(int(b[i])<<8 | int(b[i+1]))
			i += 2

		case 0x88: // GPS Location: 9 bytes (lat3 + lon3 + alt3) signed BE
			if i+9 > len(b) {
				return sensors, location
			}
			lat := float64(common.ReadInt24BE(b, i)) / 10000.0
			lon := float64(common.ReadInt24BE(b, i+3)) / 10000.0
			if (lat != 0 || lon != 0) && common.ValidateCoordinates(lat, lon) == nil {
				location = &common.Location{Latitude: lat, Longitude: lon}
			}
			i += 9

		case 0x00, 0x01: // Digital I/O
			if i+1 > len(b) {
				return sensors, location
			}
			i++

		case 0x02, 0x03: // Analog I/O
			if i+2 > len(b) {
				return sensors, location
			}
			i += 2

		case 0x66: // Presence
			if i+1 > len(b) {
				return sensors, location
			}
			i++

		case 0x71: // Accelerometer
			if i+6 > len(b) {
				return sensors, location
			}
			i += 6

		case 0x95: // Unix Time
			if i+4 > len(b) {
				return sensors, location
			}
			i += 4

		default:
			return sensors, location
		}
	}

	return sensors, location
}
