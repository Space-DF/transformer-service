package abeeway

import (
	"encoding/binary"
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

	var abeewayPayload *AbeewayPayload
	var err error

	// Try to parse from decoded_payload first (if available)
	abeewayPayload, err = p.parseFromDecodedPayload(payload.Metadata)
	if err != nil {
		// Fallback to parsing from raw data
		abeewayPayload, err = p.parseFromRawData(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Abeeway data: %w", err)
		}
	}

	sensorData := make(map[string]interface{})
	var location *components.Location

	// Parse based on message type
	switch abeewayPayload.MessageType {
	case MsgTypePositioningStatus:
		// Positioning Status message (0x01) - contains GPS data
		// Data format: Type(1) + Status(1) + Lat(4) + Lon(4) + Alt(2) + Course(2) + Speed(2)
		if len(abeewayPayload.Data) >= 2 {
			posType := abeewayPayload.Data[0]
			status := abeewayPayload.Data[1]

			sensorData["positioning_status_type"] = fmt.Sprintf("0x%02X", posType)
			sensorData["positioning_status"] = status

			// Parse GPS coordinates if data is long enough (min 17 bytes after type/status)
			if len(abeewayPayload.Data) >= 17 {
				lat := bigEndianInt32(abeewayPayload.Data[2:6])
				lon := bigEndianInt32(abeewayPayload.Data[6:10])
				alt := bigEndianInt16(abeewayPayload.Data[10:12])
				course := binary.BigEndian.Uint16(abeewayPayload.Data[12:14])
				speed := binary.BigEndian.Uint16(abeewayPayload.Data[14:16])

				latitude := float64(lat) / 10000000.0
				longitude := float64(lon) / 10000000.0

				// Only set location if we have valid coordinates (not 0,0)
				if latitude != 0 || longitude != 0 {
					sensorData["latitude"] = latitude
					sensorData["longitude"] = longitude
					sensorData["altitude"] = float64(alt)
					sensorData["speed"] = float64(speed)
					sensorData["heading"] = float64(course)

					if validateCoordinates(latitude, longitude) == nil {
						location = &components.Location{
							Latitude:  latitude,
							Longitude: longitude,
							Altitude:  float64(alt),
						}
					}
				}

				sensorData["gps_fix_valid"] = (status & 0x01) != 0
			}
		}

		// Include battery and temp from header
		sensorData["battery_voltage"] = decodeBattery(abeewayPayload.Battery)
		sensorData["battery_percent"] = decodeBatteryPercent(abeewayPayload.Battery)
		sensorData["temperature"] = decodeTemperature(abeewayPayload.Temperature)
		sensorData["status"] = decodeStatus(abeewayPayload.Status)

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

	// Get battery level as pointer
	var batteryLevelPtr *float64
	if bp, ok := sensorData["battery_percent"].(float64); ok {
		batteryLevelPtr = &bp
	}

	return &components.ParsedData{
		DeviceEUI:    devEUI,
		DeviceType:   components.DeviceTypeAbeewayIndustrialTracker,
		Timestamp:    payload.Timestamp,
		Location:     location,
		SensorData:   sensorData,
		BatteryLevel: batteryLevelPtr,
		RawData:      payload.Data,
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
func (p *IndustrialTrackerParser) ParseToEntities(orgSlug, model string, payload *components.RawPayload, deviceLocation *components.Location) ([]components.Entity, error) {
	devEUI := payload.DeviceEUI
	if devEUI == "" {
		devEUI = extractDevEUI(payload.Metadata)
	}
	if devEUI == "" {
		return nil, fmt.Errorf("device EUI is required")
	}

	var abeewayPayload *AbeewayPayload
	var err error

	// Try to parse from decoded_payload first (if available)
	abeewayPayload, err = p.parseFromDecodedPayload(payload.Metadata)
	if err != nil {
		// Fallback to parsing from raw data
		abeewayPayload, err = p.parseFromRawData(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Abeeway data: %w", err)
		}
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
	case MsgTypePositioningStatus:
		// Positioning Status message (0x01) - parse GPS data directly
		if len(abeewayPayload.Data) >= 17 {
			posType := abeewayPayload.Data[0]
			posStatus := abeewayPayload.Data[1]

			lat := bigEndianInt32(abeewayPayload.Data[2:6])
			lon := bigEndianInt32(abeewayPayload.Data[6:10])
			alt := bigEndianInt16(abeewayPayload.Data[10:12])
			course := binary.BigEndian.Uint16(abeewayPayload.Data[12:14])
			speed := binary.BigEndian.Uint16(abeewayPayload.Data[14:16])

			latitude := float64(lat) / 10000000.0
			longitude := float64(lon) / 10000000.0

			// Positioning Status Type Entity
			entities = append(entities, components.Entity{
				UniqueID: components.GenerateUniqueID(model, devEUI, "positioning_status_type"),
				EntityID: components.GenerateEntityID(
					"sensor",
					orgSlug, "abeeway", modelID, devEUI, "positioning_status_type",
				),
				EntityType:  "sensor",
				DeviceClass: "position_type",
				Name:        "Positioning Status Type",
				State:       fmt.Sprintf("0x%02X", posType),
				Attributes: map[string]interface{}{
					"device_model": "Abeeway Industrial Tracker",
					"gps_fix_valid": (posStatus & 0x01) != 0,
				},
				Enabled:   true,
				Timestamp: timestamp,
			})

			// Location Entity (if we have valid coordinates)
			if latitude != 0 || longitude != 0 {
				if validateCoordinates(latitude, longitude) == nil {
					locationAttrs := map[string]interface{}{
						"source":       "positioning_status",
						"gps_capable":  true,
						"device_model": "Abeeway Industrial Tracker",
						"latitude":     latitude,
						"longitude":    longitude,
						"altitude":     float64(alt),
						"gps_fix_valid": (posStatus & 0x01) != 0,
					}

					if course > 0 {
						locationAttrs["heading"] = float64(course)
					}
					if speed > 0 {
						locationAttrs["speed"] = float64(speed)
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
						State:       "home",
						DisplayType: []string{"map"},
						Attributes:  locationAttrs,
						Enabled:     true,
						Timestamp:   timestamp,
					})
				}
			}

			// Speed Entity (if available)
			if speed > 0 {
				entities = append(entities, components.Entity{
					UniqueID: components.GenerateUniqueID(model, devEUI, "speed"),
					EntityID: components.GenerateEntityID(
						"sensor",
						orgSlug, "abeeway", modelID, devEUI, "speed",
					),
					EntityType:  "sensor",
					DeviceClass: "speed",
					Name:        "Speed",
					State:       float64(speed),
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
			if course > 0 {
				entities = append(entities, components.Entity{
					UniqueID: components.GenerateUniqueID(model, devEUI, "heading"),
					EntityID: components.GenerateEntityID(
						"sensor",
						orgSlug, "abeeway", modelID, devEUI, "heading",
					),
					EntityType:  "sensor",
					DeviceClass: "heading",
					Name:        "Heading",
					State:       float64(course),
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

// extractDevEUI extracts DevEUI from various payload formats
func extractDevEUI(payload map[string]interface{}) string {
	// Try TTN format: end_device_ids.dev_eui
	if endDeviceIDs, ok := payload["end_device_ids"].(map[string]interface{}); ok {
		if devEUI, ok := endDeviceIDs["dev_eui"].(string); ok {
			return devEUI
		}
	}

	// Try top-level dev_eui (snake_case)
	if devEUI, ok := payload["dev_eui"].(string); ok {
		return devEUI
	}

	// Try top-level devEui (camelCase)
	if devEUI, ok := payload["devEui"].(string); ok {
		return devEUI
	}

	// Try ChirpStack format: deviceInfo.devEui
	if deviceInfo, ok := payload["deviceInfo"].(map[string]interface{}); ok {
		if devEUI, ok := deviceInfo["devEui"].(string); ok {
			return devEUI
		}
	}

	return ""
}

// parseFromDecodedPayload extracts Abeeway data from pre-decoded payload metadata
func (p *IndustrialTrackerParser) parseFromDecodedPayload(metadata map[string]interface{}) (*AbeewayPayload, error) {
	var decoded map[string]interface{}

	// Try decoded_payload at top level first
	if d, ok := metadata["decoded_payload"].(map[string]interface{}); ok {
		decoded = d
	} else if d, ok := metadata["decoded_raw_data"].(map[string]interface{}); ok {
		// Then try decoded_raw_data.decoded_payload
		if dp, ok := d["decoded_payload"].(map[string]interface{}); ok {
			decoded = dp
		} else {
			decoded = d
		}
	} else {
		return nil, fmt.Errorf("no decoded_payload found")
	}

	result := &AbeewayPayload{
		MessageType: MsgTypePositioningStatus, // Default message type
	}

	// Extract GPS data
	if gps, ok := decoded["gps"].(map[string]interface{}); ok {
		if lat, ok := gps["latitude"].(float64); ok {
			latInt := int32(lat * 10000000)
			result.Data = append(result.Data, byte(latInt>>24), byte(latInt>>16), byte(latInt>>8), byte(latInt))
		}
		if lon, ok := gps["longitude"].(float64); ok {
			lonInt := int32(lon * 10000000)
			result.Data = append(result.Data, byte(lonInt>>24), byte(lonInt>>16), byte(lonInt>>8), byte(lonInt))
		}
		if alt, ok := gps["altitude"].(float64); ok {
			altInt := uint16(alt)
			result.Data = append(result.Data, byte(altInt>>8), byte(altInt))
		}
	}

	// Extract battery voltage
	if battV, ok := decoded["battery_voltage"].(float64); ok {
		// Convert voltage back to battery code (simplified)
		result.Battery = byte((battV - 2.0) / 0.1)
		if result.Battery > 100 {
			result.Battery = 100
		}
	}

	// Extract battery percent
	if battPct, ok := decoded["battery_percent"].(float64); ok {
		if result.Battery == 0 {
			result.Battery = byte(battPct)
		}
	}

	// Extract temperature
	if temp, ok := decoded["temperature"].(float64); ok {
		// Convert temp back to temperature code (simplified)
		result.Temperature = byte((temp + 20) * 2)
	}

	// Extract speed
	if speed, ok := decoded["speed"].(float64); ok {
		result.Data = append(result.Data, byte(speed))
	}

	// Extract heading
	if heading, ok := decoded["heading"].(float64); ok {
		headingInt := uint16(heading)
		result.Data = append(result.Data, byte(headingInt>>8), byte(headingInt))
	}

	// Extract satellites
	if sats, ok := decoded["satellites"].(float64); ok {
		result.Data = append(result.Data, byte(sats))
	}

	return result, nil
}

// parseFromRawData extracts Abeeway data from raw binary payload
func (p *IndustrialTrackerParser) parseFromRawData(payload *components.RawPayload) (*AbeewayPayload, error) {
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
	return parseAbeewayHeader(bytes)
}
