package common

import "math"

// AbeewayBatV converts an Abeeway raw battery byte to volts.
// Formula: (raw * 10 + 2000) / 1000
func AbeewayBatV(raw byte) float64 {
	return (float64(raw)*10.0 + 2000.0) / 1000.0
}

// YabbyBatV converts a Yabby Edge raw uint32 reading to volts.
// Formula: 2.0 + 0.007 * raw, rounded to 3 decimal places.
func YabbyBatV(raw uint32) float64 {
	return math.Round((2.0+0.007*float64(raw))*1000) / 1000
}

// LinearBatPct returns a battery percentage assuming a linear curve from
// minV (0 %) to maxV (100 %), clamped to [0, 100].
func LinearBatPct(v, minV, maxV float64) float64 {
	if v <= minV {
		return 0
	}
	if v >= maxV {
		return 100
	}
	return math.Round((v - minV) / (maxV - minV) * 100)
}
