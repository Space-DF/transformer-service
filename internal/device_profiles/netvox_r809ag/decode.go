package netvox_r809ag

import (
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

// Netvox R809AG fPort 6 uplink report types.
const (
	reportTypeStatus  = 0x01 // Status report: OnOff, Energy (Wh), Alarms
	reportTypeMeasure = 0x02 // Measurement report: Voltage (V), Current (mA), Power (W)
)

// Decode parses the R809AG binary uplink (fPort 6) into sensor readings.
//
// Payload layout (11 bytes, fPort 6):
//
//	Byte 0:   Protocol version (0x01)
//	Byte 1:   Device type
//	Byte 2:   Report type (0x00 / 0x01 / 0x02)
//
// Report type 0x01 (status):
//
//	Byte 3:   OnOff   (0x00 = OFF, non-zero = ON)
//	Bytes 4–7: Energy  (uint32 big-endian, Wh)
//	Byte 8:   OverCurrentAlarm (0x00 = no alarm)
//	Byte 9:   DashCurrentAlarm (0x00 = no alarm)
//	Byte 10:  PowerOffAlarm    (0x00 = no alarm)
//
// Report type 0x02 (measurement):
//
//	Bytes 3–4: Voltage (uint16 big-endian, V)
//	Bytes 5–6: Current (uint16 big-endian, mA)
//	Bytes 7–8: Power   (uint16 big-endian, W)
func Decode(payload *common.RawPayload) map[string]interface{} {
	sensors := make(map[string]interface{})
	if payload.FPort != 6 {
		return sensors
	}

	b := common.ExtractBytes(payload)
	if len(b) < 3 {
		return sensors
	}

	switch b[2] {
	case reportTypeStatus:
		if len(b) < 11 {
			break
		}
		if b[3] == 0x00 {
			sensors["switch"] = "off"
		} else {
			sensors["switch"] = "on"
		}
		sensors["energy"] = float64(common.U32BE(b, 4))
		sensors["overcurrent_alarm"] = b[8] != 0x00
		sensors["dash_current_alarm"] = b[9] != 0x00
		sensors["power_off_alarm"] = b[10] != 0x00

	case reportTypeMeasure:
		if len(b) < 9 {
			break
		}
		sensors["voltage"] = float64(common.U16BE(b, 3))
		sensors["current"] = float64(common.U16BE(b, 5))
		sensors["power"] = float64(common.U16BE(b, 7))
	}

	return sensors
}
