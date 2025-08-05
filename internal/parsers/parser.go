package parsers

import (
	"fmt"

	"github.com/Space-DF/transformer-service/internal/models"
)

// DeviceParser interface defines methods for parsing device-specific payloads
type DeviceParser interface {
	ParsePayload(payload map[string]interface{}) (*models.DeviceLocationData, error)
	SupportsGPS() bool
}

// ParserFactory creates device-specific parsers
type ParserFactory struct{}

// NewParserFactory creates a new parser factory
func NewParserFactory() *ParserFactory {
	return &ParserFactory{}
}

// GetParser returns the appropriate parser for the given device type
func (pf *ParserFactory) GetParser(parserType string) (DeviceParser, error) {
	switch parserType {
	case "rak2270":
		return NewRAK2270Parser(), nil
	case "rak4630":
		return NewRAK4630Parser(), nil
	case "rak7200":
		return NewRAK7200Parser(), nil
	default:
		return nil, fmt.Errorf("unsupported parser type: %s", parserType)
	}
}
