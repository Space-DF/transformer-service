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
		if len(abeewayPayload.Data) >= 1 {
			posType := abeewayPayload.Data[0]
			sensorData["position_type"] = fmt.Sprintf("0x%02X", posType)

			// Check for compact 9-byte GPS format: Type(1) + Lat(4) + Lon(4)
			// This format is used by some Abeeway firmwares
			if (posType == 0x00 || posType == 0x01 || posType == 0x04) && len(abeewayPayload.Data) >= 9 {
				// - Compact: Type(1) + Lat(4) + Lon(4) = 9 bytes
				// - Full: Type(1) + Status(1) + Lat(4) + Lon(4) + ... = 17+ bytes

				var lat, lon int32
				var latOffset, lonOffset int

				if len(abeewayPayload.Data) >= 17 {
					// Full format with Status byte
					posStatus := abeewayPayload.Data[1]
					sensorData["position_status"] = posStatus
					sensorData["gps_fix_valid"] = (posStatus & 0x01) != 0
					latOffset = 2
					lonOffset = 6
				} else {
					latOffset = 1
					lonOffset = 5
				}

				lat = bigEndianInt32(abeewayPayload.Data[latOffset : latOffset+4])
				lon = bigEndianInt32(abeewayPayload.Data[lonOffset : lonOffset+4])

				latitude := float64(lat) / components.CoordScale
				longitude := float64(lon) / components.CoordScale

				sensorData["latitude"] = latitude
				sensorData["longitude"] = longitude

				// Parse additional fields if available
				if len(abeewayPayload.Data) >= 17 {
					alt := bigEndianInt16(abeewayPayload.Data[10:12])
					course := binary.BigEndian.Uint16(abeewayPayload.Data[12:14])
					speed := binary.BigEndian.Uint16(abeewayPayload.Data[14:16])

					sensorData["altitude"] = float64(alt)
					sensorData["speed"] = float64(speed)
					sensorData["heading"] = float64(course)
				}

				// Set location if coordinates are valid
				if latitude != 0 || longitude != 0 {
					if validateCoordinates(latitude, longitude) == nil {
						location = &components.Location{
							Latitude:  latitude,
							Longitude: longitude,
						}
						if len(abeewayPayload.Data) >= 17 {
							location.Altitude = float64(bigEndianInt16(abeewayPayload.Data[10:12]))
						}
					}
				}
			} else if (posType == 0x00 || posType == 0x01 || posType == 0x04) && len(abeewayPayload.Data) >= 17 {
				// Original full format
				posStatus := abeewayPayload.Data[1]
				sensorData["position_status"] = posStatus
				sensorData["gps_fix_valid"] = (posStatus & 0x01) != 0
			} else if posType == 0x09 && len(abeewayPayload.Data) >= 13 {
				// WiFi fingerprinting position
				// Format: Type(1) + Status(1) + Lat(4) + Lon(4) + Age(2) + NbrBSSID(1) + BSSIDList...
				posStatus := abeewayPayload.Data[1]
				sensorData["position_status"] = posStatus
				lat := bigEndianInt32(abeewayPayload.Data[2:6])
				lon := bigEndianInt32(abeewayPayload.Data[6:10])
				age := int(binary.BigEndian.Uint16(abeewayPayload.Data[10:12]))
				nbrBSSID := int(abeewayPayload.Data[12])

				sensorData["position_age"] = age
				sensorData["wifi_bssid_count"] = nbrBSSID

				// Check if WiFi fix is valid
				if (posStatus&0x01) != 0 && lat != 0 && lon != 0 {
					latitude := float64(lat) / components.CoordScale
					longitude := float64(lon) / components.CoordScale
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
				posStatus := abeewayPayload.Data[1]
				sensorData["position_status"] = posStatus
				lat := bigEndianInt32(abeewayPayload.Data[2:6])
				lon := bigEndianInt32(abeewayPayload.Data[6:10])
				age := int(binary.BigEndian.Uint16(abeewayPayload.Data[10:12]))
				nbrBeacons := int(abeewayPayload.Data[12])

				sensorData["position_age"] = age
				sensorData["ble_beacon_count"] = nbrBeacons

				// Check if BLE fix is valid
				if (posStatus&0x01) != 0 && lat != 0 && lon != 0 {
					latitude := float64(lat) / components.CoordScale
					longitude := float64(lon) / components.CoordScale
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
						"rssi":  int(int8(abeewayPayload.Data[offset+6])), //#nosec G115
						"major": int(binary.BigEndian.Uint16(abeewayPayload.Data[offset+7 : offset+9])),
						"minor": int(binary.BigEndian.Uint16(abeewayPayload.Data[offset+9 : offset+11])),
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
		DeviceType:   DeviceTypeAbeewayIndustrialTracker,
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

	// Determine message type from decoded payload
	msgType := MsgTypePosition // Default
	if mt, ok := decoded["message_type"].(string); ok {
		switch mt {
		case "position":
			msgType = MsgTypePosition
		case "status":
			msgType = MsgTypeStatus
		case "heartbeat":
			msgType = MsgTypeHeartbeat
		case "activity_status":
			msgType = MsgTypeActivityStatus
		case "shutdown":
			msgType = MsgTypeShutdown
		case "event":
			msgType = MsgTypeEvent
		}
	}

	result := &AbeewayPayload{
		MessageType: byte(msgType),
	}

	// Extract GPS data
	var lat, lon, alt float64
	var hasGPS bool

	// Try nested "gps" object first
	if gps, ok := decoded["gps"].(map[string]interface{}); ok {
		if l, ok := gps["latitude"].(float64); ok {
			lat = l
			hasGPS = true
		}
		if l, ok := gps["longitude"].(float64); ok {
			lon = l
			hasGPS = true
		}
		if a, ok := gps["altitude"].(float64); ok {
			alt = a
		}
	}

	if hasGPS && (lat != 0 || lon != 0) {
		// Build a proper 17-byte position data buffer matching the format expected by parsePositionData:
		// Type(1) + Status(1) + Lat(4) + Lon(4) + Alt(2) + Course(2) + Speed(2) + Satellites(1)
		data := make([]byte, 17)

		// Position type
		data[0] = 0x01 // GPS type

		// Status: bit 0 = valid fix
		data[1] = 0x01

		// Latitude (int32, scaled by 1e7)
		latInt := int32(lat * components.CoordScale)
		data[2] = byte(latInt >> 24) //#nosec G115
		data[3] = byte(latInt >> 16) //#nosec G115
		data[4] = byte(latInt >> 8)  //#nosec G115
		data[5] = byte(latInt)       //#nosec G115

		// Longitude (int32, scaled by 1e7)
		lonInt := int32(lon * components.CoordScale)
		data[6] = byte(lonInt >> 24) //#nosec G115
		data[7] = byte(lonInt >> 16) //#nosec G115
		data[8] = byte(lonInt >> 8)  //#nosec G115
		data[9] = byte(lonInt)       //#nosec G115

		// Altitude (int16, meters)
		altInt := int16(alt)
		data[10] = byte(altInt >> 8) //#nosec G115
		data[11] = byte(altInt)      //#nosec G115

		// Course/heading (uint16, degrees)
		if heading, ok := decoded["heading"].(float64); ok {
			headingInt := uint16(heading)
			data[12] = byte(headingInt >> 8)
			data[13] = byte(headingInt) //#nosec G115
		}

		// Speed (uint16)
		if speed, ok := decoded["speed"].(float64); ok {
			speedInt := uint16(speed)
			data[14] = byte(speedInt >> 8)
			data[15] = byte(speedInt) //#nosec G115
		}

		// Satellites (uint8)
		if sats, ok := decoded["satellites"].(float64); ok {
			data[16] = byte(sats)
		}

		result.Data = data
	}

	// Extract battery voltage
	if battV, ok := decoded["battery_voltage"].(float64); ok {
		battCode := int((battV*1000.0 - 2000.0) / 10.0)
		if battCode < 0 {
			battCode = 0
		}
		if battCode > 255 {
			battCode = 255
		}
		result.Battery = byte(battCode)
	}

	// Extract battery percent (fallback if no voltage)
	if battPct, ok := decoded["battery_percent"].(float64); ok {
		if result.Battery == 0 {
			// Approximate: map 0-100% to 3.0V-4.2V range, then encode
			voltage := 3.0 + (battPct/100.0)*1.2
			battCode := int((voltage*1000.0 - 2000.0) / 10.0)
			if battCode < 0 {
				battCode = 0
			}
			if battCode > 255 {
				battCode = 255
			}
			result.Battery = byte(battCode)
		}
	}

	// Extract temperature
	if temp, ok := decoded["temperature"].(float64); ok {
		result.Temperature = byte(temp * 2.0)
	}

	// Extract operating mode into status byte
	if mode, ok := decoded["operating_mode"].(float64); ok {
		result.Status = byte(mode)
	}

	return result, nil
}

// parseFromRawData extracts Abeeway data from raw binary payload
func (p *IndustrialTrackerParser) parseFromRawData(payload *components.RawPayload) (*AbeewayPayload, error) {
	// Decode payload bytes
	encoded := components.ExtractPayloadData(payload.Data)
	if encoded == "" {
		encoded = components.ExtractPayloadData(payload.Metadata)
	}
	if encoded == "" {
		return nil, fmt.Errorf("no payload data found")
	}

	bytes, err := components.DecodePayloadBytes(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	// Parse Abeeway header
	return parseAbeewayHeader(bytes)
}
