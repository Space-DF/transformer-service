package rak2270

import (
	"context"
	"fmt"

	"github.com/Space-DF/transformer-service/internal/devices"
)

// Parser implements the DeviceParser interface for RAK2270 Sticker Tracker
type Parser struct {
	metadata devices.ParserMetadata
}

// NewParser creates a new RAK2270 parser instance
func NewParser() *Parser {
	return &Parser{
		metadata: devices.ParserMetadata{
			DeviceType:     devices.DeviceTypeRAK2270,
			Manufacturer:   "RAKwireless",
			Model:          "RAK2270",
			Version:        "1.0.0",
			Description:    "RAK2270 Sticker Tracker - LoRaWAN GPS tracker with sensors",
			SupportedPorts: []int{2}, // RAK2270 typically uses fPort 2
		},
	}
}

// GetDeviceType returns the device type this parser handles
func (p *Parser) GetDeviceType() devices.DeviceType {
	return devices.DeviceTypeRAK2270
}

// GetMetadata returns parser metadata
func (p *Parser) GetMetadata() devices.ParserMetadata {
	return p.metadata
}

// CanParse checks if this parser can handle the given payload
// TODO: Implement device detection logic
func (p *Parser) CanParse(payload *devices.RawPayload) bool {

	return false
}

// Parse converts raw payload into structured ParsedData
// TODO: Implement actual parsing logic for RAK2270 payload format
func (p *Parser) Parse(ctx context.Context, payload *devices.RawPayload) (*devices.ParsedData, error) {
	// TODO: Implement RAK2270-specific payload extraction
	// The data is already decoded by ChirpStack - just extract from payload structure:
	// - GPS coordinates from payload.RxInfo[0].Location

	parsedData := &devices.ParsedData{
		DeviceEUI:  payload.DeviceEUI,
		DeviceType: devices.DeviceTypeRAK2270,
		Timestamp:  payload.Timestamp,
		SensorData: make(map[string]interface{}),
		RawData:    payload.Data,
	}

	// TODO: Extract location from gateway info
	// if len(payload.RxInfo) > 0 {
	//     parsedData.Location = &devices.Location{
	//         Latitude:  payload.RxInfo[0].Location.Latitude,
	//         Longitude: payload.RxInfo[0].Location.Longitude,
	//         Altitude:  payload.RxInfo[0].Location.Altitude,
	//     }
	// }

	return parsedData, nil
}

// Validate performs device-specific validation on the parsed data
func (p *Parser) Validate(data *devices.ParsedData) error {
	if data.DeviceType != devices.DeviceTypeRAK2270 {
		return fmt.Errorf("invalid device type: expected %s, got %s",
			devices.DeviceTypeRAK2270, data.DeviceType)
	}

	// TODO: Add RAK2270-specific validations:

	return nil
}
