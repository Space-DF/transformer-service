package lsn50v2_s31

import (
	"log"
	"time"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

// Mode 31 constant (min/max report for SHT31 thresholds).
const ModeMinMax = 31

// Decode extracts sensor readings from a Dragino LSN50v2-S31/S31B uplink.
// Based on V1.8.0 TTN Decoder from Dragino.
//
// fPort 2 — Uplink report (11 bytes, big-endian):
//
//	[0-1]  BatV            uint16 BE / 1000 V
//	[2-5]  Data_time       uint32 BE (unix timestamp in seconds)
//	[6]    flags byte:
//	         bits 2-6 = mode (31 = min/max report, else normal)
//	         bit  0   = exti_trigger (1=TRUE, 0=FALSE)
//	         bit  7   = door_status (1=CLOSE, 0=OPEN)
//	[7-8]  TempC_SHT31    int16  BE / 10 °C
//	[9-10] Hum_SHT31      uint16 BE / 10 %
//
// When mode == 31 (min/max thresholds):
//
//	[7]  SHTEMP_MIN  int8 (°C)
//	[8]  SHTEMP_MAX  int8 (°C)
//	[9]  SHHUM_MIN   uint8 (%)
//	[10] SHHUM_MAX   uint8 (%)
//
// fPort 3 — Data log (11-byte repeated records):
//
//	[i+3-4]  TempC   int16 BE / 10
//	[i+5-6]  Hum     uint16 BE / 10
//	[i+7-10] Time    uint32 BE (unix seconds)
//
// fPort 5 — Version info:
//
//	[0]    freq_band
//	[1]    sub_band
//	[2-3]  firmware version (BCD)
//	[4-6]  TDC (transmit interval, seconds)
func Decode(payload *common.RawPayload) map[string]interface{} {
	sensors := make(map[string]interface{})

	b := common.ExtractBytes(payload)
	if len(b) == 0 {
		log.Printf("[LSN50v2-S31] No payload bytes extracted")
		return sensors
	}

	log.Printf("[LSN50v2-S31] Payload bytes: %x (len=%d, fPort=%d)", b, len(b), payload.FPort)

	switch payload.FPort {
	case 2:
		parseFPort2(b, sensors)
	case 3:
		parseFPort3(b, sensors)
	case 5:
		parseFPort5(b, sensors)
	}

	log.Printf("[LSN50v2-S31] Parsed sensors: %+v", sensors)
	return sensors
}

func parseFPort2(b []byte, sensors map[string]interface{}) {
	if len(b) < 11 {
		log.Printf("[LSN50v2-S31] fPort 2 payload too short: %d bytes", len(b))
		return
	}

	sensors["node_type"] = "LSN50-S31"

	mode := (b[6] & 0x7C) >> 2
	sensors["work_mode"] = float64(mode)

	if mode == ModeMinMax {
		sensors["sht_temp_min"] = float64(b[7])
		sensors["sht_temp_max"] = float64(b[8])
		sensors["sht_hum_min"] = float64(b[9])
		sensors["sht_hum_max"] = float64(b[10])
		return
	}

	sensors["battery_voltage"] = float64(common.U16BE(b, 0)) / 1000.0
	sensors["exti_trigger"] = float64(b[6] & 0x01)
	sensors["door_status"] = float64((b[6] & 0x80) >> 7)
	sensors["temperature_sht31"] = float64(common.I16BE(b, 7)) / 10.0
	sensors["humidity_sht31"] = float64(common.U16BE(b, 9)) / 10.0

	ts := int64(common.U32BE(b, 2))
	if ts > 0 {
		sensors["data_time"] = float64(ts)
		sensors["data_time_iso"] = time.Unix(ts, 0).UTC().Format(time.RFC3339)
	}
}

func parseFPort3(b []byte, sensors map[string]interface{}) {
	sensors["node_type"] = "LSN50-S31"
	sensors["report_mode"] = float64(3)

	var entries []map[string]interface{}
	for i := 0; i+10 < len(b); i += 11 {
		temp := float64(common.I16BE(b, i+3)) / 10.0
		hum := float64(common.U16BE(b, i+5)) / 10.0
		ts := int64(common.U32BE(b, i+7))
		entry := map[string]interface{}{
			"temperature": temp,
			"humidity":    hum,
			"timestamp":   float64(ts),
		}
		if ts > 0 {
			entry["timestamp_iso"] = time.Unix(ts, 0).UTC().Format(time.RFC3339)
		}
		entries = append(entries, entry)
	}

	if len(entries) > 0 {
		sensors["datalog"] = entries
		sensors["temperature_sht31"] = entries[len(entries)-1]["temperature"]
		sensors["humidity_sht31"] = entries[len(entries)-1]["humidity"]
	}
}

func parseFPort5(b []byte, sensors map[string]interface{}) {
	if len(b) < 7 {
		log.Printf("[LSN50v2-S31] fPort 5 payload too short: %d bytes", len(b))
		return
	}

	freqBands := map[byte]string{
		0x01: "EU868", 0x02: "US915", 0x03: "IN865", 0x04: "AU915",
		0x05: "KZ865", 0x06: "RU864", 0x07: "AS923", 0x08: "AS923_1",
		0x09: "AS923_2", 0x0A: "AS923_3", 0x0F: "AS923_4",
		0x0B: "CN470", 0x0C: "EU433", 0x0D: "KR920", 0x0E: "MA869",
	}

	if band, ok := freqBands[b[0]]; ok {
		sensors["freq_band"] = band
	}

	if b[1] == 0xFF {
		sensors["sub_band"] = "NULL"
	} else {
		sensors["sub_band"] = float64(b[1])
	}

	firmMajor := float64(b[2] & 0x0F)
	firmMinor := float64((b[3] >> 4) & 0x0F)
	firmPatch := float64(b[3] & 0x0F)
	sensors["firmware_version"] = firmMajor + firmMinor/10 + firmPatch/100

	sensors["tdc_sec"] = float64(uint32(b[4])<<16 | uint32(b[5])<<8 | uint32(b[6]))
}
