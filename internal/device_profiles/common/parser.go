package common

import (
	"encoding/base64"

	"github.com/Space-DF/transformer-service/internal/lns"
)

// ExtractBytes returns raw payload bytes from a RawPayload.
// It first tries the LNS handler (when LNSType is set), then falls back to
// standard base64 decoding of the Data field.
func ExtractBytes(payload *RawPayload) []byte {
	if payload.LNSType != "" {
		if h, err := lns.GetLNSHandler(payload.LNSType); err == nil {
			if raw, err := h.ExtractPayloadBytes(payload.Metadata); err == nil && raw != nil {
				return raw
			}
		}
	}
	if payload.Data != "" {
		if raw, err := base64.StdEncoding.DecodeString(payload.Data); err == nil {
			return raw
		}
	}
	return nil
}
