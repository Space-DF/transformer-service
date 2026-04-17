package netvox

import (
	"log"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

// Report type constants for R718N17
const (
	ReportTypeData      = 0x01 // Uplink report data
	CmdIDSetConfig      = 0x01 // Set configuration
	CmdIDReadConfig     = 0x02 // Read configuration
	CmdIDSetConfigResp  = 0x81 // Set config response
	CmdIDReadConfigResp = 0x82 // Read config response
)

// Device type constant
const (
	DeviceTypeR718N17 = 0x49
)

// Decode extracts sensor readings from a Netvox R718N17 uplink.
// The payload format is BIG-ENDIAN.
//
// Uplink (Port 6): Report Data
//
//	[01][49][01][bat][curr_hi][curr_lo][mult][00][00][00][00]
//
// Downlink (Port 7): Set/Read Configuration
//
//	Set Config: [01][49][min_hi][min_lo][max_hi][max_lo][chg_hi][chg_lo][00][00][00]
//	Read Config: [02][49][00][00][00][00][00][00][00][00][00]
//
// Config Response (Port 7):
//
//	Set Response: [81][status][00...]
//	Read Response: [82][min_hi][min_lo][max_hi][max_lo][chg_hi][chg_lo][00]
func Decode(payload *common.RawPayload) map[string]interface{} {
	sensors := make(map[string]interface{})

	// Parse the raw binary payload
	b := common.ExtractBytes(payload)
	if len(b) == 0 {
		log.Printf("[R718N17] No payload bytes extracted")
		return sensors
	}

	log.Printf("[R718N17] Payload bytes: %x (len=%d)", b, len(b))

	// Minimum payload length: [version][device_type][report_type] = 3 bytes
	if len(b) < 3 {
		log.Printf("[R718N17] Payload too short: %d bytes", len(b))
		return sensors
	}

	version := b[0]
	deviceType := b[1]

	log.Printf("[R718N17] version=0x%02x, deviceType=0x%02x", version, deviceType)

	sensors["version"] = int(version)
	sensors["device_type"] = deviceType
	sensors["device_name"] = Model

	// Validate device type
	if deviceType != DeviceTypeR718N17 {
		sensors["warning"] = "Device type 0x49 expected, got 0x" + twoHexDigits(deviceType)
		log.Printf("[R718N17] %v", sensors["warning"])
		return sensors
	}

	switch payload.FPort {
	case 6:
		parseUplinkData(b, sensors)
	case 7:
		parseConfigResponse(b, sensors)
	}

	return sensors
}

// parseUplinkData handles fPort 6 report data (version info + sensor readings).
func parseUplinkData(b []byte, sensors map[string]interface{}) {
	reportType := b[2]
	sensors["report_type"] = float64(reportType)

	log.Printf("[R718N17] reportType=0x%02x (fPort 6)", reportType)

	if reportType == 0x00 && len(b) >= 9 {
		log.Printf("[R718N17] Parsing version report")
		sensors["report_mode"] = float64(0)
		sensors["sw_version"] = float64(b[3]) / 10
		sensors["hw_version"] = float64(b[4])
		sensors["date_code"] = twoHexDigits(b[5]) + twoHexDigits(b[6]) +
			twoHexDigits(b[7]) + twoHexDigits(b[8])
		return
	}

	if reportType == ReportTypeData && len(b) >= 7 {
		log.Printf("[R718N17] Parsing report data")
		parseReportData(b, sensors)
		log.Printf("[R718N17] Parsed sensors: %+v", sensors)
	} else {
		sensors["warning"] = "Unknown report type: 0x" + twoHexDigits(reportType)
		log.Printf("[R718N17] %v", sensors["warning"])
	}
}

// parseConfigResponse handles fPort 7 config responses (0x81 set, 0x82 read).
func parseConfigResponse(b []byte, sensors map[string]interface{}) {
	cmdID := b[0]

	if cmdID == CmdIDSetConfigResp {
		log.Printf("[R718N17] Config response: 0x81 (Set Config)")
		parseSetConfigResponse(b, sensors)
		return
	}

	if cmdID == CmdIDReadConfigResp {
		log.Printf("[R718N17] Config response: 0x82 (Read Config)")
		parseReadConfigResponse(b, sensors)
		return
	}

	sensors["warning"] = "Unhandled fPort 7 cmdID: 0x" + twoHexDigits(cmdID)
	log.Printf("[R718N17] %v", sensors["warning"])
}

// parseReportData parses uplink report data
// Format: [01][49][01][bat][curr_hi][curr_lo][mult][00][00][00][00]
func parseReportData(b []byte, sensors map[string]interface{}) {
	sensors["report_mode"] = float64(1)

	// Battery: bit7 = low battery flag, bits0-6 = voltage * 0.1V
	rawBat := b[3]
	lowBattery := (rawBat & 0x80) != 0
	batteryVoltage := float64(rawBat&0x7F) * 0.1
	sensors["battery_voltage"] = batteryVoltage
	if lowBattery {
		sensors["low_battery"] = float64(1)
	}

	// Current: 2 bytes big-endian, unit 1mA
	currentRaw := common.U16BE(b, 4)
	sensors["current_raw_ma"] = float64(currentRaw)

	multiplier := int(b[6])
	sensors["multiplier"] = float64(multiplier)

	// Calculate actual current
	sensors["current_ma"] = float64(currentRaw) * float64(multiplier)
}

// parseSetConfigResponse parses set configuration response
// Format: [81][status][00...]
func parseSetConfigResponse(b []byte, sensors map[string]interface{}) {
	sensors["report_mode"] = float64(2)
	sensors["config_type"] = float64(1)

	if len(b) >= 3 {
		status := float64(0)
		if b[2] == 0x00 {
			status = float64(0)
		} else if b[2] == 0x01 {
			status = float64(1)
		}
		sensors["config_status"] = status
	}
}

// parseReadConfigResponse parses read configuration response
// Format: [82][min_hi][min_lo][max_hi][max_lo][chg_hi][chg_lo][00]
func parseReadConfigResponse(b []byte, sensors map[string]interface{}) {
	sensors["report_mode"] = float64(2)
	sensors["config_type"] = float64(2)

	if len(b) >= 9 {
		sensors["min_time_s"] = float64(common.U16BE(b, 2))
		sensors["max_time_s"] = float64(common.U16BE(b, 4))
		sensors["current_change_ma"] = float64(common.U16BE(b, 6))
	}
}

// twoHexDigits formats a byte as 2-digit uppercase hex
func twoHexDigits(b byte) string {
	const hexDigits = "0123456789ABCDEF"
	return string(hexDigits[(b>>4)&0x0F]) + string(hexDigits[b&0x0F])
}
