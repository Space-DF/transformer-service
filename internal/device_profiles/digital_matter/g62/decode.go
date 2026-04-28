package g62

import (
	"fmt"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

func tripTypeName(b byte) string {
	switch b & 0x3 {
	case 0:
		return "none"
	case 1:
		return "ignition"
	case 2:
		return "movement"
	case 3:
		return "run_detect"
	default:
		return "unknown"
	}
}

func signedLatLon(raw uint32) float64 {
	if raw >= 0x80000000 {
		return (float64(raw) - 4294967296.0) / 1e7
	}
	return float64(raw) / 1e7
}

func decodeLat(b0, b1, b2, b3 byte) float64 {
	raw := uint32(b0&0xF0) | uint32(b1)<<8 | uint32(b2)<<16 | uint32(b3)<<24
	return signedLatLon(raw)
}

func decodeLon(b4, b5, b6, b7 byte) float64 {
	raw := uint32(b4&0xF0) | uint32(b5)<<8 | uint32(b6)<<16 | uint32(b7)<<24
	return signedLatLon(raw)
}

func Decode(payload *common.RawPayload) (map[string]interface{}, *common.Location) {
	sensors := make(map[string]interface{})
	var location *common.Location

	b := common.ExtractBytes(payload)
	if len(b) == 0 {
		return sensors, location
	}

	switch payload.FPort {
	case 1:
		decodeFullData(b, sensors, &location)
	case 2:
		decodeDataPart1(b, sensors, &location)
	case 3:
		decodeDataPart2(b, sensors)
	case 4:
		decodeOdometer(b, sensors)
	case 5:
		decodeDownlinkAck(b, sensors)
	default:
		sensors["warning"] = fmt.Sprintf("unknown fPort %d", payload.FPort)
	}

	return sensors, location
}

func decodeFullData(b []byte, out map[string]interface{}, loc **common.Location) {
	if len(b) != 17 && len(b) < 19 {
		out["warning"] = fmt.Sprintf("full data payload invalid length: %d bytes", len(b))
		return
	}

	out["trip_type"] = tripTypeName(b[0])

	lat := decodeLat(b[0], b[1], b[2], b[3])
	lon := decodeLon(b[4], b[5], b[6], b[7])

	if common.ValidateCoordinates(lat, lon) == nil {
		*loc = &common.Location{Latitude: lat, Longitude: lon}
	}

	out["v_ext_good"] = (b[0] & 0x04) != 0
	out["gps_current"] = (b[0] & 0x08) != 0
	out["ignition"] = (b[4] & 0x01) != 0
	out["dig_in_1"] = (b[4] & 0x02) != 0
	out["dig_in_2"] = (b[4] & 0x04) != 0
	out["dig_out"] = (b[4] & 0x08) != 0

	out["heading"] = float64(b[8]) * 2
	out["speed"] = float64(b[9])
	out["battery_voltage"] = float64(b[10]) * 0.02
	out["external_voltage"] = float64(common.U16LE(b, 11)) * 0.001
	out["analog_input"] = float64(common.U16LE(b, 13)) * 0.001

	tempC := int(b[15])
	if tempC >= 0x80 {
		tempC -= 0x100
	}
	out["temperature"] = float64(tempC)
	out["gps_accuracy"] = float64(b[16])

	if len(b) >= 19 {
		ts := common.U16LE(b, 17)
		out["timestamp_raw"] = float64(ts)
	}
}

func decodeDataPart1(b []byte, out map[string]interface{}, loc **common.Location) {
	if len(b) != 11 {
		out["warning"] = fmt.Sprintf("data part 1 payload invalid length: %d bytes", len(b))
		return
	}

	out["trip_type"] = tripTypeName(b[0])

	lat := decodeLat(b[0], b[1], b[2], b[3])
	lon := decodeLon(b[4], b[5], b[6], b[7])

	if common.ValidateCoordinates(lat, lon) == nil {
		*loc = &common.Location{Latitude: lat, Longitude: lon}
	}

	out["v_ext_good"] = (b[0] & 0x04) != 0
	out["gps_current"] = (b[0] & 0x08) != 0
	out["ignition"] = (b[4] & 0x01) != 0
	out["dig_in_1"] = (b[4] & 0x02) != 0
	out["dig_in_2"] = (b[4] & 0x04) != 0
	out["dig_out"] = (b[4] & 0x08) != 0

	out["heading"] = float64(b[8]) * 2
	out["speed"] = float64(b[9])
	out["battery_voltage"] = float64(b[10]) * 0.02
}

func decodeDataPart2(b []byte, out map[string]interface{}) {
	if len(b) != 6 && len(b) < 8 {
		out["warning"] = fmt.Sprintf("data part 2 payload invalid length: %d bytes", len(b))
		return
	}

	out["external_voltage"] = float64(common.U16LE(b, 0)) * 0.001
	out["analog_input"] = float64(common.U16LE(b, 2)) * 0.001

	tempC := int(b[4])
	if tempC >= 0x80 {
		tempC -= 0x100
	}
	out["temperature"] = float64(tempC)
	out["gps_accuracy"] = float64(b[5])

	if len(b) >= 8 {
		ts := common.U16LE(b, 6)
		out["timestamp_raw"] = float64(ts)
	}
}

func decodeOdometer(b []byte, out map[string]interface{}) {
	if len(b) != 8 {
		out["warning"] = fmt.Sprintf("odometer payload invalid length: %d bytes", len(b))
		return
	}

	runtimeS := uint64(common.U32LE(b, 0))
	days := runtimeS / 86400
	remaining := runtimeS % 86400
	hours := remaining / 3600
	remaining %= 3600
	minutes := remaining / 60
	seconds := remaining % 60

	out["runtime"] = fmt.Sprintf("%dd%dh%dm%ds", days, hours, minutes, seconds)
	out["runtime_seconds"] = float64(runtimeS)
	out["odometer"] = float64(common.U32LE(b, 4)) * 0.01
}

func decodeDownlinkAck(b []byte, out map[string]interface{}) {
	if len(b) != 3 {
		out["warning"] = fmt.Sprintf("downlink ack payload invalid length: %d bytes", len(b))
		return
	}

	out["downlink_ack"] = (b[0] & 0x80) != 0
	out["sequence"] = float64(b[0] & 0x7F)
	out["firmware"] = fmt.Sprintf("%d.%d", b[1], b[2])
}
