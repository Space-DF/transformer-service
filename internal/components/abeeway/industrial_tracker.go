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
		devEUI = components.ExtractDevEUI(payload.Metadata)
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
	case MsgTypePosition:
		// Position message (0x03) - GPS, WiFi, BLE or low power GPS position data
		// Format: Header(5) + Type(1) + Status(1) + [Position data...]
		if len(abeewayPayload.Data) >= 2 {
			posType := abeewayPayload.Data[0]
			posStatus := abeewayPayload.Data[1]

			sensorData["position_type"] = fmt.Sprintf("0x%02X", posType)
			sensorData["position_status"] = posStatus

			// Parse GPS position data if available
			if (posType == 0x00 || posType == 0x01 || posType == 0x04) && len(abeewayPayload.Data) >= 17 {
				// GPS or Low Power GPS position
				// Format: Type(1) + Status(1) + Lat(4) + Lon(4) + Alt(2) + Course(2) + Speed(2) + [Satellites(1)] + [HDOP(1)]
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

					// Check for GPS fix validity (bit 0 of status)
					sensorData["gps_fix_valid"] = (posStatus & 0x01) != 0

					if validateCoordinates(latitude, longitude) == nil {
						location = &components.Location{
							Latitude:  latitude,
							Longitude: longitude,
							Altitude:  float64(alt),
						}
					}
				}

				// Extract satellites if available
				if len(abeewayPayload.Data) >= 18 {
					sensorData["satellites"] = int(abeewayPayload.Data[17])
				}
				// Extract HDOP for accuracy if available
				if len(abeewayPayload.Data) >= 19 {
					hdop := float64(abeewayPayload.Data[18]) / 10.0
					sensorData["hdop"] = hdop
					sensorData["accuracy"] = hdop * 5
				}
			} else if posType == 0x09 && len(abeewayPayload.Data) >= 13 {
				// WiFi fingerprinting position
				// Format: Type(1) + Status(1) + Lat(4) + Lon(4) + Age(2) + NbrBSSID(1) + BSSIDList...
				lat := bigEndianInt32(abeewayPayload.Data[2:6])
				lon := bigEndianInt32(abeewayPayload.Data[6:10])
				age := int(binary.BigEndian.Uint16(abeewayPayload.Data[10:12]))
				nbrBSSID := int(abeewayPayload.Data[12])

				sensorData["position_age"] = age
				sensorData["wifi_bssid_count"] = nbrBSSID

				// Check if WiFi fix is valid
				if (posStatus & 0x01) != 0 && lat != 0 && lon != 0 {
					latitude := float64(lat) / 10000000.0
					longitude := float64(lon) / 10000000.0
					sensorData["latitude"] = latitude
					sensorData["longitude"] = longitude
					sensorData["accuracy"] = 100

					if validateCoordinates(latitude, longitude) == nil {
						location = &components.Location{
							Latitude:  latitude,
							Longitude: longitude,
						}
					}
				}

				// Parse BSSID list (each BSSID is 6 bytes)
				offset := 13
				bssids := make([]string, 0)
				for i := 0; i < nbrBSSID && offset+6 <= len(abeewayPayload.Data); i++ {
					bssid := fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
						abeewayPayload.Data[offset], abeewayPayload.Data[offset+1], abeewayPayload.Data[offset+2],
						abeewayPayload.Data[offset+3], abeewayPayload.Data[offset+4], abeewayPayload.Data[offset+5])
					bssids = append(bssids, bssid)
					offset += 6
				}
				if len(bssids) > 0 {
					sensorData["wifi_bssids"] = bssids
				}
			} else if posType == 0x07 && len(abeewayPayload.Data) >= 13 {
				// BLE beacon position
				// Format: Type(1) + Status(1) + Lat(4) + Lon(4) + Age(2) + NbrBeacons(1) + Beacons...
				lat := bigEndianInt32(abeewayPayload.Data[2:6])
				lon := bigEndianInt32(abeewayPayload.Data[6:10])
				age := int(binary.BigEndian.Uint16(abeewayPayload.Data[10:12]))
				nbrBeacons := int(abeewayPayload.Data[12])

				sensorData["position_age"] = age
				sensorData["ble_beacon_count"] = nbrBeacons

				// Check if BLE fix is valid
				if (posStatus & 0x01) != 0 && lat != 0 && lon != 0 {
					latitude := float64(lat) / 10000000.0
					longitude := float64(lon) / 10000000.0
					sensorData["latitude"] = latitude
					sensorData["longitude"] = longitude
					sensorData["accuracy"] = 50

					if validateCoordinates(latitude, longitude) == nil {
						location = &components.Location{
							Latitude:  latitude,
							Longitude: longitude,
						}
					}
				}

				// Parse beacon list
				offset := 13
				beacons := make([]map[string]interface{}, 0)
				for i := 0; i < nbrBeacons && offset+14 <= len(abeewayPayload.Data); i++ {
					beacon := map[string]interface{}{
						"mac": fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
							abeewayPayload.Data[offset], abeewayPayload.Data[offset+1], abeewayPayload.Data[offset+2],
							abeewayPayload.Data[offset+3], abeewayPayload.Data[offset+4], abeewayPayload.Data[offset+5]),
						"rssi":  int(int8(abeewayPayload.Data[offset+6])),
						"major": int(binary.BigEndian.Uint16(abeewayPayload.Data[offset+7:offset+9])),
						"minor": int(binary.BigEndian.Uint16(abeewayPayload.Data[offset+9:offset+11])),
					}
					beacons = append(beacons, beacon)
					offset += 14
				}
				if len(beacons) > 0 {
					sensorData["ble_beacons"] = beacons
				}
			}
		}

		// Include battery and temp from header
		sensorData["battery_voltage"] = decodeBattery(abeewayPayload.Battery)
		sensorData["battery_percent"] = decodeBatteryPercent(abeewayPayload.Battery)
		sensorData["temperature"] = decodeTemperature(abeewayPayload.Temperature)
		sensorData["status"] = decodeStatus(abeewayPayload.Status)

	case MsgTypeStatus:
		// Status message (0x04) - power and health status of the tracker
		// Format: BatteryMv(2) + Temperature(1) + MainSupply(1) + PowerMode(1)
		energyData, err := parseEnergyStatus(abeewayPayload.Data)
		if err == nil {
			sensorData["battery_voltage"] = energyData.BatteryVoltage
			sensorData["battery_percent"] = energyData.BatteryLevel
			sensorData["temperature"] = energyData.Temperature
			sensorData["main_supply"] = energyData.MainSupply
			sensorData["charging"] = energyData.Charging
			sensorData["power_consumption"] = energyData.PowerConsumption
		}

	case MsgTypeHeartbeat:
		// Heartbeat message (0x05) - notify that tracker is operational
		// Format: Header(5) + ResetCause(1) + FirmwareVer(3)
		sensorData["message_type"] = GetMessageTypeName(abeewayPayload.MessageType)
		sensorData["battery_voltage"] = decodeBattery(abeewayPayload.Battery)
		sensorData["battery_percent"] = decodeBatteryPercent(abeewayPayload.Battery)
		sensorData["temperature"] = decodeTemperature(abeewayPayload.Temperature)
		sensorData["status"] = decodeStatus(abeewayPayload.Status)

		// Parse heartbeat-specific data if available
		if len(abeewayPayload.Data) >= 1 {
			sensorData["reset_cause"] = abeewayPayload.Data[0]
		}
		if len(abeewayPayload.Data) >= 4 {
			sensorData["firmware_version"] = fmt.Sprintf("%d.%d.%d",
				abeewayPayload.Data[1], abeewayPayload.Data[2], abeewayPayload.Data[3])
		}

	case MsgTypeActivityStatus:
		// Activity Status/Config/Shock/BLE MAC address message (0x07)
		// Multiple sub-types share this message type
		sensorData["message_type"] = "Activity Status/Configuration"
		sensorData["battery_voltage"] = decodeBattery(abeewayPayload.Battery)
		sensorData["battery_percent"] = decodeBatteryPercent(abeewayPayload.Battery)
		sensorData["temperature"] = decodeTemperature(abeewayPayload.Temperature)
		sensorData["status"] = decodeStatus(abeewayPayload.Status)

		// Parse activity data if available
		if len(abeewayPayload.Data) >= 4 {
			// Could be activity counter, configuration, shock data, or BLE MAC
			sensorData["activity_data"] = fmt.Sprintf("%x", abeewayPayload.Data[:min(4, len(abeewayPayload.Data))])
		}

	case MsgTypeShutdown:
		// Shutdown message (0x09) - sent when tracker is set off
		sensorData["message_type"] = "Shutdown"
		sensorData["shutdown"] = true
		if len(abeewayPayload.Data) >= 1 {
			sensorData["shutdown_reason"] = abeewayPayload.Data[0]
		}

	case MsgTypeEvent:
		// Event message (0x0A) - sends event information about tracker
		sensorData["message_type"] = "Event"
		if len(abeewayPayload.Data) >= 1 {
			sensorData["event_type"] = abeewayPayload.Data[0]
		}

	case MsgTypeCollectionScan:
		// Collection scan message (0x0B) - WIFI or BLE collection scan data
		sensorData["message_type"] = "Collection Scan"

	case MsgTypeExtendedPosition:
		// Extended Position message (0x0E) - GPS, WiFi, or BLE position
		sensorData["message_type"] = "Extended Position"
		// Parse as regular position for now
		posData, err := parsePositionData(abeewayPayload.Data)
		if err == nil && posData != nil {
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
				}
			}
		}

	case MsgTypeFramePending:
		// Frame pending message (0x00) - trigger sending
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
		devEUI = components.ExtractDevEUI(payload.Metadata)
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
		Icon:        "mdi:battery",
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
		Icon:        "mdi:battery",
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
		Icon:        "mdi:thermometer",
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
		Icon:        "mdi:information",
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
			Icon:        "mdi:alert-circle",
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
			Icon:        "mdi:run-fast",
			Attributes: map[string]interface{}{
				"device_model": "Abeeway Industrial Tracker",
			},
			Enabled:   true,
			Timestamp: timestamp,
		})
	}

	// Parse message-specific data for entities
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
				Icon:        "mdi:crosshairs-gps",
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
						Icon:        "mdi:map-marker",
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
					Icon:        "mdi:speedometer",
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
					Icon:        "mdi:compass",
					DisplayType: []string{"value"},
					Attributes: map[string]interface{}{
						"device_model": "Abeeway Industrial Tracker",
					},
					Enabled:   true,
					Timestamp: timestamp,
				})
			}
		}

	case MsgTypeStatus:
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
				Icon:        "mdi:power",
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
				Icon:        "mdi:battery-charging",
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
		MessageType: MsgTypePosition, // Default message type for position data
	}

	// Extract GPS data
	if gps, ok := decoded["gps"].(map[string]interface{}); ok {
		// For pre-decoded GPS data, we need to add type/status prefix bytes
		// to match the PositioningStatus format: Type(1) + Status(1) + Lat(4) + Lon(4) + Alt(2) + Course(2) + Speed(2)
		// Use GPS position type (0x01) and valid fix status (0x01)
		result.Data = append(result.Data, 0x01, 0x01) // Type + Status prefix (2 bytes)

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
		// Pad with zeros for course (2 bytes) + speed (2 bytes) to reach 16+ bytes total
		result.Data = append(result.Data, 0x00, 0x00, 0x00, 0x00)
		// Ensure we have at least 17 bytes by adding one more byte if needed
		if len(result.Data) < 17 {
			result.Data = append(result.Data, 0x00)
		}
	}

	// Extract battery voltage
	if battV, ok := decoded["battery_voltage"].(float64); ok {
		// Convert voltage back to battery code
		result.Battery = byte((battV - 2.0) / 0.01)
		if result.Battery > 255 {
			result.Battery = 255
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
		// Convert temp back to temperature code
		result.Temperature = byte(temp * 2.0)
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
