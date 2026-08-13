package sensecap_t1000e

import (
	"fmt"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

const coordScale = 1000000.0

type frame struct {
	id   byte
	data []byte
}

// Decode extracts sensor readings and location from a SenseCAP T1000E uplink.
// The T1000E uses Seeed framed data IDs 0x1E through 0x26.
func Decode(payload *common.RawPayload) (map[string]interface{}, *common.Location) {
	sensors := make(map[string]interface{})

	if payload.FPort == 199 || payload.FPort == 192 {
		sensors["raw_system_payload"] = payload.Data
		return sensors, nil
	}

	frames := unpack(common.ExtractBytes(payload))
	var location *common.Location
	for _, f := range frames {
		decodeFrame(f, sensors, &location)
	}

	return sensors, location
}

func unpack(b []byte) []frame {
	frames := make([]frame, 0)
	for len(b) > 0 {
		size := frameSize(b)
		if size == 0 || len(b) < size {
			return frames
		}
		frames = append(frames, frame{id: b[0], data: b[1:size]})
		b = b[size:]
	}
	return frames
}

func frameSize(b []byte) int {
	switch b[0] {
	case 0x1E:
		return 13
	case 0x1F:
		return 21
	case 0x20, 0x21:
		if len(b) < 14 {
			return 0
		}
		scanCount := int(b[13])
		if scanCount < 1 {
			return 0
		}
		return 21 + (scanCount-1)*7
	case 0x22:
		return 15
	case 0x23, 0x24:
		if len(b) < 8 {
			return 0
		}
		scanCount := int(b[7])
		if scanCount < 1 {
			return 0
		}
		return 15 + (scanCount-1)*7
	case 0x25:
		return 13
	case 0x26:
		return 7
	default:
		return 0
	}
}

func decodeFrame(f frame, out map[string]interface{}, loc **common.Location) {
	out["packet_id"] = fmt.Sprintf("0x%02X", f.id)

	switch f.id {
	case 0x1E:
		decodeStatus(f.data, out)
	case 0x1F:
		decodeHeaderWithSensorAnd3Axis(f.data, out)
		parseLocation(f.data, 12, loc)
	case 0x20:
		decodeHeaderWithSensorAnd3Axis(f.data, out)
		parseScan(f.data, 12, "wifi_scan", out)
	case 0x21:
		decodeHeaderWithSensorAnd3Axis(f.data, out)
		parseScan(f.data, 12, "ble_scan", out)
	case 0x22:
		decodeHeaderWithSensor(f.data, out)
		parseLocation(f.data, 6, loc)
	case 0x23:
		decodeHeaderWithSensor(f.data, out)
		parseScan(f.data, 6, "wifi_scan", out)
	case 0x24:
		decodeHeaderWithSensor(f.data, out)
		parseScan(f.data, 6, "ble_scan", out)
	case 0x25:
		decodeHeaderWithSensorAnd3Axis(f.data, out)
	case 0x26:
		decodeHeaderWithSensor(f.data, out)
	}
}

func decodeStatus(b []byte, out map[string]interface{}) {
	if len(b) < 12 {
		return
	}
	out["battery_level"] = float64(b[0])
	out["firmware_version"] = fmt.Sprintf("%d.%d", b[1], b[2])
	out["hardware_version"] = fmt.Sprintf("%d.%d", b[3], b[4])
	out["positioning_strategy"] = float64(b[5])
	out["uplink_interval"] = float64(common.U16BE(b, 6))
	out["accelerometer_enabled"] = b[8] != 0
	out["sos_mode"] = float64(b[9])
	out["wifi_scan_limit"] = float64(b[10])
	out["beacon_scan_limit"] = float64(b[11])
}

func decodeHeaderWithSensorAnd3Axis(b []byte, out map[string]interface{}) {
	decodeHeaderWithSensor(b, out)
	if len(b) < 12 {
		return
	}
	out["accelerometer_x"] = nullableSignedSensor(b[6:8], 1)
	out["accelerometer_y"] = nullableSignedSensor(b[8:10], 1)
	out["accelerometer_z"] = nullableSignedSensor(b[10:12], 1)
}

func decodeHeaderWithSensor(b []byte, out map[string]interface{}) {
	if len(b) < 6 {
		return
	}
	parseEventStatus(b[0], out)
	out["battery_level"] = float64(b[1])
	if temperature := nullableSignedSensor(b[2:4], 10); temperature != nil {
		out["temperature"] = *temperature
	}
	if light := nullableSignedSensor(b[4:6], 1); light != nil {
		out["light"] = *light
	}
}

func parseEventStatus(es byte, out map[string]interface{}) {
	out["motion"] = (es&0x01) != 0 || (es&0x02) != 0 || (es&0x04) != 0
	out["shock_event"] = (es & 0x08) != 0
	out["temperature_event"] = (es & 0x10) != 0
	out["light_event"] = (es & 0x20) != 0
	out["sos_alert"] = (es & 0x40) != 0
	out["press_once_event"] = (es & 0x80) != 0
}

func parseLocation(b []byte, off int, loc **common.Location) {
	if len(b) < off+8 {
		return
	}

	lon := signedBE(b[off:off+4]) / coordScale
	lat := signedBE(b[off+4:off+8]) / coordScale
	if common.ValidateCoordinates(lat, lon) == nil {
		*loc = &common.Location{Latitude: lat, Longitude: lon}
	}
}

func parseScan(b []byte, countOff int, key string, out map[string]interface{}) {
	if len(b) <= countOff {
		return
	}
	count := int(b[countOff])
	if count < 1 || len(b) < countOff+1+count*7 {
		return
	}

	scans := make([]map[string]interface{}, 0, count)
	pairOff := countOff + 1
	for i := 0; i < count; i++ {
		pair := b[pairOff+i*7 : pairOff+(i+1)*7]
		if isEmptyMAC(pair[:6]) {
			continue
		}
		scans = append(scans, map[string]interface{}{
			"mac":  fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X", pair[0], pair[1], pair[2], pair[3], pair[4], pair[5]),
			"rssi": signedBE(pair[6:7]),
		})
	}
	if len(scans) > 0 {
		out[key] = scans
	}
}

func isEmptyMAC(b []byte) bool {
	for _, v := range b {
		if v != 0xFF {
			return false
		}
	}
	return true
}

func nullableSignedSensor(b []byte, divisor float64) *float64 {
	if isNull(b) {
		return nil
	}
	value := signedBE(b) / divisor
	return &value
}

func isNull(b []byte) bool {
	if len(b) == 0 || b[0] != 0x80 {
		return false
	}
	for _, v := range b[1:] {
		if v != 0 {
			return false
		}
	}
	return true
}

func signedBE(b []byte) float64 {
	switch len(b) {
	case 1:
		return float64(int8(b[0]))
	case 2:
		return float64(common.I16BE(b, 0))
	case 4:
		return float64(common.I32BE(b, 0))
	default:
		return 0
	}
}
