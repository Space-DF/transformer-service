package devices

import (
	"context"
	"fmt"
	"sync"
)

// Registry manages all registered device parsers
type Registry struct {
	mu      sync.RWMutex
	parsers map[DeviceType]DeviceParser
}

// NewRegistry creates a new device parser registry
func NewRegistry() *Registry {
	return &Registry{
		parsers: make(map[DeviceType]DeviceParser),
	}
}

// Register adds a device parser to the registry
func (r *Registry) Register(parser DeviceParser) error {
	if parser == nil {
		return fmt.Errorf("cannot register nil parser")
	}

	deviceType := parser.GetDeviceType()
	if deviceType == "" {
		return fmt.Errorf("parser must have a valid device type")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.parsers[deviceType]; exists {
		return fmt.Errorf("parser for device type %s already registered", deviceType)
	}

	r.parsers[deviceType] = parser
	return nil
}

// Unregister removes a device parser from the registry
func (r *Registry) Unregister(deviceType DeviceType) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.parsers, deviceType)
}

// GetParser retrieves a parser by device type
func (r *Registry) GetParser(deviceType DeviceType) (DeviceParser, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	parser, exists := r.parsers[deviceType]
	if !exists {
		return nil, fmt.Errorf("no parser registered for device type: %s", deviceType)
	}

	return parser, nil
}

// DetectAndParse attempts to detect the device type and parse the payload
// It tries all registered parsers until one succeeds
func (r *Registry) DetectAndParse(ctx context.Context, payload *RawPayload) (*ParsedData, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try each parser's CanParse method
	for _, parser := range r.parsers {
		if parser.CanParse(payload) {
			parsed, err := parser.Parse(ctx, payload)
			if err != nil {
				return nil, fmt.Errorf("parser %s failed: %w", parser.GetDeviceType(), err)
			}

			// Validate the parsed data
			if err := parser.Validate(parsed); err != nil {
				return nil, fmt.Errorf("validation failed for %s: %w", parser.GetDeviceType(), err)
			}

			return parsed, nil
		}
	}

	return nil, fmt.Errorf("no parser could handle the payload for device %s", payload.DeviceEUI)
}

// ParseWithType parses using a specific device type parser
func (r *Registry) ParseWithType(ctx context.Context, payload *RawPayload, deviceType DeviceType) (*ParsedData, error) {
	parser, err := r.GetParser(deviceType)
	if err != nil {
		return nil, err
	}

	parsed, err := parser.Parse(ctx, payload)
	if err != nil {
		return nil, fmt.Errorf("parsing failed: %w", err)
	}

	if err := parser.Validate(parsed); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return parsed, nil
}

// ListRegisteredParsers returns all registered device types
func (r *Registry) ListRegisteredParsers() []DeviceType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]DeviceType, 0, len(r.parsers))
	for deviceType := range r.parsers {
		types = append(types, deviceType)
	}
	return types
}

// GetAllMetadata returns metadata for all registered parsers
func (r *Registry) GetAllMetadata() []ParserMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metadata := make([]ParserMetadata, 0, len(r.parsers))
	for _, parser := range r.parsers {
		if p, ok := parser.(DeviceParserWithMetadata); ok {
			metadata = append(metadata, p.GetMetadata())
		}
	}
	return metadata
}

// Global registry instance (optional - for convenience)
var globalRegistry = NewRegistry()

// RegisterGlobal registers a parser in the global registry
func RegisterGlobal(parser DeviceParser) error {
	return globalRegistry.Register(parser)
}

// GetGlobalRegistry returns the global registry instance
func GetGlobalRegistry() *Registry {
	return globalRegistry
}
