package components

import (
	"github.com/Space-DF/transformer-service/internal/lns"
)

// ExtractLNSSource extracts the LNS type from metadata.
// MPA service sets metadata.lorawan_source - we use that single source of truth.
func ExtractLNSSource(payload map[string]interface{}) lns.LNSType {
	if payload == nil {
		return lns.LNSTypeUnknown
	}

	metadata, ok := payload["metadata"].(map[string]interface{})
	if !ok {
		return lns.LNSTypeUnknown
	}

	source, ok := metadata["lorawan_source"].(string)
	if !ok || source == "" {
		return lns.LNSTypeUnknown
	}

	return lns.ParseLNSType(source)
}
