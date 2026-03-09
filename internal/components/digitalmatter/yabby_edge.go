package digitalmatter

import (
	"encoding/hex"
	"fmt"
	"math"
	"time"

	"github.com/Space-DF/transformer-service/internal/components"
)

// YabbyEdgeParser handles parsing of Digital Matter Yabby Edge LoRaWAN payloads.
//
// The Yabby Edge is a compact battery-powered GPS/WiFi tracker that uses
// cloud-based location solving. It sends raw WiFi MAC scans and GNSS
// satellite timing data over LoRaWAN, which are resolved to positions
// by the Digital Matter Location Engine or a custom resolver.
//
// Uplink port mapping:
//
//	Port 1:       Hello message (firmware info, hardware info, reset flags)
//	Port 2:       Downlink ACK (acknowledgement of a parameter change)
//	Port 3:       Battery statistics (voltage, usage, trip count, uptime)
//	Port 5:       Location message (WiFi MACs + optional GNSS nav data)
//	Port 89:      Connect message (device ID, firmware info)
//	Port 90:      MTU advice
//	Port 91:      Almanac cookie request
//	Port 92:      Almanac chunk request
//	Port 101-116: Fragmented uplinks (reassembled by Location Engine)
//	Port 202:     Time request
//
// Battery voltage is encoded as: voltage = 2.0 + 0.007 * raw_value (V)
// All multi-byte fields use LITTLE-ENDIAN bit packing.
type YabbyEdgeParser struct{}

// NewYabbyEdgeParser creates a new Yabby Edge parser.
func NewYabbyEdgeParser() *YabbyEdgeParser {
	return &YabbyEdgeParser{}
}

// bitParser reads individual bits from a byte slice in little-endian bit order.
// This matches the Digital Matter payload specification where fields are packed
// at the bit level with LSB-first ordering within each byte.
type bitParser struct {
	bits   []byte
	offset int // current bit offset
	length int // total bits available
}

func newBitParser(data []byte) *bitParser {
	return &bitParser{
		bits:   data,
		offset: 0,
		length: len(data) * 8,
	}
}

// u32LE reads `n` bits as an unsigned 32-bit LE value (max 32 bits).
// Bits are extracted LSB-first from each byte.
func (bp *bitParser) u32LE(n int) (uint32, error) {
	if n > 32 {
		return 0, fmt.Errorf("cannot read more than 32 bits, requested %d", n)
	}
	if bp.offset+n > bp.length {
		return 0, fmt.Errorf("read past end of data: offset=%d, bits=%d, length=%d", bp.offset, n, bp.length)
	}

	var out uint32
	total := 0
	remaining := n

	for remaining > 0 {
		byteNum := bp.offset / 8
		discardLSbs := bp.offset & 7
		avail := 8 - discardLSbs
		if avail > remaining {
			avail = remaining
		}
		extracted := bp.bits[byteNum] >> uint(discardLSbs)
		mask := uint32((1 << uint(avail)) - 1)
		masked := uint32(extracted) & mask
		out |= masked << uint(total)

		total += avail
		remaining -= avail
		bp.offset += avail
	}

	return out, nil
}

// skip advances the bit offset by n bits.
func (bp *bitParser) skip(n int) error {
	if bp.offset+n > bp.length {
		return fmt.Errorf("skip past end of data")
	}
	bp.offset += n
	return nil
}

// YabbyEdgePayload represents the parsed Yabby Edge uplink data.
type YabbyEdgePayload struct {
	MessageType string `json:"type"`
	FPort       int    `json:"fport"`

	// Hello (port 1) / Connect (port 89)
	FwMaj          uint32  `json:"fw_maj,omitempty"`
	FwMin          uint32  `json:"fw_min,omitempty"`
	ProdID         uint32  `json:"prod_id,omitempty"`
	HwRev          uint32  `json:"hw_rev,omitempty"`
	LrHw           uint32  `json:"lr_hw,omitempty"`
	LrMaj          uint32  `json:"lr_maj,omitempty"`
	LrMin          uint32  `json:"lr_min,omitempty"`
	ResetPowerOn   bool    `json:"reset_power_on,omitempty"`
	ResetWatchdog  bool    `json:"reset_watchdog,omitempty"`
	ResetExternal  bool    `json:"reset_external,omitempty"`
	ResetSoftware  bool    `json:"reset_software,omitempty"`
	WatchdogReason uint32  `json:"watchdog_reason,omitempty"`
	InitialBatV    float64 `json:"initial_bat_v,omitempty"`

	// Downlink ACK (port 2)
	Sequence uint32 `json:"sequence,omitempty"`
	Accepted bool   `json:"accepted,omitempty"`
	AckPort  uint32 `json:"ack_port,omitempty"`

	// Stats (port 3)
	BatVMax        float64 `json:"bat_v_max,omitempty"`
	WakeupsPerTrip uint32  `json:"wakeups_per_trip,omitempty"`
	TripCount      uint32  `json:"trip_count,omitempty"`
	UptimeWeeks    uint32  `json:"uptime_weeks,omitempty"`
	MAhUsed        uint32  `json:"mah_used,omitempty"`
	PercentLora    float64 `json:"percent_lora,omitempty"`
	PercentGnss    float64 `json:"percent_gnss,omitempty"`
	PercentWifi    float64 `json:"percent_wifi,omitempty"`
	PercentSleep   float64 `json:"percent_sleep,omitempty"`
	PercentDisch   float64 `json:"percent_disch,omitempty"`
	PercentOther   float64 `json:"percent_other,omitempty"`

	// Location (port 5)
	InTrip   bool       `json:"in_trip,omitempty"`
	Inactive bool       `json:"inactive,omitempty"`
	TimeSet  bool       `json:"time_set,omitempty"`
	PosSeq   uint32     `json:"pos_seq,omitempty"`
	WiFi     []WiFiScan `json:"wifi,omitempty"`
	NavData  string     `json:"nav,omitempty"` // Raw GNSS nav hex data

	// Connect (port 89)
	ConnectID string `json:"connect_id,omitempty"`
	DevReset  bool   `json:"dev_reset,omitempty"`
	FcntReset bool   `json:"fcnt_reset,omitempty"`

	// Fragment (ports 101-116)
	FrameID    uint32 `json:"frame_id,omitempty"`
	FragTotal  uint32 `json:"frag_total,omitempty"`
	FragOffset uint32 `json:"frag_offset,omitempty"`
	FragData   string `json:"frag_data,omitempty"`
	FragPort   uint32 `json:"frag_port,omitempty"`
	FragSize   uint32 `json:"frag_size,omitempty"`

	// Resolved location (from Location Engine or custom resolver)
	Latitude  float64 `json:"latitude,omitempty"`
	Longitude float64 `json:"longitude,omitempty"`
	Altitude  float64 `json:"altitude,omitempty"`
	Accuracy  float64 `json:"accuracy,omitempty"`

	// Battery voltage (computed from raw)
	BatteryVoltage float64 `json:"battery_voltage,omitempty"`
}

// WiFiScan represents a single WiFi access point scan result.
type WiFiScan struct {
	RSSI int    `json:"rssi"` // Signal strength in dBm
	MAC  string `json:"mac"`  // MAC address as hex string (12 chars)
}

// decodeBatteryVoltage converts the raw 8-bit battery value to voltage.
// Formula from DM spec: V = 2.0 + 0.007 * raw
func decodeBatteryVoltage(raw uint32) float64 {
	return math.Round((2.0+0.007*float64(raw))*1000) / 1000
}

// printHex converts a byte slice segment to a hex string.
func printHex(data []byte, offset, length int) string {
	if offset+length > len(data) {
		length = len(data) - offset
	}
	if length <= 0 {
		return ""
	}
	return hex.EncodeToString(data[offset : offset+length])
}

// parseHelloPort1 decodes port 1 "hello" message.
// Format (little-endian bit-packed):
//
//	fwMaj(8) + fwMin(8) + prodId(8) + hwRev(8)
//	+ resetPowerOn(1) + resetWatchdog(1) + resetExternal(1) + resetSoftware(1) + reserved(4)
//	+ watchdogReason(16) + initialBatV(8) + lrHw(8) + lrMaj(8) + lrMin(8)
func parseHelloPort1(data []byte) (*YabbyEdgePayload, error) {
	bp := newBitParser(data)
	p := &YabbyEdgePayload{MessageType: "hello", FPort: 1}
	var err error

	if p.FwMaj, err = bp.u32LE(8); err != nil {
		return nil, fmt.Errorf("hello: fwMaj: %w", err)
	}
	if p.FwMin, err = bp.u32LE(8); err != nil {
		return nil, fmt.Errorf("hello: fwMin: %w", err)
	}
	if p.ProdID, err = bp.u32LE(8); err != nil {
		return nil, fmt.Errorf("hello: prodId: %w", err)
	}
	if p.HwRev, err = bp.u32LE(8); err != nil {
		return nil, fmt.Errorf("hello: hwRev: %w", err)
	}

	var v uint32
	if v, err = bp.u32LE(1); err != nil {
		return nil, fmt.Errorf("hello: resetPowerOn: %w", err)
	}
	p.ResetPowerOn = v != 0

	if v, err = bp.u32LE(1); err != nil {
		return nil, fmt.Errorf("hello: resetWatchdog: %w", err)
	}
	p.ResetWatchdog = v != 0

	if v, err = bp.u32LE(1); err != nil {
		return nil, fmt.Errorf("hello: resetExternal: %w", err)
	}
	p.ResetExternal = v != 0

	if v, err = bp.u32LE(1); err != nil {
		return nil, fmt.Errorf("hello: resetSoftware: %w", err)
	}
	p.ResetSoftware = v != 0

	if err = bp.skip(4); err != nil {
		return nil, fmt.Errorf("hello: reserved bits: %w", err)
	}

	if p.WatchdogReason, err = bp.u32LE(16); err != nil {
		return nil, fmt.Errorf("hello: watchdogReason: %w", err)
	}

	if v, err = bp.u32LE(8); err != nil {
		return nil, fmt.Errorf("hello: initialBatV: %w", err)
	}
	p.InitialBatV = decodeBatteryVoltage(v)
	p.BatteryVoltage = p.InitialBatV

	if p.LrHw, err = bp.u32LE(8); err != nil {
		return nil, fmt.Errorf("hello: lrHw: %w", err)
	}
	if p.LrMaj, err = bp.u32LE(8); err != nil {
		return nil, fmt.Errorf("hello: lrMaj: %w", err)
	}
	if p.LrMin, err = bp.u32LE(8); err != nil {
		return nil, fmt.Errorf("hello: lrMin: %w", err)
	}

	return p, nil
}

// parseDownlinkAckPort2 decodes port 2 "downlink ack" message.
// Format: sequence(7) + accepted(1) + fwMaj(8) + fwMin(8) + prodId(8) + hwRev(8)
//
//   - port(8) + lrHw(8) + lrMaj(8) + lrMin(8)
func parseDownlinkAckPort2(data []byte) (*YabbyEdgePayload, error) {
	bp := newBitParser(data)
	p := &YabbyEdgePayload{MessageType: "downlink_ack", FPort: 2}
	var err error

	if p.Sequence, err = bp.u32LE(7); err != nil {
		return nil, fmt.Errorf("ack: sequence: %w", err)
	}

	var v uint32
	if v, err = bp.u32LE(1); err != nil {
		return nil, fmt.Errorf("ack: accepted: %w", err)
	}
	p.Accepted = v != 0

	if p.FwMaj, err = bp.u32LE(8); err != nil {
		return nil, fmt.Errorf("ack: fwMaj: %w", err)
	}
	if p.FwMin, err = bp.u32LE(8); err != nil {
		return nil, fmt.Errorf("ack: fwMin: %w", err)
	}
	if p.ProdID, err = bp.u32LE(8); err != nil {
		return nil, fmt.Errorf("ack: prodId: %w", err)
	}
	if p.HwRev, err = bp.u32LE(8); err != nil {
		return nil, fmt.Errorf("ack: hwRev: %w", err)
	}
	if p.AckPort, err = bp.u32LE(8); err != nil {
		return nil, fmt.Errorf("ack: port: %w", err)
	}
	if p.LrHw, err = bp.u32LE(8); err != nil {
		return nil, fmt.Errorf("ack: lrHw: %w", err)
	}
	if p.LrMaj, err = bp.u32LE(8); err != nil {
		return nil, fmt.Errorf("ack: lrMaj: %w", err)
	}
	if p.LrMin, err = bp.u32LE(8); err != nil {
		return nil, fmt.Errorf("ack: lrMin: %w", err)
	}

	return p, nil
}

// parseStatsPort3 decodes port 3 "stats" message.
// Format: initialBatV(8) + batVMax(8) + wakeupsPerTrip(8) + tripCount(14)
//
//   - uptimeWeeks(10) + mAhUsed(10) + percentLora(6) + percentGnss(6)
//   - percentWifi(6) + percentSleep(6) + percentDisch(6)
func parseStatsPort3(data []byte) (*YabbyEdgePayload, error) {
	bp := newBitParser(data)
	p := &YabbyEdgePayload{MessageType: "stats", FPort: 3}
	var err error
	var v uint32

	if v, err = bp.u32LE(8); err != nil {
		return nil, fmt.Errorf("stats: initialBatV: %w", err)
	}
	p.InitialBatV = decodeBatteryVoltage(v)
	p.BatteryVoltage = p.InitialBatV

	if v, err = bp.u32LE(8); err != nil {
		return nil, fmt.Errorf("stats: batVMax: %w", err)
	}
	p.BatVMax = decodeBatteryVoltage(v)

	if p.WakeupsPerTrip, err = bp.u32LE(8); err != nil {
		return nil, fmt.Errorf("stats: wakeupsPerTrip: %w", err)
	}
	if v, err = bp.u32LE(14); err != nil {
		return nil, fmt.Errorf("stats: tripCount: %w", err)
	}
	p.TripCount = 32 * v

	if p.UptimeWeeks, err = bp.u32LE(10); err != nil {
		return nil, fmt.Errorf("stats: uptimeWeeks: %w", err)
	}
	if v, err = bp.u32LE(10); err != nil {
		return nil, fmt.Errorf("stats: mAhUsed: %w", err)
	}
	p.MAhUsed = 2 * v

	if v, err = bp.u32LE(6); err != nil {
		return nil, fmt.Errorf("stats: percentLora: %w", err)
	}
	p.PercentLora = 100.0 / 64.0 * float64(v)

	if v, err = bp.u32LE(6); err != nil {
		return nil, fmt.Errorf("stats: percentGnss: %w", err)
	}
	p.PercentGnss = 100.0 / 64.0 * float64(v)

	if v, err = bp.u32LE(6); err != nil {
		return nil, fmt.Errorf("stats: percentWifi: %w", err)
	}
	p.PercentWifi = 100.0 / 64.0 * float64(v)

	if v, err = bp.u32LE(6); err != nil {
		return nil, fmt.Errorf("stats: percentSleep: %w", err)
	}
	p.PercentSleep = 100.0 / 64.0 * float64(v)

	if v, err = bp.u32LE(6); err != nil {
		return nil, fmt.Errorf("stats: percentDisch: %w", err)
	}
	p.PercentDisch = 100.0 / 64.0 * float64(v)

	p.PercentOther = 100.0 - p.PercentLora - p.PercentGnss -
		p.PercentWifi - p.PercentSleep - p.PercentDisch

	return p, nil
}

// parseLocationPort5 decodes port 5 "location" message.
//
// This is the primary position uplink. It contains WiFi MAC scans and
// optional GNSS navigation data. The WiFi MACs and GNSS data are
// typically resolved to a position by the DM Location Engine.
//
// Format: wifiCount(5) + inTrip(1) + inactive(1) + reserved(3) + timeSet(1) + posSeq(5)
//
//   - [wifiCount × (rssi(8) + mac(48))]
//   - [remaining bytes: GNSS nav data]
func parseLocationPort5(data []byte) (*YabbyEdgePayload, error) {
	bp := newBitParser(data)
	p := &YabbyEdgePayload{MessageType: "location", FPort: 5}
	var err error
	var v uint32

	// WiFi count (5 bits)
	var wifiCount uint32
	if wifiCount, err = bp.u32LE(5); err != nil {
		return nil, fmt.Errorf("location: wifiCount: %w", err)
	}

	// inTrip (1 bit)
	if v, err = bp.u32LE(1); err != nil {
		return nil, fmt.Errorf("location: inTrip: %w", err)
	}
	p.InTrip = v != 0

	// inactive (1 bit)
	if v, err = bp.u32LE(1); err != nil {
		return nil, fmt.Errorf("location: inactive: %w", err)
	}
	p.Inactive = v != 0

	// reserved (3 bits)
	if err = bp.skip(3); err != nil {
		return nil, fmt.Errorf("location: reserved: %w", err)
	}

	// timeSet (1 bit)
	if v, err = bp.u32LE(1); err != nil {
		return nil, fmt.Errorf("location: timeSet: %w", err)
	}
	p.TimeSet = v != 0

	// posSeq (5 bits)
	if p.PosSeq, err = bp.u32LE(5); err != nil {
		return nil, fmt.Errorf("location: posSeq: %w", err)
	}

	// Parse WiFi MAC entries.
	// Each entry is: RSSI (1 byte, signed) + MAC address (6 bytes) = 7 bytes.
	// The WiFi data starts at byte offset 2 in the raw data (after the 16-bit header).
	p.WiFi = make([]WiFiScan, 0, wifiCount)
	for i := uint32(0); i < wifiCount; i++ {
		entryOffset := 2 + int(i)*7
		if entryOffset+7 > len(data) {
			break
		}
		rssi := int(int8(data[entryOffset])) //#nosec G115
		mac := printHex(data, entryOffset+1, 6)
		p.WiFi = append(p.WiFi, WiFiScan{
			RSSI: rssi,
			MAC:  mac,
		})
	}

	// Remaining bytes after WiFi entries are GNSS navigation data
	navStart := 2 + int(wifiCount)*7
	if navStart < len(data) {
		navBytes := data[navStart:]
		if len(navBytes) > 0 {
			p.NavData = hex.EncodeToString(navBytes)
		}
	}

	return p, nil
}

// parseConnectPort89 decodes port 89 "connect" message.
// Format: id(48) + flags(8) + fwMaj(8) + fwMin(8) + prodId(8) + hwRev(8)
func parseConnectPort89(data []byte) (*YabbyEdgePayload, error) {
	if len(data) < 11 {
		return nil, fmt.Errorf("connect: payload too short (%d bytes)", len(data))
	}
	p := &YabbyEdgePayload{MessageType: "connect", FPort: 89}
	p.ConnectID = printHex(data, 0, 6)
	p.DevReset = (data[6] & 1) != 0
	p.FcntReset = (data[6] & 2) != 0
	p.FwMaj = uint32(data[7])
	p.FwMin = uint32(data[8])
	p.ProdID = uint32(data[9])
	p.HwRev = uint32(data[10])

	return p, nil
}

// parseFragmentPort101_116 decodes port 101-116 "fragment" message.
// These are parts of a larger message reassembled by the Location Engine.
// Format: frameId_low(4) + total(5) + offset(7) + [data bytes]
// If offset == 0: port(8) + size_remainder(4) at start of data
func parseFragmentPort101_116(fPort int, data []byte) (*YabbyEdgePayload, error) {
	bp := newBitParser(data)
	p := &YabbyEdgePayload{MessageType: "fragments", FPort: fPort}
	var err error
	var v uint32

	// frameId low 4 bits (combined with port offset for full ID)
	if v, err = bp.u32LE(4); err != nil {
		return nil, fmt.Errorf("fragment: frameId: %w", err)
	}
	p.FrameID = uint32(fPort) - 101 + 16*v

	// total fragment count (5 bits) + 1
	if v, err = bp.u32LE(5); err != nil {
		return nil, fmt.Errorf("fragment: total: %w", err)
	}
	p.FragTotal = v + 1

	// offset (7 bits)
	if p.FragOffset, err = bp.u32LE(7); err != nil {
		return nil, fmt.Errorf("fragment: offset: %w", err)
	}

	// Fragment data starts at byte 2
	if len(data) > 2 {
		p.FragData = printHex(data, 2, len(data)-2)
	}

	// If this is the first fragment (offset == 0), decode port and size
	if p.FragOffset == 0 && len(data) > 2 {
		p.FragPort = uint32(data[2]) // the reassembled port (e.g., 5)
		// size = total * 9 - low4bits (remaining)
		var sizeBits uint32
		bp2 := newBitParser(data)
		_ = bp2.skip(16 + 8) // skip header (16) + port byte (8)
		if sizeBits, err = bp2.u32LE(4); err == nil {
			p.FragSize = p.FragTotal*9 - sizeBits
		}
	}

	return p, nil
}

// parseYabbyEdgeUplink decodes a Yabby Edge uplink based on fPort and raw bytes.
func parseYabbyEdgeUplink(fPort int, data []byte) (*YabbyEdgePayload, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty payload")
	}

	switch {
	case fPort == 1:
		return parseHelloPort1(data)
	case fPort == 2:
		return parseDownlinkAckPort2(data)
	case fPort == 3:
		return parseStatsPort3(data)
	case fPort == 5:
		return parseLocationPort5(data)
	case fPort == 89:
		return parseConnectPort89(data)
	case fPort >= 101 && fPort <= 116:
		return parseFragmentPort101_116(fPort, data)
	default:
		// For other ports (90, 91, 92, 202), return a generic payload
		return &YabbyEdgePayload{
			MessageType: fmt.Sprintf("port_%d", fPort),
			FPort:       fPort,
		}, nil
	}
}

// extractResolvedLocation attempts to extract a resolved location from the metadata
func extractResolvedLocation(metadata map[string]interface{}) (lat, lon, alt, acc float64, ok bool) {
	// Try direct fields
	latVal, latOk := getFloat64(metadata, "latitude", "lat")
	lonVal, lonOk := getFloat64(metadata, "longitude", "lon", "lng")
	if latOk && lonOk && (latVal != 0 || lonVal != 0) {
		altVal, _ := getFloat64(metadata, "altitude", "alt")
		accVal, _ := getFloat64(metadata, "accuracy", "acc", "hdop")
		return latVal, lonVal, altVal, accVal, true
	}

	// Try nested location object
	for _, key := range []string{"location", "position", "gps", "resolved_location"} {
		if locObj, ok := metadata[key].(map[string]interface{}); ok {
			latVal, latOk = getFloat64(locObj, "latitude", "lat")
			lonVal, lonOk = getFloat64(locObj, "longitude", "lon", "lng")
			if latOk && lonOk && (latVal != 0 || lonVal != 0) {
				altVal, _ := getFloat64(locObj, "altitude", "alt")
				accVal, _ := getFloat64(locObj, "accuracy", "acc")
				return latVal, lonVal, altVal, accVal, true
			}
		}
	}

	// Try DM Location Engine format: decoded object with lat/lon
	if decoded, ok := metadata["decoded"].(map[string]interface{}); ok {
		latVal, latOk = getFloat64(decoded, "latitude", "lat")
		lonVal, lonOk = getFloat64(decoded, "longitude", "lon", "lng")
		if latOk && lonOk && (latVal != 0 || lonVal != 0) {
			altVal, _ := getFloat64(decoded, "altitude", "alt")
			accVal, _ := getFloat64(decoded, "accuracy", "acc")
			return latVal, lonVal, altVal, accVal, true
		}
	}

	return 0, 0, 0, 0, false
}

// getFloat64 tries multiple keys in a map and returns the first float64 found.
func getFloat64(m map[string]interface{}, keys ...string) (float64, bool) {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch val := v.(type) {
			case float64:
				return val, true
			case int:
				return float64(val), true
			case int64:
				return float64(val), true
			}
		}
	}
	return 0, false
}

// ParsePayload parses Yabby Edge device payload.
func (p *YabbyEdgeParser) ParsePayload(payload *components.RawPayload) (*components.ParsedData, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = components.ExtractDevEUI(payload.Metadata)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI not found")
	}

	// Extract fPort
	fPort := payload.FPort
	if fPort == 0 {
		fPort = components.ExtractFPort(payload.Metadata)
	}
	if fPort == 0 {
		return nil, fmt.Errorf("fPort not found in payload")
	}

	// Extract and decode payload bytes
	encoded := components.ExtractPayloadData(payload.Data)
	if encoded == "" {
		encoded = components.ExtractPayloadData(payload.Metadata)
	}
	if encoded == "" {
		return nil, fmt.Errorf("no payload data found")
	}

	rawBytes, err := components.DecodePayloadBytes(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	// Parse the uplink
	yabbyData, err := parseYabbyEdgeUplink(fPort, rawBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Yabby Edge uplink: %w", err)
	}

	sensorData := make(map[string]interface{})
	var location *components.Location

	sensorData["message_type"] = yabbyData.MessageType
	sensorData["fport"] = fPort

	switch fPort {
	case 1: // Hello
		sensorData["firmware_version"] = fmt.Sprintf("%d.%d", yabbyData.FwMaj, yabbyData.FwMin)
		sensorData["product_id"] = yabbyData.ProdID
		sensorData["hardware_revision"] = yabbyData.HwRev
		sensorData["battery_voltage"] = yabbyData.BatteryVoltage
		sensorData["lorawan_version"] = fmt.Sprintf("%d.%d", yabbyData.LrMaj, yabbyData.LrMin)
		sensorData["reset_power_on"] = yabbyData.ResetPowerOn
		sensorData["reset_watchdog"] = yabbyData.ResetWatchdog
		sensorData["reset_external"] = yabbyData.ResetExternal
		sensorData["reset_software"] = yabbyData.ResetSoftware
	case 3: // Stats
		sensorData["battery_voltage"] = yabbyData.InitialBatV
		sensorData["battery_voltage_max"] = yabbyData.BatVMax
		sensorData["wakeups_per_trip"] = yabbyData.WakeupsPerTrip
		sensorData["trip_count"] = yabbyData.TripCount
		sensorData["uptime_weeks"] = yabbyData.UptimeWeeks
		sensorData["mah_used"] = yabbyData.MAhUsed
		sensorData["percent_lora"] = yabbyData.PercentLora
		sensorData["percent_gnss"] = yabbyData.PercentGnss
		sensorData["percent_wifi"] = yabbyData.PercentWifi
		sensorData["percent_sleep"] = yabbyData.PercentSleep
	case 5: // Location
		sensorData["in_trip"] = yabbyData.InTrip
		sensorData["inactive"] = yabbyData.Inactive
		sensorData["time_set"] = yabbyData.TimeSet
		sensorData["position_sequence"] = yabbyData.PosSeq
		if len(yabbyData.WiFi) > 0 {
			sensorData["wifi_scan_count"] = len(yabbyData.WiFi)
			sensorData["wifi_scans"] = yabbyData.WiFi
		}
		if yabbyData.NavData != "" {
			sensorData["gnss_nav_data"] = yabbyData.NavData
		}

		// Try to extract resolved location from metadata
		if lat, lon, alt, acc, ok := extractResolvedLocation(payload.Metadata); ok {
			location = &components.Location{
				Latitude:  lat,
				Longitude: lon,
				Altitude:  alt,
			}
			sensorData["latitude"] = lat
			sensorData["longitude"] = lon
			sensorData["altitude"] = alt
			sensorData["accuracy"] = acc
			sensorData["location_source"] = "location_engine"
		}
	}

	var batteryLevel *float64
	if yabbyData.BatteryVoltage > 0 {
		// Estimate battery percentage from voltage (LiFeS2 AAA: 1.8V empty, 3.6V full, 2 cells)
		pct := estimateBatteryPercent(yabbyData.BatteryVoltage)
		batteryLevel = &pct
	}

	return &components.ParsedData{
		DeviceEUI:    devEUI,
		DeviceType:   DeviceTypeYabbyEdge,
		Timestamp:    payload.Timestamp,
		Location:     location,
		SensorData:   sensorData,
		BatteryLevel: batteryLevel,
		RawData:      encoded,
	}, nil
}

// SupportsGPS returns true since Yabby Edge has GNSS capability (cloud-resolved).
func (p *YabbyEdgeParser) SupportsGPS() bool {
	return true
}

// GetSupportedPorts returns the fPorts typically used by Yabby Edge.
func (p *YabbyEdgeParser) GetSupportedPorts() []int {
	return []int{1, 2, 3, 5, 89, 90, 91, 92, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 202}
}

// GetSupportedEntityTypes returns entity types supported by Yabby Edge.
func (p *YabbyEdgeParser) GetSupportedEntityTypes() []string {
	return []string{
		"location",
		"battery",
		"trip_status",
		"firmware_info",
		"battery_stats",
		"inactivity",
	}
}

// ParseToEntities creates entities for Yabby Edge device.
func (p *YabbyEdgeParser) ParseToEntities(orgSlug, model string, payload *components.RawPayload, deviceLocation *components.Location) ([]components.Entity, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = components.ExtractDevEUI(payload.Metadata)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI is required")
	}

	// Extract fPort
	fPort := payload.FPort
	if fPort == 0 {
		fPort = components.ExtractFPort(payload.Metadata)
	}
	if fPort == 0 {
		return nil, fmt.Errorf("fPort not found in payload")
	}

	// Extract and decode payload bytes
	encoded := components.ExtractPayloadData(payload.Data)
	if encoded == "" {
		encoded = components.ExtractPayloadData(payload.Metadata)
	}
	if encoded == "" {
		return nil, fmt.Errorf("no payload data found")
	}

	rawBytes, err := components.DecodePayloadBytes(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	// Parse the uplink
	yabbyData, err := parseYabbyEdgeUplink(fPort, rawBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Yabby Edge uplink: %w", err)
	}

	var entities []components.Entity
	timestamp := payload.Timestamp
	modelID := "yabby_edge"

	switch fPort {
	case 1: // Hello message
		entities = append(entities, components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "firmware"),
			EntityID: components.GenerateEntityID(
				"sensor",
				orgSlug, "digitalmatter", modelID, devEUI, "firmware",
			),
			EntityType:  "sensor",
			DeviceClass: "firmware",
			Name:        "Firmware Version",
			State:       fmt.Sprintf("%d.%d", yabbyData.FwMaj, yabbyData.FwMin),
			Attributes: map[string]interface{}{
				"device_model":      "Yabby Edge LoRaWAN",
				"product_id":        yabbyData.ProdID,
				"hardware_revision": yabbyData.HwRev,
				"lorawan_hw":        yabbyData.LrHw,
				"lorawan_version":   fmt.Sprintf("%d.%d", yabbyData.LrMaj, yabbyData.LrMin),
				"reset_power_on":    yabbyData.ResetPowerOn,
				"reset_watchdog":    yabbyData.ResetWatchdog,
				"reset_external":    yabbyData.ResetExternal,
				"reset_software":    yabbyData.ResetSoftware,
			},
			Enabled:   true,
			Timestamp: timestamp,
		})

		// Battery entity from hello message
		if yabbyData.BatteryVoltage > 0 {
			entities = append(entities, p.createBatteryVoltageEntity(orgSlug, model, modelID, devEUI, yabbyData.BatteryVoltage, timestamp))
			entities = append(entities, p.createBatteryPercentEntity(orgSlug, model, modelID, devEUI, yabbyData.BatteryVoltage, timestamp))
		}

	case 2: // Downlink ACK
		entities = append(entities, components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "downlink_ack"),
			EntityID: components.GenerateEntityID(
				"sensor",
				orgSlug, "digitalmatter", modelID, devEUI, "downlink_ack",
			),
			EntityType:  "sensor",
			DeviceClass: "status",
			Name:        "Downlink ACK",
			State:       yabbyData.Accepted,
			Attributes: map[string]interface{}{
				"device_model":     "Yabby Edge LoRaWAN",
				"sequence":         yabbyData.Sequence,
				"accepted":         yabbyData.Accepted,
				"ack_port":         yabbyData.AckPort,
				"firmware_version": fmt.Sprintf("%d.%d", yabbyData.FwMaj, yabbyData.FwMin),
			},
			Enabled:   true,
			Timestamp: timestamp,
		})

	case 3: // Battery stats
		// Battery voltage entity
		if yabbyData.InitialBatV > 0 {
			entities = append(entities, p.createBatteryVoltageEntity(orgSlug, model, modelID, devEUI, yabbyData.InitialBatV, timestamp))
			entities = append(entities, p.createBatteryPercentEntity(orgSlug, model, modelID, devEUI, yabbyData.InitialBatV, timestamp))
		}

		// Battery stats entity
		entities = append(entities, components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "battery_stats"),
			EntityID: components.GenerateEntityID(
				"sensor",
				orgSlug, "digitalmatter", modelID, devEUI, "battery_stats",
			),
			EntityType:  "sensor",
			DeviceClass: "battery_stats",
			Name:        "Battery Statistics",
			State:       fmt.Sprintf("%.0f mAh used", float64(yabbyData.MAhUsed)),
			Attributes: map[string]interface{}{
				"device_model":      "Yabby Edge LoRaWAN",
				"initial_voltage":   yabbyData.InitialBatV,
				"max_voltage":       yabbyData.BatVMax,
				"wakeups_per_trip":  yabbyData.WakeupsPerTrip,
				"trip_count":        yabbyData.TripCount,
				"uptime_weeks":      yabbyData.UptimeWeeks,
				"mah_used":          yabbyData.MAhUsed,
				"percent_lora":      math.Round(yabbyData.PercentLora*10) / 10,
				"percent_gnss":      math.Round(yabbyData.PercentGnss*10) / 10,
				"percent_wifi":      math.Round(yabbyData.PercentWifi*10) / 10,
				"percent_sleep":     math.Round(yabbyData.PercentSleep*10) / 10,
				"percent_discharge": math.Round(yabbyData.PercentDisch*10) / 10,
				"percent_other":     math.Round(yabbyData.PercentOther*10) / 10,
			},
			Enabled:   true,
			Timestamp: timestamp,
		})

		// Trip count entity
		entities = append(entities, components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "trip_count"),
			EntityID: components.GenerateEntityID(
				"sensor",
				orgSlug, "digitalmatter", modelID, devEUI, "trip_count",
			),
			EntityType:  "sensor",
			DeviceClass: "trip_count",
			Name:        "Trip Count",
			State:       yabbyData.TripCount,
			Attributes: map[string]interface{}{
				"device_model": "Yabby Edge LoRaWAN",
				"uptime_weeks": yabbyData.UptimeWeeks,
			},
			Enabled:   true,
			Timestamp: timestamp,
		})

	case 5: // Location message
		// Location Entity
		locationState := "unknown"
		locationSource := "wifi_gnss_scan"
		locationAttrs := map[string]interface{}{
			"device_model":      "Yabby Edge LoRaWAN",
			"gps_capable":       true,
			"source":            locationSource,
			"in_trip":           yabbyData.InTrip,
			"inactive":          yabbyData.Inactive,
			"time_set":          yabbyData.TimeSet,
			"position_sequence": yabbyData.PosSeq,
		}

		if len(yabbyData.WiFi) > 0 {
			locationAttrs["wifi_scan_count"] = len(yabbyData.WiFi)
			wifiData := make([]map[string]interface{}, 0, len(yabbyData.WiFi))
			for _, w := range yabbyData.WiFi {
				wifiData = append(wifiData, map[string]interface{}{
					"rssi": w.RSSI,
					"mac":  w.MAC,
				})
			}
			locationAttrs["wifi_scans"] = wifiData
		}
		if yabbyData.NavData != "" {
			locationAttrs["has_gnss_data"] = true
			locationAttrs["gnss_nav_data"] = yabbyData.NavData
		}

		// Check for resolved location from Location Engine (in metadata)
		if lat, lon, alt, acc, ok := extractResolvedLocation(payload.Metadata); ok {
			locationState = fmt.Sprintf("%f,%f", lat, lon)
			locationSource = "location_engine"
			locationAttrs["latitude"] = lat
			locationAttrs["longitude"] = lon
			locationAttrs["altitude"] = alt
			locationAttrs["accuracy"] = acc
			locationAttrs["source"] = locationSource
		} else if deviceLocation != nil {
			// Use trilateration or externally provided location
			locationState = fmt.Sprintf("%f,%f", deviceLocation.Latitude, deviceLocation.Longitude)
			locationAttrs["latitude"] = deviceLocation.Latitude
			locationAttrs["longitude"] = deviceLocation.Longitude
			locationAttrs["source"] = "trilateration"
		}

		entities = append(entities, components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "location"),
			EntityID: components.GenerateEntityID(
				components.GetEntityDomain("location"),
				orgSlug, "digitalmatter", modelID, devEUI, "location",
			),
			EntityType:  "location",
			DeviceClass: "location",
			Name:        "Location",
			State:       locationState,
			DisplayType: []string{"map"},
			Attributes:  locationAttrs,
			Enabled:     true,
			Timestamp:   timestamp,
		})

		// Trip status entity (binary sensor)
		entities = append(entities, components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "trip_status"),
			EntityID: components.GenerateEntityID(
				"binary_sensor",
				orgSlug, "digitalmatter", modelID, devEUI, "trip_status",
			),
			EntityType:  "binary_sensor",
			DeviceClass: "moving",
			Name:        "Trip Status",
			State:       yabbyData.InTrip,
			Attributes: map[string]interface{}{
				"device_model": "Yabby Edge LoRaWAN",
				"in_trip":      yabbyData.InTrip,
			},
			Enabled:   true,
			Timestamp: timestamp,
		})

		// Inactivity indicator entity (binary sensor)
		entities = append(entities, components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "inactivity"),
			EntityID: components.GenerateEntityID(
				"binary_sensor",
				orgSlug, "digitalmatter", modelID, devEUI, "inactivity",
			),
			EntityType:  "binary_sensor",
			DeviceClass: "problem",
			Name:        "Inactivity",
			State:       yabbyData.Inactive,
			Attributes: map[string]interface{}{
				"device_model": "Yabby Edge LoRaWAN",
				"inactive":     yabbyData.Inactive,
			},
			Enabled:   true,
			Timestamp: timestamp,
		})

	case 89: // Connect message
		entities = append(entities, components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "connect"),
			EntityID: components.GenerateEntityID(
				"sensor",
				orgSlug, "digitalmatter", modelID, devEUI, "connect",
			),
			EntityType:  "sensor",
			DeviceClass: "connectivity",
			Name:        "Connection Status",
			State:       "connected",
			Attributes: map[string]interface{}{
				"device_model":      "Yabby Edge LoRaWAN",
				"connect_id":        yabbyData.ConnectID,
				"firmware_version":  fmt.Sprintf("%d.%d", yabbyData.FwMaj, yabbyData.FwMin),
				"product_id":        yabbyData.ProdID,
				"hardware_revision": yabbyData.HwRev,
				"dev_reset":         yabbyData.DevReset,
				"fcnt_reset":        yabbyData.FcntReset,
			},
			Enabled:   true,
			Timestamp: timestamp,
		})
	}

	return entities, nil
}

// createBatteryVoltageEntity creates a battery voltage sensor entity.
func (p *YabbyEdgeParser) createBatteryVoltageEntity(orgSlug, model, modelID, devEUI string, voltage float64, ts time.Time) components.Entity {
	return components.Entity{
		UniqueID: components.GenerateUniqueID(model, devEUI, "battery_voltage"),
		EntityID: components.GenerateEntityID(
			components.GetEntityDomain("battery"),
			orgSlug, "digitalmatter", modelID, devEUI, "battery_voltage",
		),
		EntityType:  "battery",
		DeviceClass: "voltage",
		Name:        "Battery Voltage",
		State:       voltage,
		UnitOfMeas:  "V",
		Icon:        "mdi:battery",
		DisplayType: []string{"chart", "gauge", "value"},
		Attributes: map[string]interface{}{
			"device_model": "Yabby Edge LoRaWAN",
			"battery_type": "2x AAA LiFeS2",
		},
		Enabled:   true,
		Timestamp: ts,
	}
}

// createBatteryPercentEntity creates a battery percentage sensor entity.
func (p *YabbyEdgeParser) createBatteryPercentEntity(orgSlug, model, modelID, devEUI string, voltage float64, ts time.Time) components.Entity {
	pct := estimateBatteryPercent(voltage)
	return components.Entity{
		UniqueID: components.GenerateUniqueID(model, devEUI, "battery_level"),
		EntityID: components.GenerateEntityID(
			components.GetEntityDomain("battery"),
			orgSlug, "digitalmatter", modelID, devEUI, "battery_level",
		),
		EntityType:  "battery",
		DeviceClass: "battery",
		Name:        "Battery Level",
		State:       math.Round(pct),
		UnitOfMeas:  "%",
		Icon:        "mdi:battery",
		DisplayType: []string{"chart", "gauge", "value", "slider"},
		Attributes: map[string]interface{}{
			"device_model":   "Yabby Edge LoRaWAN",
			"battery_type":   "2x AAA LiFeS2",
			"source_voltage": voltage,
		},
		Enabled:   true,
		Timestamp: ts,
	}
}

// The recommended battery for the Yabby Edge.
// estimateBatteryPercent estimates battery percentage from voltage.
// Yabby Edge uses 2x AAA LiFeS2 batteries (3.0-3.6V total, i.e. 1.5-1.8V per cell).
// Total voltage range: ~2.0V (empty) to ~3.6V (fresh).
// We use a linear approximation across the usable range.
func estimateBatteryPercent(voltage float64) float64 {
	const (
		vMin = 2.0 // Minimum operating voltage (device cutoff)
		vMax = 3.6 // Fresh battery voltage (2x 1.8V LiFeS2)
	)
	if voltage <= vMin {
		return 0
	}
	if voltage >= vMax {
		return 100
	}
	return (voltage - vMin) / (vMax - vMin) * 100
}
