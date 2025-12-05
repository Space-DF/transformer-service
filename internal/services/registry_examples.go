package services

// This file demonstrates how to use the device parser system
// It's meant as a reference for integration, not for actual use

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/Space-DF/transformer-service/internal/devices"
)

// ExampleBasicUsage demonstrates basic usage of the device parser system
func ExampleBasicUsage() {
	// 1. Create a registry
	registry := NewRegistry()

	// 2. Register device parsers (import actual parsers)
	// Example: registry.Register(rak2270.NewParser())

	// 3. Prepare raw payload (this would come from RabbitMQ)
	rawPayload := &devices.RawPayload{
		DeviceEUI: "1486e6546b37cec8",
		FPort:     2,
		Data:      "base64_encoded_data_here",
		// ... other fields
	}

	// 4. Auto-detect and parse
	ctx := context.Background()
	parsed, err := registry.DetectAndParse(ctx, rawPayload)
	if err != nil {
		log.Printf("Failed to parse device data: %v", err)
		return
	}

	// 5. Use parsed data
	fmt.Printf("Device Type: %s\n", parsed.DeviceType)
	if parsed.Location != nil {
		fmt.Printf("Location: %.6f, %.6f\n", parsed.Location.Latitude, parsed.Location.Longitude)
	}
}

// ExampleWithSpecificType demonstrates parsing with a known device type
func ExampleWithSpecificType() {
	registry := NewRegistry()

	rawPayload := &devices.RawPayload{
		DeviceEUI: "1486e6546b37cec8",
		FPort:     2,
		Data:      "base64_encoded_data_here",
	}

	// If you already know the device type (e.g., from database)
	ctx := context.Background()
	parsed, err := registry.ParseWithType(ctx, rawPayload, devices.DeviceTypeRAK2270)
	if err != nil {
		log.Printf("Failed to parse: %v", err)
		return
	}

	fmt.Printf("Parsed device: %s\n", parsed.DeviceEUI)
}

// ExampleIntegrationWithRabbitMQ shows how to integrate with message queue
func ExampleIntegrationWithRabbitMQ(registry *Registry, messageBody []byte) error {
	// 1. Unmarshal RabbitMQ message
	var rawPayload devices.RawPayload
	if err := json.Unmarshal(messageBody, &rawPayload); err != nil {
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	// 2. Parse device data
	ctx := context.Background()
	parsed, err := registry.DetectAndParse(ctx, &rawPayload)
	if err != nil {
		return fmt.Errorf("failed to parse device data: %w", err)
	}

	// 3. Process parsed data
	return processDeviceData(parsed)
}

// ExampleListRegisteredDevices shows how to query available parsers
func ExampleListRegisteredDevices(registry *Registry) {
	// Get all registered device types
	deviceTypes := registry.ListRegisteredParsers()
	fmt.Printf("Registered device types: %v\n", deviceTypes)

	// Get detailed metadata
	metadata := registry.GetAllMetadata()
	for _, meta := range metadata {
		fmt.Printf("Device: %s (%s %s)\n", meta.DeviceType, meta.Manufacturer, meta.Model)
		fmt.Printf("  Supported FPorts: %v\n", meta.SupportedPorts)
	}
}

// processDeviceData is a placeholder for actual data processing
func processDeviceData(data *devices.ParsedData) error {
	// Here you would:
	// 1. Perform trilateration if needed
	// 2. Store location data
	// 3. Update device cache
	// 4. Publish to MQTT
	// 5. Log metrics
	fmt.Printf("Processing data for device %s\n", data.DeviceEUI)
	return nil
}

// ExampleErrorHandling demonstrates proper error handling
func ExampleErrorHandling(registry *Registry, rawPayload *devices.RawPayload) {
	ctx := context.Background()

	parsed, err := registry.DetectAndParse(ctx, rawPayload)
	if err != nil {
		// Log error with context
		log.Printf("Device parsing failed for EUI %s: %v", rawPayload.DeviceEUI, err)

		// You might want to:
		// 1. Send to dead letter queue
		// 2. Alert monitoring system
		// 3. Store raw payload for manual inspection
		// 4. Update device error metrics

		return
	}

	// Success path
	log.Printf("Successfully parsed device %s (type: %s)", parsed.DeviceEUI, parsed.DeviceType)
}

// ExampleServiceInitialization shows how to initialize the parser system at service startup
func ExampleServiceInitialization() *Registry {
	registry := NewRegistry()

	// Option 1: Register built-in parsers directly
	// Example (uncomment when implemented):
	// registry.Register(rak2270.NewParser())
	// registry.Register(other_device.NewParser())

	// Option 2: Load parsers from plugins directory
	loader := NewPluginLoader("./plugins", registry)
	if err := loader.LoadAll(); err != nil {
		log.Printf("Warning: failed to load some plugins: %v", err)
	}

	log.Printf("Device parser registry initialized with %d parsers",
		len(registry.ListRegisteredParsers()))

	return registry
}

// ExamplePluginLoading demonstrates dynamic plugin loading
func ExamplePluginLoading() {
	registry := NewRegistry()

	// Load all plugins from directory
	loader := NewPluginLoader("./plugins", registry)
	if err := loader.LoadAll(); err != nil {
		log.Fatalf("Failed to load plugins: %v", err)
	}

	// List loaded parsers
	for _, deviceType := range registry.ListRegisteredParsers() {
		fmt.Printf("Loaded parser: %s\n", deviceType)
	}

	// Get metadata for all parsers
	for _, meta := range registry.GetAllMetadata() {
		fmt.Printf("Device: %s %s (%s)\n", meta.Manufacturer, meta.Model, meta.DeviceType)
	}
}

// ExampleLoadSpecificPlugin shows loading a single plugin
func ExampleLoadSpecificPlugin() {
	registry := NewRegistry()
	loader := NewPluginLoader("./plugins", registry)

	// Load specific plugin
	if err := loader.LoadPluginFromPath("./plugins/rak2270.dylib"); err != nil {
		log.Printf("Failed to load RAK2270 plugin: %v", err)
		return
	}

	fmt.Println("RAK2270 plugin loaded successfully")
}
