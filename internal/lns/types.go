package lns

import (
	"strings"
)

// LNSType represents the LoRaWAN Network Server type
type LNSType string

const (
	// LNSTypeChirpStack represents ChirpStack LNS
	LNSTypeChirpStack LNSType = "chirpstack"
	// LNSTypeTTN represents The Things Network LNS
	LNSTypeTTN LNSType = "ttn"
	// LNSTypeHelium represents Helium LNS
	LNSTypeHelium LNSType = "helium"
	// LNSTypeUnknown represents an unknown LNS type
	LNSTypeUnknown LNSType = "unknown"
)

// String returns the string representation of LNSType
func (l LNSType) String() string {
	return string(l)
}

// Valid returns true if this is a known LNS type
func (l LNSType) Valid() bool {
	switch l {
	case LNSTypeChirpStack, LNSTypeTTN, LNSTypeHelium:
		return true
	}
	return false
}

// IsKnown returns true if the LNS type is one of the known types
func (l LNSType) IsKnown() bool {
	return l.Valid()
}

// ParseLNSType converts string to LNSType
func ParseLNSType(s string) LNSType {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "chirpstack":
		return LNSTypeChirpStack
	case "ttn":
		return LNSTypeTTN
	case "helium":
		return LNSTypeHelium
	default:
		return LNSTypeUnknown
	}
}

// GatewayMetadata represents gateway location and signal information
type GatewayMetadata struct {
	Latitude  float64
	Longitude float64
	RSSI      int
}
