package rak7200

import (
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

// Decode extracts sensor readings and location from a RAK7200 uplink.
// Binary layout (little-endian):
//
//	[0..3]  int32  latitude   (/ 1e6 °)
//	[4..7]  int32  longitude  (/ 1e6 °)
//	[8]     uint8  battery    (%)
//	[9..10] int16  temperature (/ 100 °C)
func Decode(payload *common.RawPayload) (map[string]interface{}, *common.Location) {
	sensors := make(map[string]interface{})
	var location *common.Location

	extractMetadata(payload.Metadata, sensors, &location)
	if len(sensors) > 0 {
		return sensors, location
	}

	b := common.ExtractBytes(payload)
	if len(b) < 11 {
		return sensors, location
	}

	lat := float64(common.I32LE(b, 0)) / 1e6
	lon := float64(common.I32LE(b, 4)) / 1e6
	if (lat != 0 || lon != 0) && common.ValidateCoordinates(lat, lon) == nil {
		location = &common.Location{Latitude: lat, Longitude: lon}
	}

	sensors["battery"] = float64(b[8])
	sensors["temperature"] = float64(common.I16LE(b, 9)) / 100.0

	return sensors, location
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
		for _, field := range []string{"battery", "temperature"} {
			if _, exists := out[field]; !exists {
				if v, ok := src[field].(float64); ok {
					out[field] = v
				}
			}
		}
	}
}
