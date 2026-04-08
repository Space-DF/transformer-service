package wlbv1

import (
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

// Decode extracts sensor readings and location from a WLBV1 uplink.
// Relies entirely on LNS-decoded metadata; no raw byte parsing.
func Decode(payload *common.RawPayload) (map[string]interface{}, *common.Location) {
	sensors := make(map[string]interface{})
	var location *common.Location

	extractMetadata(payload.Metadata, sensors, &location)

	return sensors, location
}

func extractMetadata(meta map[string]interface{}, out map[string]interface{}, loc **common.Location) {
	tryGPS := func(src map[string]interface{}) {
		if *loc == nil {
			if l := common.ExtractGPS(src); l != nil {
				*loc = l
			}
		}
	}

	tryBattery := func(src map[string]interface{}) {
		if _, exists := out["battery"]; exists {
			return
		}
		for _, k := range []string{"vBat", "battery", "battery_voltage", "volt"} {
			if v, ok := src[k].(float64); ok {
				out["battery"] = v
				return
			}
		}
	}

	tryWaterDepth := func(src map[string]interface{}) {
		if _, exists := out["water_depth"]; exists {
			return
		}
		for _, k := range []string{"waterlevel_cm", "water_depth", "water_level", "distance"} {
			if v, ok := src[k].(float64); ok {
				out["water_depth"] = v
				return
			}
		}
	}

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
}
