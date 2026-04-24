package cubicmeter

import (
	"fmt"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

// Decode extracts sensor readings from a Quandify Cubicmeter 1.1 uplink.
// fPort 1: Status Report (28 bytes) — main periodic report.
// fPort 6: Response (variable)     — echoes a status/hardware/settings report.
// This device has no GPS; location is always nil.
func Decode(payload *common.RawPayload) (map[string]interface{}, *common.Location) {
	sensors := make(map[string]interface{})

	// Decode raw binary payload.
	b := common.ExtractBytes(payload)
	if len(b) == 0 {
		return sensors, nil
	}

	switch payload.FPort {
	case 1:
		decodeStatusReport(b, sensors)
	case 6:
		decodeResponse(b, sensors)
	}

	fmt.Printf("[cubicmeter] Decode result: %+v\n", sensors)
	return sensors, nil
}

// decodeStatusReport parses a 28-byte fPort=1 status report payload.
//
// Byte map (all multi-byte values are little-endian):
//
//	[0..3]  uptime            uint32  seconds since last restart
//	[4..5]  errorField        uint16  bit15=!isSensing, bits0–14=errorCode
//	[6..9]  totalVolume       uint32  all-time water usage in litres
//	[10..13] totalHeat        uint32  all-time heat in kCal above 30 °C
//	[22]    leakState         uint8   0=none, 3=medium, 4=large
//	[23]    batteryActive     uint8   → 1800 + byte*8  mV
//	[24]    batteryRecovered  uint8   → 1800 + byte*8  mV
//	[25]    waterTempMin      uint8   → byte*0.5 − 20  °C
//	[26]    waterTempMax      uint8   → byte*0.5 − 20  °C
//	[27]    ambientTemp       uint8   → byte*0.5 − 20  °C
func decodeStatusReport(data []byte, out map[string]interface{}) {
	if len(data) < 28 {
		return
	}

	errorField := uint16(data[4]) | uint16(data[5])<<8
	isSensing := (errorField & 0x8000) == 0
	errorCode := errorField & 0x7FFF

	out["uptime"] = float64(common.U32LE(data, 0))
	out["is_sensing"] = isSensing
	if errorCode != 0 {
		out["error_code"] = float64(errorCode)
	}
	out["total_volume"] = float64(common.U32LE(data, 6))
	out["total_heat"] = float64(common.U32LE(data, 10))
	out["leak_state"] = leakStateName(data[22])
	out["battery_active"] = (1800.0 + float64(data[23])*8.0)
	out["battery_recovered"] = (1800.0 + float64(data[24])*8.0)
	out["water_temperature_min"] = decodeCubicTemperature(data[25])
	out["water_temperature_max"] = decodeCubicTemperature(data[26])
	out["ambient_temperature"] = decodeCubicTemperature(data[27])
}

// decodeResponse parses a fPort=6 response frame.
// When the embedded type is statusReport, the inner payload is decoded as one.
func decodeResponse(data []byte, out map[string]interface{}) {
	if len(data) < 3 {
		return
	}
	// data[0]=request fPort, data[1]=status, data[2]=response type
	responseTypes := map[byte]string{0: "none", 1: "statusReport", 2: "hardwareReport", 4: "settingsReport"}
	if rt, ok := responseTypes[data[2]]; ok {
		out["response_type"] = rt
	}
	// Decode embedded status report if present (3 header bytes + 28 payload bytes).
	if data[2] == 1 && len(data) >= 31 {
		decodeStatusReport(data[3:], out)
	}
}

func decodeCubicTemperature(b byte) float64 {
	return float64(b)*0.5 - 20.0
}

func leakStateName(state byte) string {
	switch state {
	case 3:
		return "medium"
	case 4:
		return "large"
	default:
		return "none"
	}
}
