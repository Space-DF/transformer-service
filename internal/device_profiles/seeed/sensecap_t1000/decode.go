package sensecap_t1000

import (
	"math"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

const coordScale = 10000000.0

// Decode extracts sensor readings and location from a SenseCAP T1000 uplink.
// Binary Seeed custom protocol with packet ID routing.
func Decode(payload *common.RawPayload) (map[string]interface{}, *common.Location) {
	sensors := make(map[string]interface{})
	var location *common.Location

	// Parse the raw binary payload.
	b := common.ExtractBytes(payload)
	if len(b) < 1 {
		return sensors, location
	}

	switch b[0] {
	case 0x01: // Device Status Event Mode (47 bytes)
		if len(b) >= 47 {
			sensors["battery_level"] = float64(b[1])
		}

	case 0x02: // Device Status Periodic Mode (16 bytes)
		if len(b) >= 16 {
			sensors["battery_level"] = float64(b[1])
		}

	case 0x05: // Heartbeat (5 bytes)
		if len(b) >= 5 {
			sensors["battery_level"] = float64(b[1])
		}

	case 0x06: // GNSS Location + Sensor (22 bytes)
		if len(b) >= 22 {
			parseEventStatus(common.U24BE(b, 1), sensors)
			parseGNSSCoords(b, 9, &location)
			sensors["temperature"] = float64(common.I16BE(b, 17)) / 10.0
			sensors["light"] = float64(common.U16BE(b, 19))
			sensors["battery_level"] = float64(b[21])
		}

	case 0x07: // WiFi Location + Sensor (42 bytes)
		if len(b) >= 42 {
			parseEventStatus(common.U24BE(b, 1), sensors)
			sensors["temperature"] = float64(common.I16BE(b, 37)) / 10.0
			sensors["light"] = float64(common.U16BE(b, 39))
			sensors["battery_level"] = float64(b[41])
		}

	case 0x08: // BLE Location + Sensor (35 bytes)
		if len(b) >= 35 {
			parseEventStatus(common.U24BE(b, 1), sensors)
			sensors["temperature"] = float64(common.I16BE(b, 30)) / 10.0
			sensors["light"] = float64(common.U16BE(b, 32))
			sensors["battery_level"] = float64(b[34])
		}

	case 0x09: // GNSS Location Only (18 bytes)
		if len(b) >= 18 {
			parseEventStatus(common.U24BE(b, 1), sensors)
			parseGNSSCoords(b, 9, &location)
			sensors["battery_level"] = float64(b[17])
		}

	case 0x0A: // WiFi Location Only (38 bytes)
		if len(b) >= 38 {
			parseEventStatus(common.U24BE(b, 1), sensors)
			sensors["battery_level"] = float64(b[37])
		}

	case 0x0B: // BLE Location Only (31 bytes)
		if len(b) >= 31 {
			parseEventStatus(common.U24BE(b, 1), sensors)
			sensors["battery_level"] = float64(b[30])
		}

	case 0x0D: // Error Code (5 bytes)
		if len(b) >= 5 {
			sensors["error_code"] = float64(common.U32BE(b, 1))
		}

	case 0x11: // Positioning Status + Sensor (14 bytes)
		if len(b) >= 14 {
			parseEventStatus(common.U24BE(b, 2), sensors)
			sensors["temperature"] = float64(common.I16BE(b, 9)) / 10.0
			sensors["light"] = float64(common.U16BE(b, 11))
			sensors["battery_level"] = float64(b[13])
		}
	}

	return sensors, location
}

func parseEventStatus(es uint32, out map[string]interface{}) {
	out["motion"] = (es&0x01) != 0 || (es&0x02) != 0
	out["shock_event"] = (es & 0x08) != 0
	out["temperature_event"] = (es & 0x10) != 0
	out["light_event"] = (es & 0x20) != 0
	out["sos_alert"] = (es & 0x40) != 0
	out["press_once_event"] = (es & 0x80) != 0
}

func parseGNSSCoords(b []byte, off int, loc **common.Location) {
	rawLat := common.I32BE(b, off)
	rawLon := common.I32BE(b, off+4)
	lat := float64(rawLat) / coordScale
	lon := float64(rawLon) / coordScale

	// Auto-swap if coordinates appear reversed.
	if math.Abs(lat) > 90 || math.Abs(lon) > 180 {
		sLat := float64(rawLon) / coordScale
		sLon := float64(rawLat) / coordScale
		if math.Abs(sLat) <= 90 && math.Abs(sLon) <= 180 {
			lat, lon = sLat, sLon
		}
	}

	if (lat != 0 || lon != 0) && common.ValidateCoordinates(lat, lon) == nil {
		*loc = &common.Location{Latitude: lat, Longitude: lon}
	}
}
