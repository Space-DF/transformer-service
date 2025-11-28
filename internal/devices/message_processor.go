package devices

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Space-DF/transformer-service/internal/models"
)

// MessageProcessor handles enhanced message processing with Device Registry integration
type MessageProcessor struct {
	detector *IdentifierDetector
	registry *Registry
}

// NewMessageProcessor creates a new message processor
func NewMessageProcessor(registry *Registry) *MessageProcessor {
	return &MessageProcessor{
		detector: NewIdentifierDetector(),
		registry: registry,
	}
}

// ProcessingContext combines all device information for message processing
type ProcessingContext struct {
	// Device identifiers
	Identifiers     []DeviceIdentifier `json:"identifiers"`
	PrimaryID       string             `json:"primary_id"`       // For Device Service API
	ProtocolType    string             `json:"protocol_type"`    // "lorawan", "satellite", etc.
	
	// Device entries
	TechnicalDevice *DeviceEntry       `json:"technical_device"` // From Device Registry
	BusinessDevice  *models.DeviceMapping `json:"business_device"`  // From Device Service
	
	// Processing state
	ShouldSkip      bool               `json:"should_skip"`
	Organization    string             `json:"organization"`
	Parser          DeviceParser       `json:"-"`                // Selected parser
}

// ProcessMessage processes an MQTT message using Device Registry
func (mp *MessageProcessor) ProcessMessage(ctx context.Context, orgSlug string, payload, locationPayload map[string]interface{}) (*ProcessingContext, error) {
	// Step 1: Detect all possible identifiers from payload
	identifiers := mp.detector.DetectIdentifiers(payload, locationPayload)
	if len(identifiers) == 0 {
		return nil, fmt.Errorf("no device identifiers found in message payload")
	}

	mp.detector.LogDetectedIdentifiers(identifiers)

	// Step 2: Try to find existing device in registry
	device, primaryID, protocolType, err := mp.findDevice(ctx, orgSlug, identifiers)
	if err != nil {
		// Device not found - could create new device or handle as unknown
		log.Printf("Device not found in registry: %v", err)
		return mp.handleUnknownDevice(ctx, orgSlug, identifiers, payload, locationPayload)
	}

	// Step 3: Create processing context
	return mp.createProcessingContext(ctx, orgSlug, identifiers, device, primaryID, protocolType)
}

// findDevice tries to find device using multiple identifiers
func (mp *MessageProcessor) findDevice(ctx context.Context, orgSlug string, identifiers []DeviceIdentifier) (*DeviceEntry, string, string, error) {
	// Try each identifier type with priority order
	priorityOrder := []string{"lorawan", "satellite", "cellular", "network", "wifi", "bluetooth", "hardware"}

	for _, priority := range priorityOrder {
		for _, identifier := range identifiers {
			if identifier.Type == priority {
				device, err := mp.registry.getDeviceByIdentifier(ctx, orgSlug, identifier.Type, identifier.Key, identifier.Value)
				if err == nil {
					// Found device! Return with primary identifier for Business Device lookup
					primaryID := mp.determinePrimaryID(identifier, device)
					return device, primaryID, identifier.Type, nil
				}
			}
		}
	}

	return nil, "", "", fmt.Errorf("device not found for any identifier")
}

// determinePrimaryID determines which identifier to use for Device Service API lookup
func (mp *MessageProcessor) determinePrimaryID(foundIdentifier DeviceIdentifier, device *DeviceEntry) string {
	// For Device Service API, we need to use the identifier it expects
	switch foundIdentifier.Type {
	case "lorawan":
		return device.GetDevEUI() // Use DevEUI for Device Service API
	case "satellite":
		return device.GetESN() // Use ESN for Device Service API
	case "network", "wifi":
		// For network devices, try to find a primary identifier the API understands
		if devEUI := device.GetDevEUI(); devEUI != "" {
			return devEUI // Prefer DevEUI if available
		}
		if esn := device.GetESN(); esn != "" {
			return esn // Fallback to ESN
		}
		return foundIdentifier.Value // Use the found identifier
	default:
		return foundIdentifier.Value
	}
}

// handleUnknownDevice handles devices not found in registry
func (mp *MessageProcessor) handleUnknownDevice(ctx context.Context, orgSlug string, identifiers []DeviceIdentifier, payload, locationPayload map[string]interface{}) (*ProcessingContext, error) {
	// For unknown devices, we can:
	// 1. Try Device Service API with available identifiers
	// 2. Create a new device entry in registry
	// 3. Use default processing

	// Find the best identifier for Device Service API lookup
	primaryID, protocolType := mp.findBestIdentifierForAPI(identifiers)
	if primaryID == "" {
		return nil, fmt.Errorf("no suitable identifier found for Device Service API lookup")
	}

	return &ProcessingContext{
		Identifiers:     identifiers,
		PrimaryID:       primaryID,
		ProtocolType:    protocolType,
		TechnicalDevice: nil, // Will be created later
		BusinessDevice:  nil, // Will be fetched from API
		ShouldSkip:      false,
		Organization:    orgSlug,
		Parser:          nil, // Will be determined later
	}, nil
}

// findBestIdentifierForAPI finds the best identifier for Device Service API
func (mp *MessageProcessor) findBestIdentifierForAPI(identifiers []DeviceIdentifier) (string, string) {
	// Priority order for Device Service API compatibility
	priorities := []struct {
		Type string
		Key  string
	}{
		{"lorawan", "dev_eui"},     // Highest priority - existing system
		{"satellite", "esn"},       // Second priority - satellite systems
		{"cellular", "imei"},       // Third priority - cellular devices
		{"network", "mac"},         // Fourth priority - network devices
		{"hardware", "serial"},     // Fifth priority - hardware serial
	}

	for _, priority := range priorities {
		for _, identifier := range identifiers {
			if identifier.Type == priority.Type && identifier.Key == priority.Key {
				return identifier.Value, identifier.Type
			}
		}
	}

	// Fallback: return first available identifier
	if len(identifiers) > 0 {
		return identifiers[0].Value, identifiers[0].Type
	}

	return "", ""
}

// createProcessingContext creates a complete processing context
func (mp *MessageProcessor) createProcessingContext(ctx context.Context, orgSlug string, identifiers []DeviceIdentifier, device *DeviceEntry, primaryID, protocolType string) (*ProcessingContext, error) {
	// Update device last seen
	if err := mp.registry.UpdateDeviceLastSeen(ctx, device.ID); err != nil {
		log.Printf("Failed to update device last seen: %v", err)
	}

	return &ProcessingContext{
		Identifiers:     identifiers,
		PrimaryID:       primaryID,
		ProtocolType:    protocolType,
		TechnicalDevice: device,
		BusinessDevice:  nil, // Will be populated by caller
		ShouldSkip:      false,
		Organization:    orgSlug,
		Parser:          nil, // Will be populated by caller
	}, nil
}

// GetParserForDevice gets the appropriate parser for a device
func (mp *MessageProcessor) GetParserForDevice(ctx context.Context, orgSlug string, device *DeviceEntry) (DeviceParser, error) {
	if device == nil {
		return nil, fmt.Errorf("device is nil")
	}

	// Get parser from registry
	parser, err := mp.registry.GetParser(device.DeviceType)
	if err != nil {
		return nil, fmt.Errorf("no parser found for device type %s: %w", device.DeviceType, err)
	}

	return parser, nil
}

// DetermineDeviceType determines device type from identifiers and payload
func (mp *MessageProcessor) DetermineDeviceType(identifiers []DeviceIdentifier, payload map[string]interface{}) DeviceType {
	// Try to determine device type from various sources
	
	// 1. Check for explicit device type in payload
	if deviceType, ok := payload["device_type"].(string); ok && deviceType != "" {
		return DeviceType(deviceType)
	}

	// 2. Determine from identifier types
	for _, identifier := range identifiers {
		switch identifier.Type {
		case "lorawan":
			// Could be RAK2270 or other LoRaWAN devices
			// Check payload structure or other hints
			if mp.detectRAK2270Pattern(payload) {
				return DeviceTypeRAK2270
			}
			return "LORAWAN_GENERIC"
		case "satellite":
			return "SATELLITE_TRACKER"
		case "cellular":
			return "CELLULAR_TRACKER"
		}
	}

	// 3. Fallback to unknown
	return DeviceTypeUnknown
}

// detectRAK2270Pattern detects if payload matches RAK2270 device pattern
func (mp *MessageProcessor) detectRAK2270Pattern(payload map[string]interface{}) bool {
	// Check for RAK2270-specific payload characteristics
	// This is device-specific logic that can be enhanced
	if fPort, ok := payload["f_port"].(float64); ok && fPort == 2 {
		// RAK2270 typically uses fPort 2
		return true
	}
	
	// Check for RAK-specific field patterns
	if uplinkMsg, ok := payload["uplink_message"].(map[string]interface{}); ok {
		if settings, ok := uplinkMsg["settings"].(map[string]interface{}); ok {
			// Check frequency ranges typical for RAK2270
			if freq, ok := settings["frequency"].(float64); ok {
				// LoRaWAN frequency ranges
				return freq >= 860000000 && freq <= 930000000
			}
		}
	}

	return false
}

// CreateDeviceFromIdentifiers creates a new device entry from identifiers
func (mp *MessageProcessor) CreateDeviceFromIdentifiers(ctx context.Context, orgSlug string, identifiers []DeviceIdentifier, payload map[string]interface{}) (*DeviceEntry, error) {
	deviceType := mp.DetermineDeviceType(identifiers, payload)
	
	device := &DeviceEntry{
		ID:           generateDeviceID(),
		Identifiers:  identifiers,
		DeviceType:   deviceType,
		Organization: orgSlug,
		// Add other fields as needed
	}

	// Register the new device
	if err := mp.registry.RegisterDevice(ctx, device); err != nil {
		return nil, fmt.Errorf("failed to register new device: %w", err)
	}

	return device, nil
}

// Helper function to generate device ID
func generateDeviceID() string {
	// In production, use UUID or similar
	return fmt.Sprintf("dev-%d", time.Now().UnixNano())
}