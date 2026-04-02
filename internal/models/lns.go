package models

import (
	"fmt"
	"strings"
	"sync"
)

// LNSHandler defines the interface for LNS-specific data handling.
// Each LNS has its own payload structure, so we use specialized handlers.
type LNSHandler interface {
	// Name returns the LNS name
	Name() string

	// ExtractDevEUI extracts the device EUI from the payload
	ExtractDevEUI(payload map[string]interface{}) string

	// ExtractFPort extracts the LoRaWAN fPort from the payload
	ExtractFPort(payload map[string]interface{}) int

	// ExtractFrequency extracts the frequency from the payload
	ExtractFrequency(payload map[string]interface{}) (float64, error)

	// ExtractRxMetadata extracts the rx_metadata/gateway information from the payload
	ExtractRxMetadata(payload map[string]interface{}) ([]interface{}, error)

	// ExtractPayloadData extracts the actual payload data (frm_payload, data, etc.)
	ExtractPayloadData(payload map[string]interface{}) string

	// ExtractPayloadBytes extracts the raw payload bytes from the LNS-specific metadata
	// Returns the raw bytes that device parsers can work with
	ExtractPayloadBytes(metadata map[string]interface{}) ([]byte, error)
}

// lnsHandlerRegistry holds the registered LNS handlers
var lnsHandlerRegistry = make(map[LNSType]LNSHandler)
var lnsHandlerMutex sync.RWMutex

// RegisterLNSHandler registers an LNS handler for a specific LNS type
// Call this during init() for each LNS handler
func RegisterLNSHandler(lnsType LNSType, handler LNSHandler) {
	lnsHandlerMutex.Lock()
	defer lnsHandlerMutex.Unlock()
	lnsHandlerRegistry[lnsType] = handler
}

// GetLNSHandler retrieves the LNS handler for a given LNS type
// Returns error if no handler is registered
func GetLNSHandler(lnsType LNSType) (LNSHandler, error) {
	lnsHandlerMutex.RLock()
	defer lnsHandlerMutex.RUnlock()

	handler, ok := lnsHandlerRegistry[lnsType]
	if !ok {
		return nil, fmt.Errorf("no handler registered for LNS type: %s", lnsType)
	}
	return handler, nil
}

// MustGetLNSHandler retrieves the LNS handler or panics
// Use this when you know the LNS type is valid
func MustGetLNSHandler(lnsType LNSType) LNSHandler {
	handler, err := GetLNSHandler(lnsType)
	if err != nil {
		panic(err)
	}
	return handler
}

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
