package lns

import "fmt"

// ExtractDevEUI extracts the device EUI from metadata using the LNS-specific handler.
func ExtractDevEUI(metadata map[string]interface{}, lnsType LNSType) string {
	handler, err := GetLNSHandler(lnsType)
	if err != nil {
		return ""
	}
	return handler.ExtractDevEUI(metadata)
}

// ExtractFPort extracts the LoRaWAN fPort from metadata.
func ExtractFPort(metadata map[string]interface{}, lnsType LNSType) int {
	handler, err := GetLNSHandler(lnsType)
	if err != nil {
		return 0
	}
	return handler.ExtractFPort(metadata)
}

// ExtractFrequency extracts the uplink frequency from metadata.
func ExtractFrequency(metadata map[string]interface{}, lnsType LNSType) (float64, error) {
	handler, err := GetLNSHandler(lnsType)
	if err != nil {
		return 0, fmt.Errorf("no handler found for LNS type %s: %w", lnsType, err)
	}
	return handler.ExtractFrequency(metadata)
}

// ExtractRxMetadata extracts gateway / rx-metadata from the payload.
func ExtractRxMetadata(metadata map[string]interface{}, lnsType LNSType) ([]interface{}, error) {
	handler, err := GetLNSHandler(lnsType)
	if err != nil {
		return nil, fmt.Errorf("no handler found for LNS type %s: %w", lnsType, err)
	}
	return handler.ExtractRxMetadata(metadata)
}

// ExtractPayloadDataFromMetadata extracts the raw payload data string from metadata.
func ExtractPayloadDataFromMetadata(metadata map[string]interface{}, lnsType LNSType) string {
	handler, err := GetLNSHandler(lnsType)
	if err != nil {
		return ""
	}
	return handler.ExtractPayloadData(metadata)
}

// ExtractLNSSource reads the LNS type from a top-level MPA payload map.
// MPA service sets metadata.lorawan_source — that is the single source of truth.
func ExtractLNSSource(payload map[string]interface{}) LNSType {
	if payload == nil {
		return LNSTypeUnknown
	}
	metadata, ok := payload["metadata"].(map[string]interface{})
	if !ok {
		return LNSTypeUnknown
	}
	source, ok := metadata["lorawan_source"].(string)
	if !ok || source == "" {
		return LNSTypeUnknown
	}
	return ParseLNSType(source)
}

func ExtractEventType(payload map[string]interface{}, lnsType LNSType) EventType {
	handler, err := GetLNSHandler(lnsType)
	if err != nil {
		return EventUnknown
	}
	return handler.ExtractEventType(payload)
}

func ExtractAlert(payload map[string]interface{}, lnsType LNSType) *LNSAlert {
	handler, err := GetLNSHandler(lnsType)
	if err != nil {
		return nil
	}
	return handler.ExtractAlert(payload)
}
