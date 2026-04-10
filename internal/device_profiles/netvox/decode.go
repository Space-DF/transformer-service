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
	log.Printf("[R718N17] Decode called for devEUI=%s", payload.DeviceEUI)

	// Try to extract sensor readings from metadata first
	sensors := extractMetadata(payload.Metadata)
	if len(sensors) > 0 {
		log.Printf("[R718N17] Extracted %d fields from metadata", len(sensors))
		return sensors
	}

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

	// Check if this is a config response (CmdID 0x81 or 0x82)
	// Config responses use cmdId as the first byte
	if version == CmdIDSetConfigResp {
		log.Printf("[R718N17] Config response: 0x81 (Set Config)")
		parseSetConfigResponse(b, sensors)
		return sensors
	}

	if version == CmdIDReadConfigResp {
		log.Printf("[R718N17] Config response: 0x82 (Read Config)")
		parseReadConfigResponse(b, sensors)
		return sensors
	}

	// Handle uplink report data (Port 6)
	// [version][device_type][report_type][bat][curr_hi][curr_lo][mult]...
	reportType := b[2]
	sensors["report_type"] = int(reportType)

	log.Printf("[R718N17] reportType=0x%02x", reportType)

	if reportType == ReportTypeData && len(b) >= 7 {
		log.Printf("[R718N17] Parsing report data")
		parseReportData(b, sensors)
		log.Printf("[R718N17] Parsed sensors: %+v", sensors)
	} else {
		sensors["warning"] = "Unknown report type: 0x" + twoHexDigits(reportType)
		log.Printf("[R718N17] %v", sensors["warning"])
	}

	return sensors
}

// parseReportData parses uplink report data
// Format: [01][49][01][bat][curr_hi][curr_lo][mult][00][00][00][00]
func parseReportData(b []byte, sensors map[string]interface{}) {
	sensors["report_mode"] = "report"

	// Battery: direct value * 0.1V
	batteryVoltage := float64(b[3]) * 0.1
	sensors["battery_voltage"] = batteryVoltage

	// Current: 2 bytes big-endian, unit 1mA
	currentRaw := common.U16BE(b, 4)
	sensors["current_raw_ma"] = currentRaw

	// Multiplier
	multiplier := int(b[6])
	sensors["multiplier"] = multiplier

	// Calculate actual current
	sensors["current_ma"] = float64(currentRaw) * float64(multiplier)
}

// parseSetConfigResponse parses set configuration response
// Format: [81][status][00...]
func parseSetConfigResponse(b []byte, sensors map[string]interface{}) {
	sensors["report_mode"] = "config_response"
	sensors["config_type"] = "set"

	if len(b) >= 3 {
		status := "unknown"
		if b[2] == 0x00 {
			status = "success"
		} else if b[2] == 0x01 {
			status = "failure"
		}
		sensors["config_status"] = status
	}
}

// parseReadConfigResponse parses read configuration response
// Format: [82][min_hi][min_lo][max_hi][max_lo][chg_hi][chg_lo][00]
func parseReadConfigResponse(b []byte, sensors map[string]interface{}) {
	sensors["report_mode"] = "config_response"
	sensors["config_type"] = "read"

	if len(b) >= 9 {
		sensors["min_time_s"] = common.U16BE(b, 2)
		sensors["max_time_s"] = common.U16BE(b, 4)
		sensors["current_change_ma"] = common.U16BE(b, 6)
	}
}

// extractMetadata extracts sensor readings from LNS-decoded metadata
func extractMetadata(meta map[string]interface{}) map[string]interface{} {
	sensors := make(map[string]interface{})
	for _, key := range []string{"decoded_payload", "object"} {
		src, ok := meta[key].(map[string]interface{})
		if !ok {
			continue
		}
		for _, field := range []string{
			"version", "device_type", "device_name", "battery_voltage",
			"current_ma", "current_raw_ma", "multiplier",
			"report_type", "report_mode",
			"config_status", "config_type",
			"min_time_s", "max_time_s", "current_change_ma",
			"warning",
		} {
			if _, exists := sensors[field]; !exists {
				if v, ok := src[field]; ok {
					sensors[field] = v
				}
			}
		}
	}
	return sensors
}

// twoHexDigits formats a byte as 2-digit uppercase hex
func twoHexDigits(b byte) string {
	const hexDigits = "0123456789ABCDEF"
	return string(hexDigits[(b>>4)&0x0F]) + string(hexDigits[b&0x0F])
}
