package sen0313

import (
	"encoding/json"
	"strings"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

const (
	frameHeader byte = 0xff
	frameLen         = 4
)

// Decode extracts the latest valid SEN0313 UART distance frame.
//
// Frame format:
//
//	[0] 0xff header
//	[1] distance high byte
//	[2] distance low byte
//	[3] checksum = low 8 bits of header + high + low
func Decode(payload *common.RawPayload) map[string]interface{} {
	sensors := make(map[string]interface{})

	if distanceCM, ok := decodeJSONPayload(payload); ok {
		sensors["distance"] = distanceCM
		return sensors
	}

	b := common.ExtractBytes(payload)
	if len(b) < frameLen {
		return sensors
	}

	if distanceCM, ok := decodeJSONBytes(b); ok {
		sensors["distance"] = distanceCM
		return sensors
	}

	var latestDistanceMM int
	found := false
	for i := 0; i <= len(b)-frameLen; i++ {
		frame := b[i : i+frameLen]
		distanceMM, ok := decodeFrame(frame)
		if !ok {
			continue
		}
		latestDistanceMM = distanceMM
		found = true
	}

	if !found {
		return sensors
	}

	sensors["distance"] = float64(latestDistanceMM) / 10.0
	return sensors
}

func decodeJSONPayload(payload *common.RawPayload) (float64, bool) {
	if payload == nil {
		return 0, false
	}

	for _, key := range []string{"decoded_payload", "decoded_raw_data", "decoded_data"} {
		if decoded, ok := payload.Metadata[key].(map[string]interface{}); ok {
			if distanceCM, ok := extractDistanceCM(decoded); ok {
				return distanceCM, true
			}
		}
	}

	if decoded, ok := payload.Metadata["data"].(map[string]interface{}); ok {
		if distanceCM, ok := extractDistanceCM(decoded); ok {
			return distanceCM, true
		}
	}

	return decodeJSONString(payload.Data)
}

func decodeJSONBytes(data []byte) (float64, bool) {
	trimmed := strings.TrimSpace(string(data))
	if !strings.HasPrefix(trimmed, "{") {
		return 0, false
	}
	return decodeJSONString(trimmed)
}

func decodeJSONString(data string) (float64, bool) {
	if strings.TrimSpace(data) == "" {
		return 0, false
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(data), &decoded); err != nil {
		return 0, false
	}

	return extractDistanceCM(decoded)
}

func extractDistanceCM(decoded map[string]interface{}) (float64, bool) {
	if distance, ok := common.NumericValue(decoded["distance"]); ok {
		return distance, true
	}
	return 0, false
}

func decodeFrame(frame []byte) (int, bool) {
	if len(frame) != frameLen || frame[0] != frameHeader {
		return 0, false
	}

	sum := byte(uint16(frame[0]+frame[1]+frame[2]) & 0x00ff)
	if sum != frame[3] {
		return 0, false
	}

	return int(frame[1])<<8 | int(frame[2]), true
}
