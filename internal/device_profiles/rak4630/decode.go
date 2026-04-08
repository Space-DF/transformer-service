package rak4630

import (
	"strconv"
	"strings"

	cbor "github.com/fxamacker/cbor/v2"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

// CSV column indices for the WisToolBox sensor string (0-indexed).
const (
	csvIdxTemp  = 0
	csvIdxHumid = 1
	csvIdxPres  = 2
	csvIdxLat   = 14
	csvIdxLon   = 15
	csvIdxBattV = 21
)

// Decode extracts sensor readings and location from a RAK4630 uplink.
// Strategy 1: LNS-decoded metadata fields (decoded_payload / object).
// Strategy 2: CBOR → WisToolBox sensor CSV from raw bytes.
func Decode(payload *common.RawPayload) (map[string]interface{}, *common.Location) {
	sensors := make(map[string]interface{})
	var location *common.Location

	extractMetadata(payload.Metadata, sensors, &location)

	if len(sensors) == 0 || location == nil {
		csvSensors, csvLoc := extractCSV(payload)
		for k, v := range csvSensors {
			if _, exists := sensors[k]; !exists {
				sensors[k] = v
			}
		}
		if location == nil {
			location = csvLoc
		}
	}

	return sensors, location
}

func extractMetadata(meta map[string]interface{}, out map[string]interface{}, loc **common.Location) {
	for _, key := range []string{"decoded_payload", "object"} {
		src, ok := meta[key].(map[string]interface{})
		if !ok {
			continue
		}
		if *loc == nil {
			if gps, ok := src["gps"].(map[string]interface{}); ok {
				lat, latOK := gps["latitude"].(float64)
				lon, lonOK := gps["longitude"].(float64)
				if latOK && lonOK && common.ValidateCoordinates(lat, lon) == nil {
					*loc = &common.Location{Latitude: lat, Longitude: lon}
				}
			}
		}
		for _, field := range []string{"temperature", "humidity", "pressure", "battery_voltage"} {
			if _, exists := out[field]; !exists {
				if v, ok := src[field].(float64); ok {
					out[field] = v
				}
			}
		}
	}
}

func extractCSV(payload *common.RawPayload) (map[string]interface{}, *common.Location) {
	raw := common.ExtractBytes(payload)
	if raw == nil {
		return nil, nil
	}

	var cborData map[string]interface{}
	if err := cbor.Unmarshal(raw, &cborData); err != nil {
		return nil, nil
	}

	sensor, _ := cborData["sensor"].(string)
	if sensor == "" {
		return nil, nil
	}

	return parseCSV(sensor)
}

func parseCSV(csv string) (map[string]interface{}, *common.Location) {
	cols := strings.Split(csv, ",")
	get := func(i int) (float64, bool) {
		if i >= len(cols) {
			return 0, false
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(cols[i]), 64)
		return v, err == nil
	}

	out := make(map[string]interface{})
	if v, ok := get(csvIdxTemp); ok {
		out["temperature"] = v
	}
	if v, ok := get(csvIdxHumid); ok {
		out["humidity"] = v
	}
	if v, ok := get(csvIdxPres); ok {
		out["pressure"] = v
	}
	if v, ok := get(csvIdxBattV); ok {
		out["battery_voltage"] = v
	}

	var loc *common.Location
	lat, latOK := get(csvIdxLat)
	lon, lonOK := get(csvIdxLon)
	if latOK && lonOK && common.ValidateCoordinates(lat, lon) == nil {
		loc = &common.Location{Latitude: lat, Longitude: lon}
	}

	return out, loc
}
