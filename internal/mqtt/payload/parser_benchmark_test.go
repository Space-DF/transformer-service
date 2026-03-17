package payload

import (
	"encoding/base64"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Test messages representing real IoT sensor data

// Simple message with dev_eui
var simpleMessage = amqp.Delivery{
	Body: []byte(`{
		"dev_eui": "0102030405060708",
		"timestamp": 1234567890,
		"data": "AQIDBA=="
	}`),
}

// Message with nested JSON payload (common MQTT format)
var nestedPayloadMessage = amqp.Delivery{
	Body: []byte(`{
		"dev_eui": "0102030405060708",
		"payload": "{\"dev_eui\": \"0102030405060708\", \"temperature\": 22.5, \"humidity\": 65.0}",
		"timestamp": 1234567890
	}`),
}

// Message with base64 encoded raw_data containing JSON
var rawDataRowDataMessage = amqp.Delivery{
	Body: []byte(`{
		"dev_eui": "0102030405060708",
		"raw_data": "eyJ1cGxpbmtFdmVudCI6eyJkYXRhIjoiYXNkZmZoamtsOyIsImNvdW50ZXI6NDJ9fQ==",
		"timestamp": 1234567890
	}`),
}

// Full complex message with all nested structures
var complexMessage = func() amqp.Delivery {
	// Create nested JSON for payload field
	nestedJSON := `{
		"dev_eui": "0102030405060708",
		"temperature": 22.5,
		"humidity": 65.0,
		"battery": 85
	}`
	// Create base64 encoded JSON for raw_data
	rawDataJSON := `{"uplinkEvent":{"data":"asefhalkj","counter":42},"latitude":52.5,"longitude":13.4}`
	encodedRawData := base64.StdEncoding.EncodeToString([]byte(rawDataJSON))

	return amqp.Delivery{
		Body: []byte(`{
			"dev_eui": "0102030405060708",
			"payload": ` + string(nestedJSON) + `,
			"raw_data": "` + encodedRawData + `",
			"timestamp": 1234567890,
			"metadata": {"frequency": 868300000, "data_rate": "SF10BW125"}
		}`),
	}
}()

// Typical LoRaWAN uplink message (realistic size ~200-300 bytes)
var lorawanMessage = func() amqp.Delivery {
	// Simulate decoded payload from LoRaWAN
	decodedPayload := `{
		"dev_eui": "0102030405060708",
		"temperature": 23.7,
		"humidity": 62.5,
		"pressure": 1013.25,
		"battery": 3200,
		"rssi": -85,
		"snr": 7.5
	}`
	// Simulate raw hex data as base64
	rawData := `{"data":"0102030405060708090a0b0c0d0e0f10","port":1,"counter":123}`
	encodedRawData := base64.StdEncoding.EncodeToString([]byte(rawData))

	return amqp.Delivery{
		Body: []byte(`{
			"dev_eui": "0102030405060708",
			"payload": ` + decodedPayload + `,
			"raw_data": "` + encodedRawData + `",
			"metadata": {
				"gateway": ["eui-b827ebfffe123456"],
				"time": "2024-03-11T10:30:00Z",
				"frequency": 868300000,
				"data_rate": "SF10BW125"
			}
		}`),
	}
}()

// Real-world YabbyEdge message (based on actual production logs)
// This represents a typical LoRaWAN device uplink with:
// - base64 encoded raw_data containing full device info
// - decoded_raw_data already populated
// - rxInfo arrays with gateway data
// - txInfo with modulation details
var yabbyEdgeMessage = func() amqp.Delivery {
	// The raw_data is base64 encoded JSON from the device
	rawDataJSON := `{
		"applicationID": "1",
		"applicationName": "TestApp",
		"deviceName": "YabbyEdge-001",
		"devEUI": "0004430200251091",
		"deviceInfo": {
			"tenantId": "52f14cd4-c6f1-4fbd-8f87-4025e1d49242",
			"tenantName": "SpaceDF",
			"applicationId": "ca739e26-7b67-4f14-b95e-c4ca0e4c8c9e",
			"applicationName": "Asset Trackers",
			"deviceName": "YabbyEdge-001",
			"devEui": "0004430200251091",
			"devAddr": "01234567",
			"tags": {"location": "warehouse", "type": "tracker"}
		},
		"deviceProfileID": "c7eef5c8-4063-4e2b-a123-1234567890ab",
		"devAddr": "01234567",
		"adr": true,
		"dr": 5,
		"fCnt": 3,
		"fPort": 5,
		"data": "AQHOqrvM3e7/WHlzBqhmjj8gTgAA",
		"object": {
			"message_type": "location",
			"fport": 5,
			"in_trip": true,
			"inactive": false,
			"time_set": true,
			"position_sequence": 1,
			"wifi_scan_count": 1,
			"wifi_scans": [{"mac": "aabbccddeeff", "rssi": -50}],
			"gnss_nav_data": "58797306a8668e3f204e0000"
		},
		"decoded_payload": {
			"message_type": "connect",
			"fport": 89,
			"connect_id": "aabbccddeeff",
			"dev_reset": false,
			"fcnt_reset": false,
			"firmware_version": "1.0",
			"product_id": 85,
			"hardware_revision": 4
		},
		"confirmedUplink": false,
		"txInfo": {
			"frequency": 868100000,
			"modulation": "LORA",
			"loRaModulationInfo": {"bandwidth": 125, "spreadingFactor": 7, "codeRate": "4/5"}
		},
		"rxInfo": [{
			"gatewayID": "aa555a0000000000",
			"uplinkID": "12345678-1234-1234-1234-123456789abc",
			"rssi": -60,
			"snr": 7,
			"channel": 0,
			"location": {"latitude": 2.0835, "longitude": 142.35, "altitude": 35}
		}],
		"publishedAt": "2026-03-10T13:01:42.402Z"
	}`
	encodedRawData := base64.StdEncoding.EncodeToString([]byte(rawDataJSON))

	return amqp.Delivery{
		Body: []byte(`{
			"dev_eui": "0004430200251091",
			"raw_data": "` + encodedRawData + `",
			"metadata": {
				"content_type": "application/json",
				"mqtt_topic": "tenant/worty222/device/data",
				"tenant_id": "worty222",
				"transport": "http"
			}
		}`),
	}
}()

// Benchmark simple message parsing
func BenchmarkParser_Simple(b *testing.B) {
	parser := NewParser()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, err := parser.Parse(simpleMessage)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark nested payload parsing
func BenchmarkParser_NestedPayload(b *testing.B) {
	parser := NewParser()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, err := parser.Parse(nestedPayloadMessage)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark raw_data decoding
func BenchmarkParser_RawData(b *testing.B) {
	parser := NewParser()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, err := parser.Parse(rawDataRowDataMessage)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark complex message with all features
func BenchmarkParser_Complex(b *testing.B) {
	parser := NewParser()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, err := parser.Parse(complexMessage)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark realistic LoRaWAN message
func BenchmarkParser_LoRaWAN(b *testing.B) {
	parser := NewParser()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, err := parser.Parse(lorawanMessage)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark throughput: Messages per second at different sizes
func BenchmarkParser_Throughput_Simple(b *testing.B) {
	parser := NewParser()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, err := parser.Parse(simpleMessage)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParser_Throughput_LoRaWAN(b *testing.B) {
	parser := NewParser()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, err := parser.Parse(lorawanMessage)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark parallel parsing (simulating concurrent goroutines)
func BenchmarkParser_Parallel(b *testing.B) {
	parser := NewParser()
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, err := parser.Parse(lorawanMessage)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// Sub-benchmark: Compare message types side by side
func BenchmarkParser_AllTypes(b *testing.B) {
	parser := NewParser()

	b.Run("Simple", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _, err := parser.Parse(simpleMessage)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("NestedPayload", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _, err := parser.Parse(nestedPayloadMessage)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("RawData", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _, err := parser.Parse(rawDataRowDataMessage)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Complex", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _, err := parser.Parse(complexMessage)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("LoRaWAN", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _, err := parser.Parse(lorawanMessage)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("YabbyEdge", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _, err := parser.Parse(yabbyEdgeMessage)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// Benchmark the real-world YabbyEdge message
func BenchmarkParser_YabbyEdge(b *testing.B) {
	parser := NewParser()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, err := parser.Parse(yabbyEdgeMessage)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Memory benchmark: Process many messages and check for memory leaks
func BenchmarkParser_MemoryStress(b *testing.B) {
	parser := NewParser()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Alternate between message types to simulate real traffic
		var msg amqp.Delivery
		switch i % 4 {
		case 0:
			msg = simpleMessage
		case 1:
			msg = nestedPayloadMessage
		case 2:
			msg = rawDataRowDataMessage
		case 3:
			msg = lorawanMessage
		}

		_, _, err := parser.Parse(msg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark for validating the kata goal: <100ns per operation
func BenchmarkParser_KataTarget(b *testing.B) {
	parser := NewParser()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, err := parser.Parse(simpleMessage)
		if err != nil {
			b.Fatal(err)
		}
	}
}
