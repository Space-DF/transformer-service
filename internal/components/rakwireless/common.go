package rakwireless

import (
	"strconv"
	"strings"
)

// SensorFrame represents the CBOR sensor frame structure from firmware
type SensorFrame struct {
	ID     int    `cbor:"id"`
	Type   int    `cbor:"type"`
	Fmt    string `cbor:"fmt"`
	Sensor string `cbor:"sensor"`
}

// parseSensorString parses vendor-specific comma-separated sensor payloads from CBOR.
func parseSensorString(sensorStr string) map[string]float64 {
	parts := strings.Split(sensorStr, ",")

	readings := make(map[string]float64)
	get := func(idx int) (float64, bool) {
		if idx < 0 || idx >= len(parts) {
			return 0, false
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(parts[idx]), 64)
		if err != nil {
			return 0, false
		}
		return v, true
	}

	// Parse based on field positions
	if v, ok := get(0); ok {
		readings["temperature"] = v
	}
	if v, ok := get(1); ok {
		readings["humidity"] = v
	}
	if v, ok := get(2); ok {
		readings["pressure"] = v
	}
	// Index 3-15 are placeholders (*)
	if v, ok := get(16); ok {
		readings["latitude"] = v
	}
	if v, ok := get(17); ok {
		readings["longitude"] = v
	}
	if v, ok := get(18); ok {
		readings["snr_or_altitude"] = v
	}
	if v, ok := get(19); ok {
		readings["raw_signal"] = v
	}
	if v, ok := get(20); ok {
		readings["signal_quality"] = v
	}
	// Index 21-23 are additional fields
	// I leave here for temporarily future use
	if v, ok := get(24); ok {
		readings["battery_v"] = v
	}

	if len(readings) == 0 {
		return nil
	}
	return readings
}
