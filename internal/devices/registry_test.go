package devices

import (
	"context"
	"testing"
	"time"
)

// mockParser is a mock implementation of DeviceParser for testing
type mockParser struct {
	deviceType  DeviceType
	canParse    bool
	parseErr    error
	validateErr error
}

func (m *mockParser) GetDeviceType() DeviceType {
	return m.deviceType
}

func (m *mockParser) CanParse(payload *RawPayload) bool {
	return m.canParse
}

func (m *mockParser) Parse(ctx context.Context, payload *RawPayload) (*ParsedData, error) {
	if m.parseErr != nil {
		return nil, m.parseErr
	}
	return &ParsedData{
		DeviceEUI:  payload.DeviceEUI,
		DeviceType: m.deviceType,
		Timestamp:  payload.Timestamp,
		SensorData: make(map[string]interface{}),
	}, nil
}

func (m *mockParser) Validate(data *ParsedData) error {
	return m.validateErr
}

func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()
	if registry == nil {
		t.Fatal("NewRegistry returned nil")
	}

	if len(registry.parsers) != 0 {
		t.Errorf("Expected empty registry, got %d parsers", len(registry.parsers))
	}
}

func TestRegistry_Register(t *testing.T) {
	registry := NewRegistry()

	parser := &mockParser{
		deviceType: DeviceTypeRAK2270,
		canParse:   true,
	}

	err := registry.Register(parser)
	if err != nil {
		t.Errorf("Register failed: %v", err)
	}

	// Try to register the same type again
	err = registry.Register(parser)
	if err == nil {
		t.Error("Expected error when registering duplicate device type")
	}
}

func TestRegistry_GetParser(t *testing.T) {
	registry := NewRegistry()

	parser := &mockParser{
		deviceType: DeviceTypeRAK2270,
		canParse:   true,
	}

	err := registry.Register(parser)
	if err != nil {
		t.Errorf("Register failed: %v", err)
	}

	retrieved, err := registry.GetParser(DeviceTypeRAK2270)
	if err != nil {
		t.Errorf("GetParser failed: %v", err)
	}

	if retrieved.GetDeviceType() != DeviceTypeRAK2270 {
		t.Errorf("Retrieved wrong parser type: %s", retrieved.GetDeviceType())
	}

	// Try to get non-existent parser
	_, err = registry.GetParser(DeviceTypeUnknown)
	if err == nil {
		t.Error("Expected error when getting non-existent parser")
	}
}

func TestRegistry_DetectAndParse(t *testing.T) {
	registry := NewRegistry()
	ctx := context.Background()

	// Register a parser that can parse
	parser := &mockParser{
		deviceType: DeviceTypeRAK2270,
		canParse:   true,
	}
	err := registry.Register(parser)
	if err != nil {
		t.Errorf("Register failed: %v", err)
	}

	payload := &RawPayload{
		DeviceEUI: "1486e6546b37cec8",
		FPort:     2,
		Data:      "test",
		Timestamp: time.Now(),
	}

	parsed, err := registry.DetectAndParse(ctx, payload)
	if err != nil {
		t.Errorf("DetectAndParse failed: %v", err)
	}

	if parsed.DeviceType != DeviceTypeRAK2270 {
		t.Errorf("Wrong device type: %s", parsed.DeviceType)
	}
}

func TestRegistry_ParseWithType(t *testing.T) {
	registry := NewRegistry()
	ctx := context.Background()

	parser := &mockParser{
		deviceType: DeviceTypeRAK2270,
		canParse:   true,
	}
	err := registry.Register(parser)
	if err != nil {
		t.Errorf("Register failed: %v", err)
	}

	payload := &RawPayload{
		DeviceEUI: "1486e6546b37cec8",
		FPort:     2,
		Data:      "test",
		Timestamp: time.Now(),
	}

	parsed, err := registry.ParseWithType(ctx, payload, DeviceTypeRAK2270)
	if err != nil {
		t.Errorf("ParseWithType failed: %v", err)
	}

	if parsed.DeviceType != DeviceTypeRAK2270 {
		t.Errorf("Wrong device type: %s", parsed.DeviceType)
	}

	// Try with non-existent type
	_, err = registry.ParseWithType(ctx, payload, DeviceTypeUnknown)
	if err == nil {
		t.Error("Expected error when parsing with non-existent device type")
	}
}

func TestRegistry_ListRegisteredParsers(t *testing.T) {
	registry := NewRegistry()

	parser1 := &mockParser{deviceType: DeviceTypeRAK2270, canParse: true}
	parser2 := &mockParser{deviceType: "DEVICE_TYPE_2", canParse: true}

	err := registry.Register(parser1)
	if err != nil {
		t.Errorf("Register failed: %v", err)
	}
	err = registry.Register(parser2)
	if err != nil {
		t.Errorf("Register failed: %v", err)
	}

	types := registry.ListRegisteredParsers()
	if len(types) != 2 {
		t.Errorf("Expected 2 registered types, got %d", len(types))
	}
}
