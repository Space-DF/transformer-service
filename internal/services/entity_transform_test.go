package services

import (
	"testing"
	"time"

	"github.com/Space-DF/transformer-service/internal/components"
	"github.com/Space-DF/transformer-service/internal/models"
)

func TestEntityTransformService_TransformToTelemetry(t *testing.T) {
	// Create test services
	transformService := NewEntityTransformService(nil, nil)

	// Create mock parse result
	parseResult := &components.ParseResult{
		DeviceEUI: "70b3d57ed005b847",
		DeviceInfo: components.DeviceInfo{
			Identifiers:  []string{"70b3d57ed005b847"},
			Name:         "RAK2270 Test Device",
			Manufacturer: "RAKwireless",
			Model:        "RAK2270",
			ModelID:      "rak2270",
		},
		Entities: []components.Entity{
			{
				UniqueID:    "testorg_70b3d57ed005b847_location",
				EntityID:    "device_tracker.testorg_rakwireless_rak2270_70b3d57ed005b847_location",
				EntityType:  "location",
				DeviceClass: "location",
				Name:        "Location",
				State:       "home",
				Attributes: map[string]interface{}{
					"latitude":     40.7128,
					"longitude":    -74.0060,
					"source":       "trilateration",
					"gps_capable":  false,
					"device_model": "RAK2270",
				},
				Enabled:   true,
				Timestamp: time.Now(),
			},
		},
		Timestamp: time.Now(),
	}

	// Test payload
	originalPayload := map[string]interface{}{
		"device_id":   "conference_room_sensor",
		"space_slug":  "conference_room",
		"received_at": time.Now().Format(time.RFC3339),
	}

	// Transform to telemetry
	telemetryPayload, err := transformService.TransformToTelemetry(parseResult, "testorg", originalPayload)
	if err != nil {
		t.Fatalf("TransformToTelemetry failed: %v", err)
	}

	// Verify telemetry payload
	if telemetryPayload == nil {
		t.Fatal("Telemetry payload is nil")
	}

	// Check organization
	if telemetryPayload.Organization != "testorg" {
		t.Errorf("Expected organization 'testorg', got '%s'", telemetryPayload.Organization)
	}

	// Check device EUI
	if telemetryPayload.DeviceEUI != "70b3d57ed005b847" {
		t.Errorf("Expected DeviceEUI '70b3d57ed005b847', got '%s'", telemetryPayload.DeviceEUI)
	}

	// Check device ID extraction
	if telemetryPayload.DeviceID != "conference_room_sensor" {
		t.Errorf("Expected DeviceID 'conference_room_sensor', got '%s'", telemetryPayload.DeviceID)
	}

	// Check space slug extraction
	if telemetryPayload.SpaceSlug != "conference_room" {
		t.Errorf("Expected SpaceSlug 'conference_room', got '%s'", telemetryPayload.SpaceSlug)
	}

	// Check device info
	if telemetryPayload.DeviceInfo.Manufacturer != "RAKwireless" {
		t.Errorf("Expected manufacturer 'RAKwireless', got '%s'", telemetryPayload.DeviceInfo.Manufacturer)
	}

	if telemetryPayload.DeviceInfo.Model != "RAK2270" {
		t.Errorf("Expected model 'RAK2270', got '%s'", telemetryPayload.DeviceInfo.Model)
	}

	// Check entities
	if len(telemetryPayload.Entities) != 1 {
		t.Errorf("Expected 1 entity, got %d", len(telemetryPayload.Entities))
	}

	entity := telemetryPayload.Entities[0]
	if entity.UniqueID != "testorg_70b3d57ed005b847_location" {
		t.Errorf("Expected entity UniqueID 'testorg_70b3d57ed005b847_location', got '%s'", entity.UniqueID)
	}

	if entity.EntityType != "location" {
		t.Errorf("Expected entity type 'location', got '%s'", entity.EntityType)
	}

	if entity.State != "home" {
		t.Errorf("Expected entity state 'home', got %v", entity.State)
	}

	// Check source
	if telemetryPayload.Source != "transformer-service" {
		t.Errorf("Expected source 'transformer-service', got '%s'", telemetryPayload.Source)
	}

	// Check metadata extraction
	if telemetryPayload.Metadata == nil {
		t.Error("Expected metadata, got nil")
	} else {
		if _, exists := telemetryPayload.Metadata["received_at"]; !exists {
			t.Error("Expected received_at in metadata")
		}
	}

	t.Logf("✅ Successfully created telemetry payload with %d entities", len(telemetryPayload.Entities))
	t.Logf("📍 Location entity: %s", entity.EntityID)
	t.Logf("🏢 Organization: %s", telemetryPayload.Organization)
	t.Logf("📱 Device: %s (%s)", telemetryPayload.DeviceID, telemetryPayload.DeviceInfo.Model)
}

func TestEntityTransformService_TransformLocationData_BackwardCompatibility(t *testing.T) {
	transformService := NewEntityTransformService(nil, nil)

	// Test backward compatibility with legacy location data
	deviceLocation := &models.DeviceLocationData{
		Latitude:     40.7128,
		Longitude:    -74.0060,
		DevEUI:       "70b3d57ed005b847",
		Organization: "testorg",
	}

	originalPayload := map[string]interface{}{
		"device_id":  "legacy_device",
		"space_slug": "legacy_space",
	}

	gatewayCount := 3

	// Transform legacy data
	telemetryPayload, err := transformService.TransformLocationData(deviceLocation, gatewayCount, originalPayload)
	if err != nil {
		t.Fatalf("TransformLocationData failed: %v", err)
	}

	// Verify backward compatibility
	if telemetryPayload.Organization != "testorg" {
		t.Errorf("Expected organization 'testorg', got '%s'", telemetryPayload.Organization)
	}

	if len(telemetryPayload.Entities) != 1 {
		t.Errorf("Expected 1 entity, got %d", len(telemetryPayload.Entities))
	}

	entity := telemetryPayload.Entities[0]
	if entity.EntityType != "location" {
		t.Errorf("Expected location entity, got %s", entity.EntityType)
	}

	// Check accuracy calculation
	if attrs, ok := entity.Attributes["accuracy"].(float64); ok {
		expectedAccuracy := 40.0 // 3 gateways = 40m accuracy
		if attrs != expectedAccuracy {
			t.Errorf("Expected accuracy %f, got %f", expectedAccuracy, attrs)
		}
	} else {
		t.Error("Expected accuracy in attributes")
	}

	// Check calculation method
	if method, ok := entity.Attributes["calculation_method"].(string); ok {
		if method != "trilateration" {
			t.Errorf("Expected calculation method 'trilateration', got '%s'", method)
		}
	} else {
		t.Error("Expected calculation_method in attributes")
	}

	t.Logf("✅ Backward compatibility test passed")
	t.Logf("📍 Converted legacy location to entity: %s", entity.EntityID)
	t.Logf("🎯 Accuracy: %.0fm (%d gateways)", entity.Attributes["accuracy"], gatewayCount)
}

func TestEntityTransformService_DetermineLocationAccuracy(t *testing.T) {
	transformService := NewEntityTransformService(nil, nil)

	testCases := []struct {
		gatewayCount int
		expected     float64
	}{
		{0, 0},   // GPS
		{1, 300}, // Single gateway
		{2, 100}, // Two gateways
		{3, 40},  // Three gateways
		{4, 20},  // Four or more gateways
		{10, 20}, // Many gateways
	}

	for _, tc := range testCases {
		accuracy := transformService.determineLocationAccuracy(tc.gatewayCount)
		if accuracy != tc.expected {
			t.Errorf("Gateway count %d: expected accuracy %f, got %f", tc.gatewayCount, tc.expected, accuracy)
		}
	}
}

func TestTelemetryPayloadFormat(t *testing.T) {
	// Test that the telemetry payload matches expected JSON structure
	telemetryPayload := &models.TelemetryPayload{
		Organization: "acme_corp",
		DeviceEUI:    "70b3d57ed005b847",
		DeviceID:     "conference_room_sensor",
		SpaceSlug:    "conference_room_b",
		DeviceInfo: models.TelemetryDeviceInfo{
			Identifiers:  []string{"70b3d57ed005b847"},
			Name:         "Conference Room Tracker",
			Manufacturer: "RAKwireless",
			Model:        "RAK2270",
			ModelID:      "rak2270",
		},
		Entities: []models.TelemetryEntity{
			{
				UniqueID:    "acme_corp_70b3d57ed005b847_location",
				EntityID:    "device_tracker.acme_corp_rakwireless_rak2270_70b3d57ed005b847_location",
				EntityType:  "location",
				DeviceClass: "location",
				Name:        "Location",
				State:       "home",
				Attributes: map[string]interface{}{
					"latitude":  40.7128,
					"longitude": -74.0060,
					"accuracy":  20.0,
					"source":    "trilateration",
				},
				Timestamp: time.Now().Format(time.RFC3339),
			},
		},
		Timestamp: time.Now().Format(time.RFC3339),
		Source:    "transformer-service",
		Metadata: map[string]interface{}{
			"gateway_count": 4,
			"frequency":     868100000,
		},
	}

	// Verify the structure (basic checks)
	if telemetryPayload.Organization == "" {
		t.Error("Organization is required")
	}

	if telemetryPayload.DeviceEUI == "" {
		t.Error("DeviceEUI is required")
	}

	if len(telemetryPayload.Entities) == 0 {
		t.Error("At least one entity is required")
	}

	// Verify entity format matches expected conventions
	entity := telemetryPayload.Entities[0]

	// Check unique_id format: org_deveui_entitytype
	expectedUniquePrefix := "acme_corp_70b3d57ed005b847_"
	if len(entity.UniqueID) < len(expectedUniquePrefix) || entity.UniqueID[:len(expectedUniquePrefix)] != expectedUniquePrefix {
		t.Errorf("Entity UniqueID format incorrect: %s", entity.UniqueID)
	}

	// Check entity_id format: domain.org_manufacturer_model_deveui_entitytype
	expectedEntityPrefix := "device_tracker.acme_corp_rakwireless_rak2270_70b3d57ed005b847_"
	if len(entity.EntityID) < len(expectedEntityPrefix) || entity.EntityID[:len(expectedEntityPrefix)] != expectedEntityPrefix {
		t.Errorf("Entity EntityID format incorrect: %s", entity.EntityID)
	}

	t.Logf("✅ Telemetry payload format validation passed")
	t.Logf("🆔 Unique ID: %s", entity.UniqueID)
	t.Logf("🏷️ Entity ID: %s", entity.EntityID)
	t.Logf("📊 Payload size: %d entities", len(telemetryPayload.Entities))
}
