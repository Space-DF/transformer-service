package services

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/Space-DF/transformer-service/internal/devices"
)

// Registry manages all registered device parsers and device entries
type Registry struct {
	mu      sync.RWMutex
	parsers map[devices.DeviceType]devices.DeviceParser
	
	// Device management
	devices           map[string]*devices.DeviceEntry          // deviceID → DeviceEntry
	identifierIndex   map[string]string                        // "type:key:value" → deviceID  
	connectionIndex   map[string]string                        // "type:value" → deviceID
	
	// Redis cache integration
	cache DeviceRegistryCache
}

// NewRegistry creates a new device parser registry
func NewRegistry() *Registry {
	return &Registry{
		parsers:         make(map[devices.DeviceType]devices.DeviceParser),
		devices:         make(map[string]*devices.DeviceEntry),
		identifierIndex: make(map[string]string),
		connectionIndex: make(map[string]string),
	}
}

// NewRegistryWithCache creates a new registry with Redis cache support
func NewRegistryWithCache() *Registry {
	registry := NewRegistry()
	registry.cache = NewDeviceRegistryCacheFromEnv() // Use the cache we enhanced
	return registry
}

// Register adds a device parser to the registry
func (r *Registry) Register(parser devices.DeviceParser) error {
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
func (r *Registry) Unregister(deviceType devices.DeviceType) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.parsers, deviceType)
}

// GetParser retrieves a parser by device type
func (r *Registry) GetParser(deviceType devices.DeviceType) (devices.DeviceParser, error) {
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
func (r *Registry) DetectAndParse(ctx context.Context, payload *devices.RawPayload) (*devices.ParsedData, error) {
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
func (r *Registry) ParseWithType(ctx context.Context, payload *devices.RawPayload, deviceType devices.DeviceType) (*devices.ParsedData, error) {
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
func (r *Registry) ListRegisteredParsers() []devices.DeviceType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]devices.DeviceType, 0, len(r.parsers))
	for deviceType := range r.parsers {
		types = append(types, deviceType)
	}
	return types
}

// GetAllMetadata returns metadata for all registered parsers
func (r *Registry) GetAllMetadata() []devices.ParserMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metadata := make([]devices.ParserMetadata, 0, len(r.parsers))
	for _, parser := range r.parsers {
		if p, ok := parser.(devices.DeviceParserWithMetadata); ok {
			metadata = append(metadata, p.GetMetadata())
		}
	}
	return metadata
}

// Global registry instance (optional - for convenience)
var globalRegistry = NewRegistry()

// RegisterGlobal registers a parser in the global registry
func RegisterGlobal(parser devices.DeviceParser) error {
	return globalRegistry.Register(parser)
}

// GetGlobalRegistry returns the global registry instance
func GetGlobalRegistry() *Registry {
	return globalRegistry
}

// Device management methods

// RegisterDevice adds or updates a device entry in the registry
func (r *Registry) RegisterDevice(ctx context.Context, device *devices.DeviceEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Store in memory
	r.devices[device.ID] = device

	// Update identifier indexes
	for _, identifier := range device.Identifiers {
		key := r.makeIdentifierKey(identifier.Type, identifier.Key, identifier.Value)
		r.identifierIndex[key] = device.ID
	}

	// Update connection indexes
	for _, connection := range device.Connections {
		key := r.makeConnectionKey(connection.Type, connection.Value)
		r.connectionIndex[key] = device.ID
	}

	// Store in Redis if cache is available
	if r.cache != nil {
		if err := r.cache.SetDeviceEntry(ctx, device.Organization, device.ID, *device); err != nil {
			log.Printf("Failed to cache device entry: %v", err)
		}

		// Store identifier indexes in Redis
		for _, identifier := range device.Identifiers {
			if err := r.cache.SetIdentifierMapping(ctx, device.Organization, identifier.Type, identifier.Key, identifier.Value, device.ID); err != nil {
				log.Printf("Failed to cache identifier mapping: %v", err)
			}
		}

		// Store connection indexes in Redis
		for _, connection := range device.Connections {
			if err := r.cache.SetConnectionMapping(ctx, device.Organization, connection.Type, connection.Value, device.ID); err != nil {
				log.Printf("Failed to cache connection mapping: %v", err)
			}
		}
	}

	return nil
}

// GetDevice retrieves a device by ID
func (r *Registry) GetDevice(ctx context.Context, deviceID string) (*devices.DeviceEntry, error) {
	r.mu.RLock()
	
	// Check memory first
	if device, exists := r.devices[deviceID]; exists {
		r.mu.RUnlock()
		return device, nil
	}
	r.mu.RUnlock()

	// We need organization context to check Redis cache - this method needs refactoring
	// For now, skip Redis cache lookup without organization context

	return nil, fmt.Errorf("device not found: %s", deviceID)
}

// GetDeviceByIdentifiers implements multi-identifier lookup pattern
func (r *Registry) GetDeviceByIdentifiers(ctx context.Context, org string, identifiers []devices.DeviceIdentifier) (*devices.DeviceEntry, error) {
	// Try each identifier (no switch case required)
	for _, identifier := range identifiers {
		if device, err := r.getDeviceByIdentifier(ctx, org, identifier.Type, identifier.Key, identifier.Value); err == nil {
			return device, nil
		}
	}
	return nil, fmt.Errorf("no device found for provided identifiers")
}

// GetDeviceByConnections implements connection lookup pattern
func (r *Registry) GetDeviceByConnections(ctx context.Context, org string, connections []devices.DeviceConnection) (*devices.DeviceEntry, error) {
	// Try each connection (no switch case required)
	for _, connection := range connections {
		if device, err := r.getDeviceByConnection(ctx, org, connection.Type, connection.Value); err == nil {
			return device, nil
		}
	}
	return nil, fmt.Errorf("no device found for provided connections")
}

// getDeviceByIdentifier internal helper for identifier lookup with organization context
func (r *Registry) getDeviceByIdentifier(ctx context.Context, org, identifierType, key, value string) (*devices.DeviceEntry, error) {
	r.mu.RLock()
	
	// Check memory index first
	indexKey := r.makeIdentifierKey(identifierType, key, value)
	if deviceID, exists := r.identifierIndex[indexKey]; exists {
		r.mu.RUnlock()
		return r.GetDevice(ctx, deviceID)
	}
	r.mu.RUnlock()

	// Check Redis cache if available
	if r.cache != nil {
		deviceID, err := r.cache.GetDeviceByIdentifier(ctx, org, identifierType, key, value)
		if err == nil {
			return r.GetDevice(ctx, deviceID)
		}
		if err != ErrCacheMiss {
			log.Printf("Redis identifier lookup error: %v", err)
		}
	}

	return nil, fmt.Errorf("device not found for identifier %s:%s:%s", identifierType, key, value)
}

// getDeviceByConnection internal helper for connection lookup with organization context
func (r *Registry) getDeviceByConnection(ctx context.Context, org, connectionType, value string) (*devices.DeviceEntry, error) {
	r.mu.RLock()
	
	// Check memory index first
	indexKey := r.makeConnectionKey(connectionType, value)
	if deviceID, exists := r.connectionIndex[indexKey]; exists {
		r.mu.RUnlock()
		return r.GetDevice(ctx, deviceID)
	}
	r.mu.RUnlock()

	// Check Redis cache if available
	if r.cache != nil {
		deviceID, err := r.cache.GetDeviceByConnection(ctx, org, connectionType, value)
		if err == nil {
			return r.GetDevice(ctx, deviceID)
		}
		if err != ErrCacheMiss {
			log.Printf("Redis connection lookup error: %v", err)
		}
	}

	return nil, fmt.Errorf("device not found for connection %s:%s", connectionType, value)
}

// UpdateDeviceLastSeen updates the last seen timestamp for a device
func (r *Registry) UpdateDeviceLastSeen(ctx context.Context, deviceID string) error {
	device, err := r.GetDevice(ctx, deviceID)
	if err != nil {
		return err
	}

	device.UpdateLastSeen()
	return r.RegisterDevice(ctx, device)
}

// Helper methods for creating index keys
func (r *Registry) makeIdentifierKey(identifierType, key, value string) string {
	return fmt.Sprintf("%s:%s:%s", identifierType, key, value)
}

func (r *Registry) makeConnectionKey(connectionType, value string) string {
	return fmt.Sprintf("%s:%s", connectionType, value)
}