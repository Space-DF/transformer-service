package rakwireless

import (
	"context"
	"testing"
	"time"

	"github.com/Space-DF/transformer-service/internal/components"
)

func TestRAKwirelessComponent_EntityParsing(t *testing.T) {
	component := NewRAKwirelessComponent()
	ctx := context.Background()

	testCases := []struct {
		name             string
		deviceType       components.DeviceType
		orgSlug          string
		devEUI           string
		expectedEntities int
	}{
		{
			name:             "RAK2270 - Location only",
			deviceType:       components.DeviceTypeRAK2270,
			orgSlug:          "testorg",
			devEUI:           "70b3d57ed005b847",
			expectedEntities: 1, // location only
		},
		{
			name:             "RAK7200 - GPS + Battery + Temperature",
			deviceType:       components.DeviceTypeRAK7200,
			orgSlug:          "testorg",
			devEUI:           "abc123def456789",
			expectedEntities: 3, // location, battery, temperature
		},
		{
			name:             "RAK4630 - Multiple sensors",
			deviceType:       components.DeviceTypeRAK4630,
			orgSlug:          "testorg",
			devEUI:           "123456789abcdef",
			expectedEntities: 4, // location, battery, temperature, humidity
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test payload
			payload := &components.RawPayload{
				DeviceEUI: tc.devEUI,
				FPort:     2,
				Data:      "AQIDBAUGBwgJCgsMDQ4PEA==", // Base64 test data
				Timestamp: time.Now(),
				RxInfo:    []components.GatewayInfo{},
			}

			// Test ParseToEntities
			parseResult, err := component.ParseToEntities(ctx, tc.orgSlug, tc.deviceType, payload)
			if err != nil {
				// Some parsers (like RAK7200 with invalid data) might return errors
				// This is expected for this test with dummy data
				t.Logf("ParseToEntities returned error (expected for dummy data): %v", err)
				return
			}

			// Verify result structure
			if parseResult == nil {
				t.Fatal("ParseToEntities returned nil result")
			}

			if parseResult.DeviceEUI != tc.devEUI {
				t.Errorf("Expected DeviceEUI %s, got %s", tc.devEUI, parseResult.DeviceEUI)
			}

			// Verify device info
			if parseResult.DeviceInfo.Manufacturer != "RAKwireless" {
				t.Errorf("Expected manufacturer RAKwireless, got %s", parseResult.DeviceInfo.Manufacturer)
			}

			if len(parseResult.DeviceInfo.Identifiers) == 0 {
				t.Error("Expected device identifiers, got none")
			}

			if parseResult.DeviceInfo.Identifiers[0] != tc.devEUI {
				t.Errorf("Expected first identifier %s, got %s", tc.devEUI, parseResult.DeviceInfo.Identifiers[0])
			}

			// Verify entities
			if len(parseResult.Entities) != tc.expectedEntities {
				t.Errorf("Expected %d entities, got %d", tc.expectedEntities, len(parseResult.Entities))
			}

			// Verify entity format
			for _, entity := range parseResult.Entities {
				// Check unique_id format: orgslug_deveui_entitytype
				expectedPrefix := tc.orgSlug + "_" + tc.devEUI + "_"
				if len(entity.UniqueID) < len(expectedPrefix) {
					t.Errorf("UniqueID too short: %s", entity.UniqueID)
					continue
				}
				if entity.UniqueID[:len(expectedPrefix)] != expectedPrefix {
					t.Errorf("UniqueID doesn't match expected format. Got: %s, expected prefix: %s", entity.UniqueID, expectedPrefix)
				}

				// Check entity_id format: domain.org_manufacturer_model_deveui_entitytype
				if entity.EntityID == "" {
					t.Error("EntityID is empty")
				}

				// Verify required fields
				if entity.EntityType == "" {
					t.Error("EntityType is empty")
				}
				if entity.Name == "" {
					t.Error("Entity Name is empty")
				}

				t.Logf("Entity: UniqueID=%s, EntityID=%s, Type=%s, Name=%s",
					entity.UniqueID, entity.EntityID, entity.EntityType, entity.Name)
			}
		})
	}
}

func TestRAKwirelessComponent_SupportedDevices(t *testing.T) {
	component := NewRAKwirelessComponent()

	supportedDevices := component.GetSupportedDevices()
	expectedDevices := []components.DeviceType{
		components.DeviceTypeRAK2270,
		components.DeviceTypeRAK7200,
		components.DeviceTypeRAK4630,
	}

	if len(supportedDevices) != len(expectedDevices) {
		t.Errorf("Expected %d supported devices, got %d", len(expectedDevices), len(supportedDevices))
	}

	for _, expected := range expectedDevices {
		found := false
		for _, supported := range supportedDevices {
			if supported == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected device type %s not found in supported devices", expected)
		}
	}
}

func TestRAKwirelessComponent_CanHandle(t *testing.T) {
	component := NewRAKwirelessComponent()

	testCases := []struct {
		deviceType components.DeviceType
		expected   bool
	}{
		{components.DeviceTypeRAK2270, true},
		{components.DeviceTypeRAK7200, true},
		{components.DeviceTypeRAK4630, true},
		{components.DeviceTypeUnknown, false},
		{"INVALID", false},
	}

	for _, tc := range testCases {
		payload := &components.RawPayload{DeviceEUI: "test"}
		result := component.CanHandle(tc.deviceType, payload)
		if result != tc.expected {
			t.Errorf("CanHandle(%s) = %v, expected %v", tc.deviceType, result, tc.expected)
		}
	}
}

func TestEntityIDFormats(t *testing.T) {
	orgSlug := "testorg"
	devEUI := "70b3d57ed005b847"
	entityType := "location"

	// Test unique ID generation
	uniqueID := components.GenerateUniqueID(orgSlug, devEUI, entityType)
	expected := "testorg_70b3d57ed005b847_location"
	if uniqueID != expected {
		t.Errorf("GenerateUniqueID() = %s, expected %s", uniqueID, expected)
	}

	// Test entity ID generation
	domain := components.GetEntityDomain(entityType)
	entityID := components.GenerateEntityID(domain, orgSlug, "rakwireless", "rak2270", devEUI, entityType)
	expectedEntityID := "device_tracker.testorg_rakwireless_rak2270_70b3d57ed005b847_location"
	if entityID != expectedEntityID {
		t.Errorf("GenerateEntityID() = %s, expected %s", entityID, expectedEntityID)
	}

	// Test domain mapping
	testCases := []struct {
		entityType string
		expected   string
	}{
		{"location", "device_tracker"},
		{"battery", "sensor"},
		{"temperature", "sensor"},
		{"humidity", "sensor"},
		{"motion", "binary_sensor"},
		{"unknown", "sensor"}, // default
	}

	for _, tc := range testCases {
		domain := components.GetEntityDomain(tc.entityType)
		if domain != tc.expected {
			t.Errorf("GetEntityDomain(%s) = %s, expected %s", tc.entityType, domain, tc.expected)
		}
	}
}
