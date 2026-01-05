package abeeway

import (
	"fmt"

	"github.com/Space-DF/transformer-service/internal/components"
)

// IndustrialTrackerParser handles parsing of Abeeway Industrial Tracker payloads
type IndustrialTrackerParser struct{}

// NewIndustrialTrackerParser creates a new Industrial Tracker parser
func NewIndustrialTrackerParser() *IndustrialTrackerParser {
	return &IndustrialTrackerParser{}
}

// ParsePayload parses Industrial Tracker device payload
func (p *IndustrialTrackerParser) ParsePayload(payload *components.RawPayload) (*components.ParsedData, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = extractDevEUI(payload.Metadata)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI not found")
	}

	// Decode payload bytes
	encoded := extractPayloadData(payload.Data)
	if encoded == "" {
		encoded = extractPayloadData(payload.Metadata)
	}
	if encoded == "" {
		return nil, fmt.Errorf("no payload data found")
	}

	bytes, err := decodePayloadBytes(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	// Parse Abeeway header
	abeewayPayload, err := parseAbeewayHeader(bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Abeeway header: %w", err)
	}

	sensorData := make(map[string]interface{})
	var location *components.Location

	// Parse based on message type
	switch abeewayPayload.MessageType {
	case MsgTypeHeartbeat:
		// Heartbeat message - basic status
		sensorData["message_type"] = GetMessageTypeName(abeewayPayload.MessageType)
		sensorData["battery_voltage"] = decodeBattery(abeewayPayload.Battery)
		sensorData["battery_percent"] = decodeBatteryPercent(abeewayPayload.Battery)
		sensorData["temperature"] = decodeTemperature(abeewayPayload.Temperature)
		sensorData["status"] = decodeStatus(abeewayPayload.Status)

	case MsgTypePosition:
		// Position message - contains GPS/WiFi/BLE data
		posData, err := parsePositionData(abeewayPayload.Data)
		if err == nil && posData != nil {
			sensorData["position_type"] = posData.Type
			sensorData["position_source"] = posData.Type

			if posData.Latitude != 0 || posData.Longitude != 0 {
				if validateCoordinates(posData.Latitude, posData.Longitude) == nil {
					location = &components.Location{
						Latitude:  posData.Latitude,
						Longitude: posData.Longitude,
						Altitude:  posData.Altitude,
					}
					sensorData["latitude"] = posData.Latitude
					sensorData["longitude"] = posData.Longitude
					sensorData["altitude"] = posData.Altitude
					sensorData["accuracy"] = posData.Accuracy
					sensorData["speed"] = posData.Speed
					sensorData["heading"] = posData.Heading
					sensorData["satellites"] = posData.Satellites
				}
			}

			if len(posData.BSSIDList) > 0 {
				sensorData["wifi_bssids"] = posData.BSSIDList
			}

			if len(posData.BLEData) > 0 {
				sensorData["ble_beacons"] = posData.BLEData
			}
		}

		// Also include battery and temp from header
		sensorData["battery_voltage"] = decodeBattery(abeewayPayload.Battery)
		sensorData["battery_percent"] = decodeBatteryPercent(abeewayPayload.Battery)
		sensorData["temperature"] = decodeTemperature(abeewayPayload.Temperature)
		sensorData["status"] = decodeStatus(abeewayPayload.Status)

	case MsgTypeEnergyStatus:
		// Energy status message - detailed battery info
		energyData, err := parseEnergyStatus(abeewayPayload.Data)
		if err == nil {
			sensorData["battery_voltage"] = energyData.BatteryVoltage
			sensorData["battery_percent"] = energyData.BatteryLevel
			sensorData["temperature"] = energyData.Temperature
			sensorData["main_supply"] = energyData.MainSupply
			sensorData["charging"] = energyData.Charging
			sensorData["power_consumption"] = energyData.PowerConsumption
		}

	case MsgTypeActivityConfig:
		// Activity status or configuration message
		// Differentiate by checking data length
		if len(abeewayPayload.Data) >= 4 {
			// Could be activity counter or configuration
			sensorData["message_type"] = "Activity/Configuration"
			sensorData["battery_voltage"] = decodeBattery(abeewayPayload.Battery)
			sensorData["battery_percent"] = decodeBatteryPercent(abeewayPayload.Battery)
			sensorData["temperature"] = decodeTemperature(abeewayPayload.Temperature)
			sensorData["status"] = decodeStatus(abeewayPayload.Status)
		}

	case MsgTypeShutdown:
		sensorData["message_type"] = "Shutdown"
		sensorData["shutdown_reason"] = fmt.Sprintf("0x%02X", abeewayPayload.Data)

	case MsgTypeGeolocStart:
		sensorData["message_type"] = "Geolocation Start"

	case MsgTypeFramePending:
		sensorData["message_type"] = "Frame Pending"

	default:
		sensorData["message_type"] = GetMessageTypeName(abeewayPayload.MessageType)
	}

	// Always include battery and temp from header if not already set
	if _, exists := sensorData["battery_voltage"]; !exists {
		sensorData["battery_voltage"] = decodeBattery(abeewayPayload.Battery)
		sensorData["battery_percent"] = decodeBatteryPercent(abeewayPayload.Battery)
	}
	if _, exists := sensorData["temperature"]; !exists {
		sensorData["temperature"] = decodeTemperature(abeewayPayload.Temperature)
	}

	return &components.ParsedData{
		DeviceEUI:    devEUI,
		DeviceType:   components.DeviceTypeAbeewayIndustrialTracker,
		Timestamp:    payload.Timestamp,
		Location:     location,
		SensorData:   sensorData,
		BatteryLevel: sensorData["battery_percent"].(*float64),
		RawData:      encoded,
	}, nil
}

// SupportsGPS returns true since Industrial Tracker has GPS
func (p *IndustrialTrackerParser) SupportsGPS() bool {
	return true
}

// GetSupportedPorts returns the fPorts typically used by Industrial Tracker
func (p *IndustrialTrackerParser) GetSupportedPorts() []int {
	return []int{1, 2, 5, 17, 100}
}

// GetSupportedEntityTypes returns entity types supported by Industrial Tracker
func (p *IndustrialTrackerParser) GetSupportedEntityTypes() []string {
	return []string{"location", "battery", "temperature", "status", "signal"}
}

// ParseToEntities creates entities for Industrial Tracker device
func (p *IndustrialTrackerParser) ParseToEntities(orgSlug, model string, payload *components.RawPayload) ([]components.Entity, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = extractDevEUI(payload.Metadata)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI is required")
	}

	// Decode payload bytes
	encoded := extractPayloadData(payload.Data)
	if encoded == "" {
		encoded = extractPayloadData(payload.Metadata)
	}
	if encoded == "" {
		return nil, fmt.Errorf("no payload data found")
	}

	bytes, err := decodePayloadBytes(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	// Parse Abeeway header
	abeewayPayload, err := parseAbeewayHeader(bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Abeeway header: %w", err)
	}

	var entities []components.Entity
	timestamp := payload.Timestamp
	modelID := "abeeway_industrial_tracker"

	// Decode common values
	batteryVoltage := decodeBattery(abeewayPayload.Battery)
	batteryPercent := decodeBatteryPercent(abeewayPayload.Battery)
	temperature := decodeTemperature(abeewayPayload.Temperature)
	status := decodeStatus(abeewayPayload.Status)

	// Battery Entity (voltage)
	entities = append(entities, components.Entity{
		UniqueID: components.GenerateUniqueID(model, devEUI, "battery_voltage"),
		EntityID: components.GenerateEntityID(
			components.GetEntityDomain("battery"),
			orgSlug, "abeeway", modelID, devEUI, "battery_voltage",
		),
		EntityType:  "battery",
		DeviceClass: "battery",
		Name:        "Battery Voltage",
		State:       batteryVoltage,
		UnitOfMeas:  "V",
		DisplayType: []string{"chart", "gauge", "value"},
		Attributes: map[string]interface{}{
			"device_model": "Abeeway Industrial Tracker",
		},
		Enabled:   true,
		Timestamp: timestamp,
	})

	// Battery Entity (percentage)
	entities = append(entities, components.Entity{
		UniqueID: components.GenerateUniqueID(model, devEUI, "battery_level"),
		EntityID: components.GenerateEntityID(
			components.GetEntityDomain("battery"),
			orgSlug, "abeeway", modelID, devEUI, "battery_level",
		),
		EntityType:  "battery",
		DeviceClass: "battery",
		Name:        "Battery Level",
		State:       batteryPercent,
		UnitOfMeas:  "%",
		DisplayType: []string{"chart", "gauge", "value", "slider"},
		Attributes: map[string]interface{}{
			"device_model": "Abeeway Industrial Tracker",
		},
		Enabled:   true,
		Timestamp: timestamp,
	})

	// Temperature Entity
	entities = append(entities, components.Entity{
		UniqueID: components.GenerateUniqueID(model, devEUI, "temperature"),
		EntityID: components.GenerateEntityID(
			components.GetEntityDomain("temperature"),
			orgSlug, "abeeway", modelID, devEUI, "temperature",
		),
		EntityType:  "temperature",
		DeviceClass: "temperature",
		Name:        "Temperature",
		State:       temperature,
		UnitOfMeas:  "°C",
		DisplayType: []string{"chart", "gauge", "value"},
		Attributes: map[string]interface{}{
			"device_model": "Abeeway Industrial Tracker",
			"sensor_type":  "internal",
		},
		Enabled:   true,
		Timestamp: timestamp,
	})

	// Status Entity
	statusAttrs := make(map[string]interface{})
	statusAttrs["device_model"] = "Abeeway Industrial Tracker"
	statusAttrs["message_type"] = GetMessageTypeName(abeewayPayload.MessageType)
	for k, v := range status {
		statusAttrs[k] = v
	}

	entities = append(entities, components.Entity{
		UniqueID: components.GenerateUniqueID(model, devEUI, "status"),
		EntityID: components.GenerateEntityID(
			"sensor",
			orgSlug, "abeeway", modelID, devEUI, "status",
		),
		EntityType:  "sensor",
		DeviceClass: "status",
		Name:        "Status",
		State:       GetMessageTypeName(abeewayPayload.MessageType),
		Attributes:  statusAttrs,
		Enabled:     true,
		Timestamp:   timestamp,
	})

	// SOS Alert Entity (binary sensor)
	if sosActive, ok := status["sos_bit"].(bool); ok {
		entities = append(entities, components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "sos_alert"),
			EntityID: components.GenerateEntityID(
				"binary_sensor",
				orgSlug, "abeeway", modelID, devEUI, "sos_alert",
			),
			EntityType:  "binary_sensor",
			DeviceClass: "safety",
			Name:        "SOS Alert",
			State:       sosActive,
			Attributes: map[string]interface{}{
				"device_model": "Abeeway Industrial Tracker",
			},
			Enabled:   true,
			Timestamp: timestamp,
		})
	}

	// Motion Entity (binary sensor)
	if isMoving, ok := status["tracker_is_moving_bit"].(bool); ok {
		entities = append(entities, components.Entity{
			UniqueID: components.GenerateUniqueID(model, devEUI, "motion"),
			EntityID: components.GenerateEntityID(
				"binary_sensor",
				orgSlug, "abeeway", modelID, devEUI, "motion",
			),
			EntityType:  "binary_sensor",
			DeviceClass: "motion",
			Name:        "Motion",
			State:       isMoving,
			Attributes: map[string]interface{}{
				"device_model": "Abeeway Industrial Tracker",
			},
			Enabled:   true,
			Timestamp: timestamp,
		})
	}

	// Parse message-specific data
	switch abeewayPayload.MessageType {
	case MsgTypePosition:
		posData, err := parsePositionData(abeewayPayload.Data)
		if err == nil && posData != nil {
			// Position Type Entity
			entities = append(entities, components.Entity{
				UniqueID: components.GenerateUniqueID(model, devEUI, "position_type"),
				EntityID: components.GenerateEntityID(
					"sensor",
					orgSlug, "abeeway", modelID, devEUI, "position_type",
				),
				EntityType:  "sensor",
				DeviceClass: "position_type",
				Name:        "Position Type",
				State:       posData.Type,
				Attributes: map[string]interface{}{
					"device_model": "Abeeway Industrial Tracker",
					"age_seconds":  posData.Age,
				},
				Enabled:   true,
				Timestamp: timestamp,
			})

			// Location Entity (if we have valid coordinates)
			if posData.Latitude != 0 || posData.Longitude != 0 {
				if validateCoordinates(posData.Latitude, posData.Longitude) == nil {
					locationAttrs := map[string]interface{}{
						"source":       posData.Type,
						"gps_capable":  true,
						"device_model": "Abeeway Industrial Tracker",
						"latitude":     posData.Latitude,
						"longitude":    posData.Longitude,
						"altitude":     posData.Altitude,
						"accuracy":     posData.Accuracy,
					}

					if posData.Satellites > 0 {
						locationAttrs["satellites"] = posData.Satellites
					}
					if posData.Speed > 0 {
						locationAttrs["speed"] = posData.Speed
					}
					if posData.Heading > 0 {
						locationAttrs["heading"] = posData.Heading
					}
					if len(posData.BSSIDList) > 0 {
						locationAttrs["wifi_bssids"] = posData.BSSIDList
					}
					if len(posData.BLEData) > 0 {
						locationAttrs["ble_beacons"] = posData.BLEData
					}

					entities = append(entities, components.Entity{
						UniqueID: components.GenerateUniqueID(model, devEUI, "location"),
						EntityID: components.GenerateEntityID(
							components.GetEntityDomain("location"),
							orgSlug, "abeeway", modelID, devEUI, "location",
						),
						EntityType:  "location",
						DeviceClass: "location",
						Name:        "Location",
						State:       "home", // Using "home" as default for Home Assistant compatibility
						DisplayType: []string{"map"},
						Attributes:  locationAttrs,
						Enabled:     true,
						Timestamp:   timestamp,
					})
				}
			}

			// Speed Entity (if available)
			if posData.Speed > 0 {
				entities = append(entities, components.Entity{
					UniqueID: components.GenerateUniqueID(model, devEUI, "speed"),
					EntityID: components.GenerateEntityID(
						"sensor",
						orgSlug, "abeeway", modelID, devEUI, "speed",
					),
					EntityType:  "sensor",
					DeviceClass: "speed",
					Name:        "Speed",
					State:       posData.Speed,
					UnitOfMeas:  "km/h",
					DisplayType: []string{"gauge", "value"},
					Attributes: map[string]interface{}{
						"device_model": "Abeeway Industrial Tracker",
					},
					Enabled:   true,
					Timestamp: timestamp,
				})
			}

			// Heading Entity (if available)
			if posData.Heading > 0 {
				entities = append(entities, components.Entity{
					UniqueID: components.GenerateUniqueID(model, devEUI, "heading"),
					EntityID: components.GenerateEntityID(
						"sensor",
						orgSlug, "abeeway", modelID, devEUI, "heading",
					),
					EntityType:  "sensor",
					DeviceClass: "heading",
					Name:        "Heading",
					State:       posData.Heading,
					UnitOfMeas:  "deg",
					DisplayType: []string{"value"},
					Attributes: map[string]interface{}{
						"device_model": "Abeeway Industrial Tracker",
					},
					Enabled:   true,
					Timestamp: timestamp,
				})
			}
		}

	case MsgTypeEnergyStatus:
		energyData, err := parseEnergyStatus(abeewayPayload.Data)
		if err == nil {
			// Main Supply Entity
			entities = append(entities, components.Entity{
				UniqueID: components.GenerateUniqueID(model, devEUI, "main_supply"),
				EntityID: components.GenerateEntityID(
					"binary_sensor",
					orgSlug, "abeeway", modelID, devEUI, "main_supply",
				),
				EntityType:  "binary_sensor",
				DeviceClass: "power",
				Name:        "Main Supply",
				State:       energyData.MainSupply,
				Attributes: map[string]interface{}{
					"device_model": "Abeeway Industrial Tracker",
				},
				Enabled:   true,
				Timestamp: timestamp,
			})

			// Charging Entity
			entities = append(entities, components.Entity{
				UniqueID: components.GenerateUniqueID(model, devEUI, "charging"),
				EntityID: components.GenerateEntityID(
					"binary_sensor",
					orgSlug, "abeeway", modelID, devEUI, "charging",
				),
				EntityType:  "binary_sensor",
				DeviceClass: "battery_charging",
				Name:        "Charging",
				State:       energyData.Charging,
				Attributes: map[string]interface{}{
					"device_model": "Abeeway Industrial Tracker",
				},
				Enabled:   true,
				Timestamp: timestamp,
			})
		}
	}

	return entities, nil
}
