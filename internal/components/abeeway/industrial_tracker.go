package abeeway

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/Space-DF/transformer-service/internal/components"
)

const coordScaleIT = 1e7

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

				latitude := float64(lat) / coordScaleIT
				longitude := float64(lon) / coordScaleIT

				if latitude != 0 || longitude != 0 {
					sensorData["altitude"] = float64(alt)
					sensorData["speed"] = float64(speed)
					sensorData["heading"] = float64(course)

					// Check for GPS fix validity (bit 0 of status)
					sensorData["gps_fix_valid"] = (posStatus & 0x01) != 0

					if validateCoordinates(latitude, longitude) == nil {
						sensorData["latitude"] = latitude
						sensorData["longitude"] = longitude
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
				if (posStatus&0x01) != 0 && lat != 0 && lon != 0 {
					latitude := float64(lat) / coordScaleIT
					longitude := float64(lon) / coordScaleIT
					sensorData["accuracy"] = 100

					if validateCoordinates(latitude, longitude) == nil {
						sensorData["latitude"] = latitude
						sensorData["longitude"] = longitude
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
				if (posStatus&0x01) != 0 && lat != 0 && lon != 0 {
					latitude := float64(lat) / coordScaleIT
					longitude := float64(lon) / coordScaleIT
					sensorData["accuracy"] = 50

					if validateCoordinates(latitude, longitude) == nil {
						sensorData["latitude"] = latitude
						sensorData["longitude"] = longitude
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
		// Status message (0x04)
		// In AT2 v2.5, battery/temp are present in the common header.
		// The ACK low nibble identifies status subtype (0: basic, 1: health payload).
		statusType := abeewayPayload.Ack & 0x0F
		sensorData["message_type"] = GetMessageTypeName(abeewayPayload.MessageType)
		sensorData["status_type"] = int(statusType)
		sensorData["battery_voltage"] = decodeBattery(abeewayPayload.Battery)
		sensorData["battery_percent"] = decodeBatteryPercent(abeewayPayload.Battery)
		sensorData["temperature"] = decodeTemperature(abeewayPayload.Temperature)
		sensorData["status"] = decodeStatus(abeewayPayload.Status)

		// Health status subtype payload:
		// TotalPower(2) + MaxTemp(1) + MinTemp(1) + LoRa(2) + BLE(2) + GPS(2) + WiFi(2) + BattMv(2)
		if statusType == 0x01 && len(abeewayPayload.Data) >= 14 {
			totalPower := int(binary.BigEndian.Uint16(abeewayPayload.Data[0:2]))
			maxTemp := decodeTemperature(abeewayPayload.Data[2])
			minTemp := decodeTemperature(abeewayPayload.Data[3])
			loraPower := int(binary.BigEndian.Uint16(abeewayPayload.Data[4:6]))
			blePower := int(binary.BigEndian.Uint16(abeewayPayload.Data[6:8]))
			gpsPower := int(binary.BigEndian.Uint16(abeewayPayload.Data[8:10]))
			wifiPower := int(binary.BigEndian.Uint16(abeewayPayload.Data[10:12]))
			battMV := int(binary.BigEndian.Uint16(abeewayPayload.Data[12:14]))

			sensorData["total_power"] = totalPower
			sensorData["max_temperature"] = maxTemp
			sensorData["min_temperature"] = minTemp
			sensorData["lora_power"] = loraPower
			sensorData["ble_power"] = blePower
			sensorData["gps_power"] = gpsPower
			sensorData["wifi_power"] = wifiPower
			sensorData["battery_mv"] = battMV
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
		// AT2 v2.5 status messages do not expose legacy main-supply/charging flags
		// in the current simulator payload format, so no extra binary entities are created here.
	}

	return entities, nil
}

// parseFromDecodedPayload extracts Abeeway data from pre-decoded payload metadata
func (p *IndustrialTrackerParser) parseFromDecodedPayload(metadata map[string]interface{}) (*AbeewayPayload, error) {
	var decoded map[string]interface{}

	// Try common decoded payload paths across TTN/ChirpStack/custom wrappers.
	if d, ok := metadata["decoded_payload"].(map[string]interface{}); ok {
		decoded = d
	} else if um, ok := metadata["uplink_message"].(map[string]interface{}); ok {
		if d, ok := um["decoded_payload"].(map[string]interface{}); ok {
			decoded = d
		}
	} else if d, ok := metadata["object"].(map[string]interface{}); ok {
		decoded = d
	} else if d, ok := metadata["decoded_raw_data"].(map[string]interface{}); ok {
		if dp, ok := d["decoded_payload"].(map[string]interface{}); ok {
			decoded = dp
		} else if um, ok := d["uplink_message"].(map[string]interface{}); ok {
			if dp, ok := um["decoded_payload"].(map[string]interface{}); ok {
				decoded = dp
			}
		} else {
			decoded = d
		}
	}

	if decoded == nil {
		return nil, fmt.Errorf("no decoded_payload found")
	}

	result := &AbeewayPayload{
		MessageType: MsgTypePosition,
	}

	// Map simulator decoded message_type to Abeeway message type.
	if msg, ok := decoded["message_type"].(string); ok {
		switch msg {
		case "frame_pending":
			result.MessageType = MsgTypeFramePending
		case "position":
			result.MessageType = MsgTypePosition
		case "status", "energy_status":
			result.MessageType = MsgTypeStatus
		case "heartbeat":
			result.MessageType = MsgTypeHeartbeat
		case "activity":
			result.MessageType = MsgTypeActivityStatus
		case "shutdown":
			result.MessageType = MsgTypeShutdown
		case "event":
			result.MessageType = MsgTypeEvent
		case "collection_scan":
			result.MessageType = MsgTypeCollectionScan
		case "extended_position":
			result.MessageType = MsgTypeExtendedPosition
		case "debug":
			result.MessageType = MsgTypeDebug
		}
	}

	// Build position data from flat decoded fields used by nextjs-simulator.
	lat, hasLat := decoded["latitude"].(float64)
	lon, hasLon := decoded["longitude"].(float64)
	if (result.MessageType == MsgTypePosition || result.MessageType == MsgTypeExtendedPosition) && hasLat && hasLon {
		posType := byte(0x00)
		if src, ok := decoded["position_source"].(string); ok {
			switch src {
			case "wifi":
				posType = 0x09
			case "ble":
				posType = 0x07
			case "low_power":
				posType = 0x04
			default:
				posType = 0x00
			}
		}

		// Type + valid status
		result.Data = append(result.Data, posType, 0x01)

		latInt := int32(lat * 10000000)
		lonInt := int32(lon * 10000000)
		result.Data = append(result.Data, byte(latInt>>24), byte(latInt>>16), byte(latInt>>8), byte(latInt))
		result.Data = append(result.Data, byte(lonInt>>24), byte(lonInt>>16), byte(lonInt>>8), byte(lonInt))

		altVal := 0.0
		if alt, ok := decoded["altitude"].(float64); ok {
			altVal = alt
		}
		altInt := int16(altVal)
		result.Data = append(result.Data, byte(altInt>>8), byte(altInt))

		courseVal := 0.0
		if heading, ok := decoded["heading"].(float64); ok {
			courseVal = heading
		}
		courseInt := uint16(courseVal)
		result.Data = append(result.Data, byte(courseInt>>8), byte(courseInt))

		speedVal := 0.0
		if speed, ok := decoded["speed"].(float64); ok {
			speedVal = speed
		}
		speedInt := uint16(speedVal)
		result.Data = append(result.Data, byte(speedInt>>8), byte(speedInt))

		if sats, ok := decoded["satellites"].(float64); ok {
			result.Data = append(result.Data, byte(sats))
		} else {
			result.Data = append(result.Data, 0x00)
		}
	}

	// Backward compatible extraction for nested decoded.gps object.
	if gps, ok := decoded["gps"].(map[string]interface{}); ok && len(result.Data) == 0 {
		result.MessageType = MsgTypePosition
		result.Data = append(result.Data, 0x01, 0x01)
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
		result.Data = append(result.Data, 0x00, 0x00, 0x00, 0x00)
		if len(result.Data) < 17 {
			result.Data = append(result.Data, 0x00)
		}
	}

	// Extract battery voltage
	if battV, ok := decoded["battery_voltage"].(float64); ok {
		raw := int(math.Round((battV - 2.8) / 0.0055))
		if raw < 0 {
			raw = 0
		}
		if raw > 255 {
			raw = 255
		}
		result.Battery = byte(raw)
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
		raw := int(math.Round((temp + 44.0) * 2.0))
		if raw < 0 {
			raw = 0
		}
		if raw > 255 {
			raw = 255
		}
		result.Temperature = byte(raw)
	}

	// Extract speed (only if position buffer hasn't been constructed yet)
	if speed, ok := decoded["speed"].(float64); ok && len(result.Data) == 0 {
		result.Data = append(result.Data, byte(speed))
	}

	// Extract heading (only if position buffer hasn't been constructed yet)
	if heading, ok := decoded["heading"].(float64); ok && len(result.Data) == 0 {
		headingInt := uint16(heading)
		result.Data = append(result.Data, byte(headingInt>>8), byte(headingInt))
	}

	// Extract satellites (only if position buffer hasn't been constructed yet)
	if sats, ok := decoded["satellites"].(float64); ok && len(result.Data) == 0 {
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
	parsed, err := parseAbeewayHeader(bytes)
	if err != nil {
		return nil, err
	}

	// Compatibility: nextjs-simulator encodes GPS fix as compact Position payload:
	// MsgType(1) + Status(1) + Battery(1) + Temp(1) + AckOpt(1) + Age(1) + Lat24(3) + Lon24(3) + EHPE(1) + Res(2)
	// Backend parser expects PositioningStatus-like Data:
	// Type(1) + Status(1) + Lat32(4) + Lon32(4) + Alt(2) + Course(2) + Speed(2) + [Sat(1)] + [HDOP(1)]
	if (parsed.MessageType == MsgTypePosition || parsed.MessageType == MsgTypeExtendedPosition) && len(parsed.Data) == 10 {
		posType := parsed.Ack & 0x0F
		if posType == 0x00 {
			decodeSigned24 := func(b0, b1, b2 byte) int32 {
				v := int32(b0)<<16 | int32(b1)<<8 | int32(b2)
				if v&0x800000 != 0 {
					v |= ^int32(0xFFFFFF)
				}
				return v
			}

			latRaw24 := decodeSigned24(parsed.Data[1], parsed.Data[2], parsed.Data[3])
			lonRaw24 := decodeSigned24(parsed.Data[4], parsed.Data[5], parsed.Data[6])

			lat32 := latRaw24 << 8
			lon32 := lonRaw24 << 8

			legacy := make([]byte, 0, 19)
			legacy = append(legacy, posType, 0x01)
			legacy = append(legacy, byte(lat32>>24), byte(lat32>>16), byte(lat32>>8), byte(lat32))
			legacy = append(legacy, byte(lon32>>24), byte(lon32>>16), byte(lon32>>8), byte(lon32))
			// Altitude + Course + Speed (unknown in compact payload -> 0)
			course := uint16(0)
			speed := uint16(0)
			// Extended position includes speed and heading bytes in compact format.
			if parsed.MessageType == MsgTypeExtendedPosition {
				speed = uint16(parsed.Data[8])
				course = uint16(math.Round(float64(parsed.Data[9]) * 360.0 / 255.0))
			}
			legacy = append(legacy, 0x00, 0x00, byte(course>>8), byte(course), byte(speed>>8), byte(speed))
			// Satellites + HDOP placeholders
			legacy = append(legacy, 0x00, parsed.Data[7])
			parsed.Data = legacy
		}
	}

	return parsed, nil
}
