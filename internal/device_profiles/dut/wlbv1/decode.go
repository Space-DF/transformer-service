package wlbv1

import (
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

// Decode extracts sensor readings and location from a WLBV1 uplink.
// Relies entirely on LNS-decoded metadata; no raw byte parsing.
func Decode(payload *common.RawPayload) (map[string]interface{}, *common.Location) {
	// Extract sensor readings and location from metadata.
	return extractMetadata(payload.Metadata)
}

// extractMetadata extracts sensor readings and location from metadata.
func extractMetadata(meta map[string]interface{}) (map[string]interface{}, *common.Location) {
	sensors := make(map[string]interface{})
	var location *common.Location

	// tryGPS sets location from a source map if not already found.
	tryGPS := func(src map[string]interface{}) {
		if location == nil {
			if l := common.ExtractGPS(src); l != nil {
				location = l
			}
		}
	}

	// tryBattery sets battery from any known key alias.
	tryBattery := func(src map[string]interface{}) {
		if _, exists := sensors["battery"]; exists {
			return
		}
		for _, k := range []string{"vBat", "battery", "battery_voltage", "volt"} {
			if v, ok := src[k].(float64); ok {
				sensors["battery"] = v
				return
			}
		}
	}

	// tryWaterDepth sets water_depth from any known key alias.
	tryWaterDepth := func(src map[string]interface{}) {
		if _, exists := sensors["water_depth"]; exists {
			return
		}
		for _, k := range []string{"waterlevel_cm", "water_depth", "water_level", "distance"} {
			if v, ok := src[k].(float64); ok {
				sensors["water_depth"] = v
				return
			}
		}
	}

	// Check each known metadata key in priority order.
	if dp, ok := meta["decoded_payload"].(map[string]interface{}); ok {
		tryGPS(dp)
		tryBattery(dp)
		tryWaterDepth(dp)
	}
	if drd, ok := meta["decoded_raw_data"].(map[string]interface{}); ok {
		if obj, ok := drd["object"].(map[string]interface{}); ok {
			tryGPS(obj)
			tryBattery(obj)
			tryWaterDepth(obj)
		}
	}
	if obj, ok := meta["object"].(map[string]interface{}); ok {
		tryGPS(obj)
		tryBattery(obj)
		tryWaterDepth(obj)
	}

	return sensors, location
}
