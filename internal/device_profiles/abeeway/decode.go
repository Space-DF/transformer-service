package abeeway

import (
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

const coordScale = 10000000.0

// Decode extracts sensor readings and location from an Abeeway Industrial Tracker uplink.
// Binary protocol: 4-byte header (msgType, status, battery, temp) + message-specific data.
func Decode(payload *common.RawPayload) (map[string]interface{}, *common.Location) {
	sensors := make(map[string]interface{})
	var location *common.Location

	extractMetadata(payload.Metadata, sensors, &location)
	if len(sensors) > 0 {
		return sensors, location
	}

	b := common.ExtractBytes(payload)
	if len(b) < 5 {
		return sensors, location
	}

	msgType := b[0]
	status := b[1]
	battery := b[2]
	temp := b[3]
	data := b[5:]

	battV := common.AbeewayBatV(battery)
	sensors["battery_voltage"] = battV
	sensors["battery_percent"] = common.LinearBatPct(battV, 3.0, 4.2)

	if temp > 127 {
		sensors["temperature"] = float64(int(temp)-256) / 2.0
	} else {
		sensors["temperature"] = float64(temp) / 2.0
	}

	sensors["sos_alert"] = (status & 0x10) != 0
	sensors["motion"] = (status & 0x04) != 0

	switch msgType {
	case 0x03, 0x0E:
		parseGPSPosition(data, sensors, &location)
	}

	return sensors, location
}

func parseGPSPosition(data []byte, sensors map[string]interface{}, loc **common.Location) {
	if len(data) < 1 {
		return
	}
	posType := data[0]

	switch {
	case (posType == 0x00 || posType == 0x01 || posType == 0x04) && len(data) >= 9:
		latOff, lonOff := 1, 5
		if len(data) >= 17 {
			latOff, lonOff = 2, 6
		}
		lat := float64(common.I32BE(data, latOff)) / coordScale
		lon := float64(common.I32BE(data, lonOff)) / coordScale
		if (lat != 0 || lon != 0) && common.ValidateCoordinates(lat, lon) == nil {
			*loc = &common.Location{Latitude: lat, Longitude: lon}
		}
		if len(data) >= 17 {
			sensors["heading"] = float64(common.U16BE(data, 12))
			sensors["speed"] = float64(common.U16BE(data, 14))
		}

	case (posType == 0x07 || posType == 0x09) && len(data) >= 13:
		posStatus := data[1]
		if (posStatus & 0x01) != 0 {
			lat := float64(common.I32BE(data, 2)) / coordScale
			lon := float64(common.I32BE(data, 6)) / coordScale
			if (lat != 0 || lon != 0) && common.ValidateCoordinates(lat, lon) == nil {
				*loc = &common.Location{Latitude: lat, Longitude: lon}
			}
		}
	}
}

func extractMetadata(meta map[string]interface{}, out map[string]interface{}, loc **common.Location) {
	for _, key := range []string{"decoded_payload", "object"} {
		src, ok := meta[key].(map[string]interface{})
		if !ok {
			continue
		}
		if *loc == nil {
			if l := common.ExtractGPS(src); l != nil {
				*loc = l
			}
		}
		for _, field := range []string{"battery_voltage", "battery_percent", "temperature", "speed", "heading"} {
			if _, exists := out[field]; !exists {
				if v, ok := src[field].(float64); ok {
					out[field] = v
				}
			}
		}
		for _, field := range []string{"sos_alert", "motion"} {
			if _, exists := out[field]; !exists {
				if v, ok := src[field].(bool); ok {
					out[field] = v
				}
			}
		}
	}
}
