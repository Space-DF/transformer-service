package devices

import (
	"encoding/json"
	"log"
	"strings"
)

// IdentifierDetector handles automatic detection of device identifiers from message payloads
type IdentifierDetector struct{}

// NewIdentifierDetector creates a new identifier detector
func NewIdentifierDetector() *IdentifierDetector {
	return &IdentifierDetector{}
}

// DetectIdentifiers analyzes a message payload and extracts all possible device identifiers
func (d *IdentifierDetector) DetectIdentifiers(payload, locationPayload map[string]interface{}) []DeviceIdentifier {
	var identifiers []DeviceIdentifier

	// LoRaWAN Detection (highest priority - existing system)
	if lorawan := d.detectLoRaWAN(payload, locationPayload); lorawan != nil {
		identifiers = append(identifiers, *lorawan)
	}

	// Satellite Detection (ESN-based systems)
	if satellite := d.detectSatellite(payload, locationPayload); satellite != nil {
		identifiers = append(identifiers, *satellite)
	}

	// Network Detection (MAC, IP-based)
	if network := d.detectNetwork(payload, locationPayload); network != nil {
		identifiers = append(identifiers, network...)
	}

	// Hardware Detection (Serial numbers, etc.)
	if hardware := d.detectHardware(payload, locationPayload); hardware != nil {
		identifiers = append(identifiers, hardware...)
	}

	// Custom Protocol Detection
	if custom := d.detectCustom(payload, locationPayload); custom != nil {
		identifiers = append(identifiers, custom...)
	}

	return identifiers
}

// detectLoRaWAN detects LoRaWAN DevEUI (reusing existing logic)
func (d *IdentifierDetector) detectLoRaWAN(payload, locationPayload map[string]interface{}) *DeviceIdentifier {
	var devEUI string

	// Try standard LoRaWAN structure (existing logic)
	if endDeviceIDs, ok := payload["end_device_ids"].(map[string]interface{}); ok {
		if eui, ok := endDeviceIDs["dev_eui"].(string); ok {
			devEUI = eui
		}
	}

	// Try alternative locations
	if devEUI == "" {
		if eui, ok := locationPayload["dev_eui"].(string); ok {
			devEUI = eui
		} else if eui, ok := payload["dev_eui"].(string); ok {
			devEUI = eui
		} else if eui, ok := locationPayload["devEui"].(string); ok {
			devEUI = eui
		} else if deviceInfo, ok := locationPayload["deviceInfo"].(map[string]interface{}); ok {
			if eui, ok := deviceInfo["devEui"].(string); ok {
				devEUI = eui
			}
		}
	}

	if devEUI != "" && d.isValidDevEUI(devEUI) {
		return &DeviceIdentifier{
			Type:  "lorawan",
			Key:   "dev_eui",
			Value: devEUI,
		}
	}

	return nil
}

// detectSatellite detects satellite ESN identifiers
func (d *IdentifierDetector) detectSatellite(payload, locationPayload map[string]interface{}) *DeviceIdentifier {
	var esn string

	// Check common satellite ESN fields
	if esn, ok := payload["esn"].(string); ok && esn != "" {
		// Direct ESN field
	} else if esn, ok := payload["device_esn"].(string); ok && esn != "" {
		// Device ESN field
	} else if deviceInfo, ok := payload["device_info"].(map[string]interface{}); ok {
		if esn, ok = deviceInfo["esn"].(string); ok && esn != "" {
			// Nested device info ESN
		}
	} else if satelliteInfo, ok := payload["satellite"].(map[string]interface{}); ok {
		if esn, ok = satelliteInfo["esn"].(string); ok && esn != "" {
			// Satellite-specific ESN
		}
	} else if iridium, ok := payload["iridium"].(map[string]interface{}); ok {
		if esn, ok = iridium["esn"].(string); ok && esn != "" {
			// Iridium satellite ESN
		}
	}

	// Check locationPayload as well
	if esn == "" {
		if esn, ok := locationPayload["esn"].(string); ok && esn != "" {
			// ESN in location payload
		} else if deviceInfo, ok := locationPayload["deviceInfo"].(map[string]interface{}); ok {
			if esn, ok = deviceInfo["esn"].(string); ok && esn != "" {
				// Nested ESN in location payload
			}
		}
	}

	if esn != "" && d.isValidESN(esn) {
		return &DeviceIdentifier{
			Type:  "satellite",
			Key:   "esn",
			Value: esn,
		}
	}

	return nil
}

// detectNetwork detects network-based identifiers (MAC, IP, etc.)
func (d *IdentifierDetector) detectNetwork(payload, locationPayload map[string]interface{}) []DeviceIdentifier {
	var identifiers []DeviceIdentifier

	// MAC address detection
	if mac := d.extractMAC(payload, locationPayload); mac != "" {
		identifiers = append(identifiers, DeviceIdentifier{
			Type:  "network",
			Key:   "mac",
			Value: mac,
		})
	}

	// IP address detection
	if ip := d.extractIP(payload, locationPayload); ip != "" {
		identifiers = append(identifiers, DeviceIdentifier{
			Type:  "network", 
			Key:   "ip",
			Value: ip,
		})
	}

	// WiFi-specific identifiers
	if wifi := d.extractWiFi(payload, locationPayload); len(wifi) > 0 {
		identifiers = append(identifiers, wifi...)
	}

	return identifiers
}

// detectHardware detects hardware serial numbers and device-specific IDs
func (d *IdentifierDetector) detectHardware(payload, locationPayload map[string]interface{}) []DeviceIdentifier {
	var identifiers []DeviceIdentifier

	// Serial number detection
	if serial := d.extractSerial(payload, locationPayload); serial != "" {
		identifiers = append(identifiers, DeviceIdentifier{
			Type:  "hardware",
			Key:   "serial",
			Value: serial,
		})
	}

	// IMEI detection (cellular devices)
	if imei := d.extractIMEI(payload, locationPayload); imei != "" {
		identifiers = append(identifiers, DeviceIdentifier{
			Type:  "cellular",
			Key:   "imei",
			Value: imei,
		})
	}

	// Bluetooth detection
	if bluetooth := d.extractBluetooth(payload, locationPayload); bluetooth != "" {
		identifiers = append(identifiers, DeviceIdentifier{
			Type:  "bluetooth",
			Key:   "address",
			Value: bluetooth,
		})
	}

	return identifiers
}

// detectCustom detects custom protocol identifiers
func (d *IdentifierDetector) detectCustom(payload, locationPayload map[string]interface{}) []DeviceIdentifier {
	var identifiers []DeviceIdentifier

	// Detect based on message structure patterns
	if d.isZigBeeMessage(payload) {
		if zigbeeID := d.extractZigBeeID(payload, locationPayload); zigbeeID != "" {
			identifiers = append(identifiers, DeviceIdentifier{
				Type:  "zigbee",
				Key:   "ieee_address",
				Value: zigbeeID,
			})
		}
	}

	// Add more custom protocol detection here
	// if d.isModbusMessage(payload) { ... }
	// if d.isMQTTMessage(payload) { ... }

	return identifiers
}

// Helper methods for identifier extraction

func (d *IdentifierDetector) extractMAC(payload, locationPayload map[string]interface{}) string {
	// Try various MAC address field names
	macFields := []string{"mac", "mac_address", "macAddress", "hw_addr", "ethernet_mac"}
	
	for _, field := range macFields {
		if mac, ok := payload[field].(string); ok && d.isValidMAC(mac) {
			return mac
		}
		if mac, ok := locationPayload[field].(string); ok && d.isValidMAC(mac) {
			return mac
		}
	}

	// Check nested structures
	if netInfo, ok := payload["network_info"].(map[string]interface{}); ok {
		for _, field := range macFields {
			if mac, ok := netInfo[field].(string); ok && d.isValidMAC(mac) {
				return mac
			}
		}
	}

	return ""
}

func (d *IdentifierDetector) extractIP(payload, locationPayload map[string]interface{}) string {
	ipFields := []string{"ip", "ip_address", "ipAddress", "device_ip", "client_ip"}
	
	for _, field := range ipFields {
		if ip, ok := payload[field].(string); ok && d.isValidIP(ip) {
			return ip
		}
		if ip, ok := locationPayload[field].(string); ok && d.isValidIP(ip) {
			return ip
		}
	}

	return ""
}

func (d *IdentifierDetector) extractSerial(payload, locationPayload map[string]interface{}) string {
	serialFields := []string{"serial", "serial_number", "serialNumber", "device_serial", "hw_serial"}
	
	for _, field := range serialFields {
		if serial, ok := payload[field].(string); ok && serial != "" {
			return serial
		}
		if serial, ok := locationPayload[field].(string); ok && serial != "" {
			return serial
		}
	}

	return ""
}

func (d *IdentifierDetector) extractIMEI(payload, locationPayload map[string]interface{}) string {
	imeiFields := []string{"imei", "device_imei", "cellular_imei"}
	
	for _, field := range imeiFields {
		if imei, ok := payload[field].(string); ok && d.isValidIMEI(imei) {
			return imei
		}
		if imei, ok := locationPayload[field].(string); ok && d.isValidIMEI(imei) {
			return imei
		}
	}

	return ""
}

func (d *IdentifierDetector) extractBluetooth(payload, locationPayload map[string]interface{}) string {
	btFields := []string{"bluetooth", "bt_address", "ble_address", "bluetooth_mac"}
	
	for _, field := range btFields {
		if bt, ok := payload[field].(string); ok && d.isValidMAC(bt) {
			return bt
		}
		if bt, ok := locationPayload[field].(string); ok && d.isValidMAC(bt) {
			return bt
		}
	}

	return ""
}

func (d *IdentifierDetector) extractWiFi(payload, locationPayload map[string]interface{}) []DeviceIdentifier {
	var identifiers []DeviceIdentifier

	// WiFi MAC
	if wifi, ok := payload["wifi"].(map[string]interface{}); ok {
		if mac, ok := wifi["mac"].(string); ok && d.isValidMAC(mac) {
			identifiers = append(identifiers, DeviceIdentifier{
				Type:  "wifi",
				Key:   "mac",
				Value: mac,
			})
		}
		// WiFi SSID for identification
		if ssid, ok := wifi["ssid"].(string); ok && ssid != "" {
			identifiers = append(identifiers, DeviceIdentifier{
				Type:  "wifi",
				Key:   "ssid",
				Value: ssid,
			})
		}
	}

	return identifiers
}

func (d *IdentifierDetector) extractZigBeeID(payload, locationPayload map[string]interface{}) string {
	if zigbee, ok := payload["zigbee"].(map[string]interface{}); ok {
		if id, ok := zigbee["ieee_address"].(string); ok {
			return id
		}
	}
	return ""
}

// Protocol detection methods

func (d *IdentifierDetector) isZigBeeMessage(payload map[string]interface{}) bool {
	_, hasZigbee := payload["zigbee"]
	_, hasIEEE := payload["ieee_address"]
	return hasZigbee || hasIEEE
}

// Validation methods

func (d *IdentifierDetector) isValidDevEUI(devEUI string) bool {
	// DevEUI should be 16 hex characters
	if len(devEUI) != 16 {
		return false
	}
	for _, c := range devEUI {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func (d *IdentifierDetector) isValidESN(esn string) bool {
	// ESN formats can vary, basic validation
	if len(esn) < 8 || len(esn) > 20 {
		return false
	}
	// Check if it starts with common ESN prefixes
	upper := strings.ToUpper(esn)
	return strings.HasPrefix(upper, "ESN") || strings.HasPrefix(upper, "300") || 
		   (len(esn) >= 10 && len(esn) <= 15) // Typical ESN length
}

func (d *IdentifierDetector) isValidMAC(mac string) bool {
	// Basic MAC address validation (simplified)
	mac = strings.ReplaceAll(mac, ":", "")
	mac = strings.ReplaceAll(mac, "-", "")
	if len(mac) != 12 {
		return false
	}
	for _, c := range mac {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func (d *IdentifierDetector) isValidIP(ip string) bool {
	// Basic IP validation (simplified)
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return false
	}
	for _, part := range parts {
		if len(part) == 0 || len(part) > 3 {
			return false
		}
	}
	return true
}

func (d *IdentifierDetector) isValidIMEI(imei string) bool {
	// IMEI should be 15 digits
	if len(imei) != 15 {
		return false
	}
	for _, c := range imei {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// LogDetectedIdentifiers logs detected identifiers for debugging
func (d *IdentifierDetector) LogDetectedIdentifiers(identifiers []DeviceIdentifier) {
	if len(identifiers) == 0 {
		log.Printf("No device identifiers detected")
		return
	}

	log.Printf("Detected %d device identifiers:", len(identifiers))
	for _, id := range identifiers {
		log.Printf("  - %s:%s = %s", id.Type, id.Key, id.Value)
	}
}