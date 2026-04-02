package components

import (
	"encoding/base64"
	"fmt"
	"math"
	"strings"

	"github.com/Space-DF/transformer-service/internal/models"
)

const (
	CoordScale = 10000000
)

// ValidateCoordinates validates that coordinates are reasonable
func ValidateCoordinates(lat, lon float64) error {
	if math.Abs(lat) > 90 || math.Abs(lon) > 180 {
		return fmt.Errorf("coordinates out of valid range")
	}
	if lat == 0 && lon == 0 {
		return fmt.Errorf("null island coordinates")
	}
	return nil
}

// DecodePayloadBytes decodes hex or base64 encoded payload string to bytes
func DecodePayloadBytes(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, fmt.Errorf("empty payload data")
	}

	// base64 decode
	if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil && len(decoded) > 0 {
		return decoded, nil
	}

	return nil, fmt.Errorf("failed to decode payload as base64")
}

// LNS-Aware Extraction Functions
// These functions use the known LNS type for efficient, explicit extraction

// ExtractDevEUI extracts device EUI using LNS-specific handler
func ExtractDevEUI(metadata map[string]interface{}, lnsType models.LNSType) string {
	handler, err := models.GetLNSHandler(lnsType)
	if err != nil {
		return ""
	}
	return strings.ToLower(handler.ExtractDevEUI(metadata))
}

// ExtractFPort extracts fPort using LNS-specific handler
func ExtractFPort(metadata map[string]interface{}, lnsType models.LNSType) int {
	handler, err := models.GetLNSHandler(lnsType)
	if err != nil {
		return 0
	}
	return handler.ExtractFPort(metadata)
}

// ExtractFrequency extracts frequency using LNS-specific handler
func ExtractFrequency(metadata map[string]interface{}, lnsType models.LNSType) (float64, error) {
	handler, err := models.GetLNSHandler(lnsType)
	if err != nil {
		return 0, fmt.Errorf("no handler found for LNS type: %s: %w", lnsType, err)
	}
	return handler.ExtractFrequency(metadata)
}

// ExtractRxMetadata extracts rx metadata using LNS-specific handler
func ExtractRxMetadata(metadata map[string]interface{}, lnsType models.LNSType) ([]interface{}, error) {
	handler, err := models.GetLNSHandler(lnsType)
	if err != nil {
		return nil, fmt.Errorf("no handler found for LNS type: %s: %w", lnsType, err)
	}
	return handler.ExtractRxMetadata(metadata)
}

// ExtractPayloadDataFromMetadata extracts payload data using LNS-specific handler
func ExtractPayloadDataFromMetadata(metadata map[string]interface{}, lnsType models.LNSType) string {
	handler, err := models.GetLNSHandler(lnsType)
	if err != nil {
		return ""
	}
	return handler.ExtractPayloadData(metadata)
}
