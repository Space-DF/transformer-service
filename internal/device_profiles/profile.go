package deviceprofile

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

// Component dispatches raw payloads to the appropriate device Parser.
type Component struct {
	parsers       map[common.DeviceType]common.Parser
	deviceTypes   []common.DeviceType
	manufacturers map[string]string // model → lowercase manufacturer
}

// RegisterParser adds a Parser for the given model/manufacturer pair.
func (c *Component) RegisterParser(model, manufacturer string, parser common.Parser) {
	dt := common.DeviceType(model)
	if _, exists := c.parsers[dt]; !exists {
		c.deviceTypes = append(c.deviceTypes, dt)
	}
	c.parsers[dt] = parser
	c.manufacturers[model] = strings.ToLower(manufacturer)
	log.Printf("🔧 Registered parser for %s (%s)", model, manufacturer)
}

// GetSupportedDevices returns all registered device types.
func (c *Component) GetSupportedDevices() []common.DeviceType {
	return c.deviceTypes
}

// CanHandle reports whether a parser exists for the given device type.
func (c *Component) CanHandle(deviceType common.DeviceType, _ *common.RawPayload) bool {
	_, exists := c.parsers[deviceType]
	return exists
}

// Parse converts a raw payload into structured ParsedData.
func (c *Component) Parse(_ context.Context, deviceType common.DeviceType, payload *common.RawPayload) (*common.ParsedData, error) {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return nil, errNoParser(string(deviceType))
	}
	return parser.ParsePayload(payload)
}

// ParseToEntities converts a raw payload into a set of home-assistant-style entities.
func (c *Component) ParseToEntities(_ context.Context, orgSlug, model string, deviceType common.DeviceType, payload *common.RawPayload, deviceLocation *common.Location) (*common.ParseResult, error) {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return nil, errNoParser(string(deviceType))
	}

	entities, err := parser.ParseToEntities(orgSlug, model, payload, deviceLocation)
	if err != nil {
		return nil, err
	}

	mfr := c.manufacturers[string(deviceType)]
	if mfr == "" {
		mfr = "unknown"
	}

	shortEUI := payload.DeviceEUI
	if len(shortEUI) > 4 {
		shortEUI = shortEUI[len(shortEUI)-4:]
	}

	return &common.ParseResult{
		DeviceEUI: payload.DeviceEUI,
		DeviceInfo: common.CreateDeviceInfo(
			payload.DeviceEUI,
			fmt.Sprintf("%s %s", string(deviceType), shortEUI),
			mfr,
			string(deviceType),
			string(deviceType),
		),
		Entities:  entities,
		Timestamp: payload.Timestamp,
	}, nil
}

// Validate performs basic sanity checks on parsed data.
func (c *Component) Validate(deviceType common.DeviceType, data *common.ParsedData) error {
	if data.DeviceEUI == "" {
		return ErrMissingDevEUI
	}
	if data.DeviceType != deviceType {
		return fmt.Errorf("device type mismatch: expected %s, got %s", deviceType, data.DeviceType)
	}
	return nil
}

// SupportsGPS reports whether the device type has built-in GPS.
func (c *Component) SupportsGPS(deviceType common.DeviceType) bool {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return false
	}
	return parser.SupportsGPS()
}

// GetSupportedPorts returns the fPorts used by a device type.
func (c *Component) GetSupportedPorts(deviceType common.DeviceType) []int {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return nil
	}
	return parser.GetSupportedPorts()
}

// GetSupportedEntityTypes returns the entity types a device type can produce.
func (c *Component) GetSupportedEntityTypes(deviceType common.DeviceType) []string {
	parser, exists := c.parsers[deviceType]
	if !exists {
		return nil
	}
	return parser.GetSupportedEntityTypes()
}

// Sentinel errors returned by the Component.
var (
	ErrNoParser      = errors.New("no parser registered for device type")
	ErrMissingDevEUI = errors.New("device EUI is required")
)

func errNoParser(dt string) error {
	return fmt.Errorf("%w: %s", ErrNoParser, dt)
}
