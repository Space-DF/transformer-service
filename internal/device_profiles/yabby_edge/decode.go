package yabby_edge

import (
	"fmt"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

// Decode extracts sensor readings and location from a Yabby Edge uplink.
// Auto-detects message type via heuristics (fPort not available at decode time).
func Decode(payload *common.RawPayload) (map[string]interface{}, *common.Location) {
	sensors := make(map[string]interface{})
	var location *common.Location

	// Parse the raw binary payload.
	b := common.ExtractBytes(payload)
	if len(b) == 0 {
		return sensors, location
	}

	switch detectPort(b) {
	case 1:
		parseHello(b, sensors)
	case 2:
		parseAck(b, sensors)
	case 3:
		parseStats(b, sensors)
	case 5:
		parseLocation(b, sensors, &location)
	case 89:
		parseConnect(b, sensors)
	}

	return sensors, location
}

type bitParser struct {
	bytes  []byte
	offset int
	length int
}

func newBitParser(b []byte) *bitParser {
	return &bitParser{bytes: b, offset: 0, length: len(b) * 8}
}

func (bp *bitParser) u32LE(n int) (uint32, bool) {
	if bp.offset+n > bp.length {
		return 0, false
	}
	var out uint32
	total := 0
	remaining := n
	for remaining > 0 {
		byteNum := bp.offset >> 3
		discardLSbs := bp.offset & 7
		avail := 8 - discardLSbs
		if avail > remaining {
			avail = remaining
		}
		if discardLSbs < 0 || discardLSbs > 7 || avail < 0 || avail > 8 {
			return 0, false
		}
		extracted := bp.bytes[byteNum] >> uint(discardLSbs)
		if avail < 0 || avail > 8 {
			return 0, false
		}
		mask := byte((1 << uint(avail)) - 1)
		out |= uint32(extracted&mask) << uint(total) // #nosec G115 -- safe, checked conversion
		total += avail
		remaining -= avail
		bp.offset += avail
	}
	return out, true
}

func (bp *bitParser) skip(n int) { bp.offset += n }

func isFwTupleLikely(fwMaj, fwMin, prodID, hwRev uint32) bool {
	return fwMaj <= 10 && fwMin <= 10 && prodID >= 80 && prodID <= 100 && hwRev <= 10
}

func isLikelyConnect(b []byte) bool {
	if len(b) < 11 || (b[6]&0xFC) != 0 {
		return false
	}
	return isFwTupleLikely(uint32(b[7]), uint32(b[8]), uint32(b[9]), uint32(b[10]))
}

func isLikelyHello(b []byte) bool {
	if len(b) < 10 || (b[4]&0xF0) != 0 {
		return false
	}
	return isFwTupleLikely(uint32(b[0]), uint32(b[1]), uint32(b[2]), uint32(b[3]))
}

func isLikelyAck(b []byte) bool {
	if len(b) < 9 {
		return false
	}
	return isFwTupleLikely(uint32(b[1]), uint32(b[2]), uint32(b[3]), uint32(b[4]))
}

func isLikelyStats(b []byte) bool {
	if len(b) != 11 {
		return false
	}
	bp := newBitParser(b)
	for _, bits := range []int{8, 8, 8, 14, 10, 10} {
		if _, ok := bp.u32LE(bits); !ok {
			return false
		}
	}
	var pctSum float64
	for i := 0; i < 5; i++ {
		v, ok := bp.u32LE(6)
		if !ok {
			return false
		}
		pctSum += 100.0 / 64.0 * float64(v)
	}
	return pctSum >= 90 && pctSum <= 110
}

func isLikelyLocation(b []byte) bool {
	if len(b) < 2 {
		return false
	}
	bp := newBitParser(b)
	wc, ok := bp.u32LE(5)
	if !ok {
		return false
	}
	bp.skip(2) // inTrip, inactive
	reserved, ok := bp.u32LE(3)
	if !ok || reserved != 0 {
		return false
	}
	bp.skip(6) // timeSet + posSeq
	return len(b) >= int(2+wc*7)
}

func detectPort(b []byte) int {
	switch {
	case isLikelyConnect(b):
		return 89
	case isLikelyHello(b):
		return 1
	case isLikelyAck(b):
		return 2
	case isLikelyStats(b):
		return 3
	case isLikelyLocation(b):
		return 5
	}
	return 0
}

func parseHello(b []byte, out map[string]interface{}) {
	bp := newBitParser(b)
	fwMaj, _ := bp.u32LE(8)
	fwMin, _ := bp.u32LE(8)
	bp.skip(8 + 8 + 4 + 4 + 16) // prodID, hwRev, reset flags, reserved, watchdog
	batRaw, _ := bp.u32LE(8)
	batV := common.YabbyBatV(batRaw)
	out["firmware"] = fmt.Sprintf("%d.%d", fwMaj, fwMin)
	out["battery_voltage"] = batV
	out["battery_level"] = common.LinearBatPct(batV, 2.0, 3.6)
}

func parseAck(b []byte, out map[string]interface{}) {
	bp := newBitParser(b)
	bp.skip(7) // seq
	accepted, _ := bp.u32LE(1)
	fwMaj, _ := bp.u32LE(8)
	fwMin, _ := bp.u32LE(8)
	out["downlink_ack"] = accepted != 0
	out["firmware"] = fmt.Sprintf("%d.%d", fwMaj, fwMin)
}

func parseStats(b []byte, out map[string]interface{}) {
	bp := newBitParser(b)
	initBatRaw, _ := bp.u32LE(8)
	bp.skip(8 + 8) // batMax, wakeupsPerTrip
	tripCountRaw, _ := bp.u32LE(14)
	bp.skip(10) // uptimeWeeks
	mahRaw, _ := bp.u32LE(10)
	batV := common.YabbyBatV(initBatRaw)
	out["battery_voltage"] = batV
	out["battery_level"] = common.LinearBatPct(batV, 2.0, 3.6)
	out["battery_stats"] = fmt.Sprintf("%d mAh used", mahRaw*2)
	out["trip_count"] = float64(tripCountRaw * 32)
}

func parseLocation(b []byte, out map[string]interface{}, loc **common.Location) {
	bp := newBitParser(b)
	wifiCount, _ := bp.u32LE(5)
	inTrip, _ := bp.u32LE(1)
	inactive, _ := bp.u32LE(1)
	bp.skip(3 + 1 + 5) // reserved, timeSet, posSeq

	out["trip_status"] = inTrip != 0
	out["inactivity"] = inactive != 0

	navStart := int(2 + wifiCount*7)
	if navStart >= len(b) {
		return
	}
	navLen := len(b) - navStart

	// Try LE 12-byte compact.
	if navLen >= 12 {
		lat := float64(common.I32LE(b, navStart)) / 1e7
		lon := float64(common.I32LE(b, navStart+4)) / 1e7
		if common.ValidateCoordinates(lat, lon) == nil {
			*loc = &common.Location{Latitude: lat, Longitude: lon}
			return
		}
	}
	// Try LE with 4-byte header.
	if navLen >= 16 {
		lat := float64(common.I32LE(b, navStart+4)) / 1e7
		lon := float64(common.I32LE(b, navStart+8)) / 1e7
		if common.ValidateCoordinates(lat, lon) == nil {
			*loc = &common.Location{Latitude: lat, Longitude: lon}
			return
		}
	}
	// BE fallback.
	if navLen >= 12 {
		lat := float64(common.I32BE(b, navStart)) / 1e7
		lon := float64(common.I32BE(b, navStart+4)) / 1e7
		if common.ValidateCoordinates(lat, lon) == nil {
			*loc = &common.Location{Latitude: lat, Longitude: lon}
		}
	}
}

func parseConnect(b []byte, out map[string]interface{}) {
	if len(b) < 11 {
		return
	}
	out["connect"] = "connected"
	out["firmware"] = fmt.Sprintf("%d.%d", b[7], b[8])
}
