package lns

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

	// ExtractGatewayLocations extracts gateway locations with signal strength
	// Returns structured gateway data for location calculation
	ExtractGatewayLocations(rxMetadata []interface{}) ([]GatewayMetadata, error)

	ExtractEventType(payload map[string]interface{}) EventType

	ExtractAlert(payload map[string]interface{}) *LNSAlert
}
