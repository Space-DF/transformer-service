package lcc01lb

import (
	"fmt"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

// Decode extracts sensor readings from a Dragino LCC01-LB uplink.
//
// fPort 2: Uplink data (10 bytes):
//
//	[0..1]  battery voltage  uint16 BE  / 1000  V
//	[2]     mod              uint8      1=no flag field, 2=with weight_flag
//	[3..5]  weight_reading   uint24 BE  raw ADC reading
//	[6]     weight_state     uint8      0=stable,1=unstable,2=overload,3=underload
//	[7..8]  scale_factor     uint16 BE  multiplier
//	[9]     weight_flag      uint8      present when mod==2
//
// actual_weight_g = weight_reading * scale_factor
//
// fPort 5: Config reply (7 bytes):
//
//	[0]     sensor model byte  0x32 = LCC01-LB
//	[1]     firmware minor/patch nibbles
//	[2]     firmware patch/sub nibbles
//	[3]     frequency band
//	[4]     sub-band (0xff = NULL)
//	[5..6]  battery voltage  uint16 BE mV
func Decode(payload *common.RawPayload) map[string]interface{} {
	sensors := make(map[string]interface{})

	// Parse the raw binary payload.
	b := common.ExtractBytes(payload)
	if len(b) == 0 {
		return sensors
	}

	switch payload.FPort {
	case 2:
		decodeUplink(b, sensors)
	case 5:
		decodeConfig(b, sensors)
	}

	fmt.Printf("[lcc01lb] Decode result: %+v\n", sensors)
	return sensors
}

func decodeUplink(data []byte, out map[string]interface{}) {
	if len(data) < 9 {
		return
	}

	batV := float64(common.U16BE(data, 0))
	mod := data[2]
	weightReading := common.U24BE(data, 3)
	weightState := data[6]
	scaleFactor := common.U16BE(data, 7)
	actualWeightG := float64(weightReading) * float64(scaleFactor)

	out["battery_voltage"] = batV
	out["mod"] = float64(mod)
	out["weight_reading"] = float64(weightReading)
	out["weight_state"] = weightStateName(weightState)
	out["scale_factor"] = float64(scaleFactor)
	out["actual_weight_g"] = actualWeightG

	if mod == 2 && len(data) >= 10 {
		out["weight_flag"] = float64(data[9])
	}
}

func decodeConfig(data []byte, out map[string]interface{}) {
	if len(data) < 7 {
		return
	}

	firmVer := fmt.Sprintf("%d.%d.%d",
		0, // major not available in this frame
		(data[1] & 0x0f),
		(data[2] >> 4 & 0x0f),
	)
	batV := float64(common.U16BE(data, 5))

	out["battery_voltage"] = batV
	out["firmware_version"] = firmVer
	out["frequency_band"] = freqBandName(data[3])
	if data[4] == 0xff {
		out["sub_band"] = "NULL"
	} else {
		out["sub_band"] = float64(data[4])
	}
}

func weightStateName(state byte) string {
	switch state {
	case 0:
		return "stable"
	case 1:
		return "unstable"
	case 2:
		return "overload"
	case 3:
		return "underload"
	default:
		return "unknown"
	}
}

func freqBandName(b byte) string {
	bands := map[byte]string{
		0x01: "EU868", 0x02: "US915", 0x03: "IN865", 0x04: "AU915",
		0x05: "KZ865", 0x06: "RU864", 0x07: "AS923", 0x08: "AS923_1",
		0x09: "AS923_2", 0x0A: "AS923_3", 0x0B: "CN470", 0x0C: "EU433",
		0x0D: "KR920", 0x0E: "MA869",
	}
	if name, ok := bands[b]; ok {
		return name
	}
	return "unknown"
}
