package components

import (
	"github.com/Space-DF/transformer-service/internal/models"
)

// ExtractLNSSource extracts the LNS type from metadata.
// MPA service sets metadata.lorawan_source - we use that single source of truth.
func ExtractLNSSource(payload map[string]interface{}) models.LNSType {
	if payload == nil {
		return models.LNSTypeUnknown
	}

	metadata, ok := payload["metadata"].(map[string]interface{})
	if !ok {
		return models.LNSTypeUnknown
	}

	source, ok := metadata["lorawan_source"].(string)
	if !ok || source == "" {
		return models.LNSTypeUnknown
	}

	return models.ParseLNSType(source)
}
