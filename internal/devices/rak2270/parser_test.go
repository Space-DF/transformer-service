package rak2270

import (
	"context"
	"testing"
	"time"

	"github.com/Space-DF/transformer-service/internal/devices"
)

func TestNewParser(t *testing.T) {
	parser := NewParser()
	if parser == nil {
		t.Fatal("NewParser returned nil")
	}

	if parser.GetDeviceType() != devices.DeviceTypeRAK2270 {
		t.Errorf("Expected device type %s, got %s",
			devices.DeviceTypeRAK2270, parser.GetDeviceType())
	}

	metadata := parser.GetMetadata()
	if metadata.Manufacturer != "RAKwireless" {
		t.Errorf("Expected manufacturer RAKwireless, got %s", metadata.Manufacturer)
	}
}

// TODO: Add test cases for CanParse
func TestParser_CanParse(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name     string
		payload  *devices.RawPayload
		expected bool
	}{
		{
			name: "valid RAK2270 payload",
			payload: &devices.RawPayload{
				DeviceEUI: "1486e6546b37cec8",
				FPort:     2,
				Data:      "v2R0eXBlAmJpZBh7Y2ZtdHAxOTA2...", // truncated for example
				Timestamp: time.Now(),
			},
			expected: false, // TODO: Will be true when CanParse is implemented
		},
		{
			name: "invalid fPort",
			payload: &devices.RawPayload{
				DeviceEUI: "1486e6546b37cec8",
				FPort:     10,
				Data:      "invalid",
				Timestamp: time.Now(),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.CanParse(tt.payload)
			if result != tt.expected {
				t.Errorf("CanParse() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TODO: Add test cases for Parse with real RAK2270 payloads
func TestParser_Parse(t *testing.T) {
	parser := NewParser()
	ctx := context.Background()

	// TODO: Use actual RAK2270 payload from your sample data
	payload := &devices.RawPayload{
		DeviceEUI: "1486e6546b37cec8",
		FPort:     2,
		Data:      "v2R0eXBlAmJpZBh7Y2ZtdHAxOTA2LDEyMDM0LDEyNTAwZnNlbnNvcnhXKiwqLCosKiwqLCosKiwqLCosKiwqLCosKiwqLDE2LjA1NDUzMTEsMTA4LjIyMDI5MTEsMC4wNjkwMDAwLDcwODIsMjg0LDAwLDAsMTAwLDQuMDc2MTMy/w==",
		Timestamp: time.Now(),
	}

	parsed, err := parser.Parse(ctx, payload)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if parsed.DeviceEUI != payload.DeviceEUI {
		t.Errorf("DeviceEUI = %s, expected %s", parsed.DeviceEUI, payload.DeviceEUI)
	}

	if parsed.DeviceType != devices.DeviceTypeRAK2270 {
		t.Errorf("DeviceType = %s, expected %s", parsed.DeviceType, devices.DeviceTypeRAK2270)
	}

	// TODO: Add assertions for parsed location, sensor data, battery level
	// Example:
	// if parsed.Location == nil {
	//     t.Error("Expected location to be parsed")
	// }
	// if parsed.Location.Latitude < -90 || parsed.Location.Latitude > 90 {
	//     t.Errorf("Invalid latitude: %f", parsed.Location.Latitude)
	// }
}

// TODO: Add test cases for Validate
func TestParser_Validate(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name    string
		data    *devices.ParsedData
		wantErr bool
	}{
		{
			name: "valid data",
			data: &devices.ParsedData{
				DeviceEUI:  "1486e6546b37cec8",
				DeviceType: devices.DeviceTypeRAK2270,
				Timestamp:  time.Now(),
				Location: &devices.Location{
					Latitude:  16.05453,
					Longitude: 108.22029,
				},
				SensorData: map[string]interface{}{
					"temperature": 22.5,
				},
			},
			wantErr: false,
		},
		{
			name: "wrong device type",
			data: &devices.ParsedData{
				DeviceEUI:  "1486e6546b37cec8",
				DeviceType: devices.DeviceTypeUnknown,
				Timestamp:  time.Now(),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parser.Validate(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
