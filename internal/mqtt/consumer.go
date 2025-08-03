package mqtt

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/Space-DF/transformer-service-go/internal/config"
	"github.com/Space-DF/transformer-service-go/internal/models"
	"github.com/Space-DF/transformer-service-go/internal/parsers"
	"github.com/Space-DF/transformer-service-go/internal/services"
)

// Consumer handles MQTT message consumption via RabbitMQ
type Consumer struct {
	config          config.MQTTConfig
	conn            *amqp.Connection
	channel         *amqp.Channel
	locationService *services.LocationService
	transformService *services.TransformService
	loggerService   *services.LoggerService
	deviceProfileService *services.DeviceProfileService
	done            chan bool
}

// NewConsumer creates a new MQTT consumer
func NewConsumer(cfg config.MQTTConfig, loggerService *services.LoggerService, deviceProfileService *services.DeviceProfileService) *Consumer {
	return &Consumer{
		config:           cfg,
		locationService:  services.NewLocationService(),
		transformService: services.NewTransformService(),
		loggerService:    loggerService,
		deviceProfileService: deviceProfileService,
		done:             make(chan bool),
	}
}

// Connect establishes connection to RabbitMQ
func (c *Consumer) Connect() error {
	var err error
	
	// Connect to RabbitMQ
	c.conn, err = amqp.Dial(c.config.BrokerURL)
	if err != nil {
		return fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	// Create a channel
	c.channel, err = c.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open channel: %w", err)
	}

	// Set QoS for prefetch count
	err = c.channel.Qos(c.config.PrefetchCount, 0, false)
	if err != nil {
		return fmt.Errorf("failed to set QoS: %w", err)
	}

	return nil
}

// Start begins consuming messages from RabbitMQ with routing
func (c *Consumer) Start(ctx context.Context) error {
	// Note: amq.topic is a built-in exchange, no need to declare it

	// Declare the queue
	inputQueue, err := c.channel.QueueDeclare(
		c.config.Queue, // queue name
		true,           // durable
		false,          // delete when unused
		false,          // exclusive
		false,          // no-wait
		nil,            // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare queue: %w", err)
	}

	// Bind the queue to the exchange with routing key
	err = c.channel.QueueBind(
		inputQueue.Name,    // queue name
		c.config.RoutingKey, // routing key
		c.config.Exchange,  // exchange
		false,              // no-wait
		nil,                // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to bind queue: %w", err)
	}

	// Declare the output topic as a queue
	_, err = c.channel.QueueDeclare(
		c.config.OutputTopic, // queue name (topic name in MQTT)
		true,                 // durable
		false,                // delete when unused
		false,                // exclusive
		false,                // no-wait
		nil,                  // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare output queue: %w", err)
	}

	// Start consuming messages
	messages, err := c.channel.Consume(
		inputQueue.Name,    // queue
		c.config.ConsumerTag, // consumer
		c.config.AutoAck,     // auto-ack
		false,                // exclusive
		false,                // no-local
		false,                // no-wait
		nil,                  // args
	)
	if err != nil {
		return fmt.Errorf("failed to register consumer: %w", err)
	}

	log.Printf("Started consuming messages from queue: %s with routing key: %s", c.config.Queue, c.config.RoutingKey)

	// Process messages
	go c.processMessages(ctx, messages)

	// Wait for context cancellation or done signal
	select {
	case <-ctx.Done():
		log.Println("Context cancelled, stopping consumer")
	case <-c.done:
		log.Println("Consumer stopped")
	}

	return nil
}

// processMessages handles incoming MQTT messages
func (c *Consumer) processMessages(ctx context.Context, messages <-chan amqp.Delivery) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-messages:
			if !ok {
				log.Println("Message channel closed")
				return
			}

			if err := c.handleMessage(msg); err != nil {
				log.Printf("Error processing message: %v", err)
				// Reject message and requeue if not auto-ack
				// if !c.config.AutoAck {
				// 	msg.Nack(false, true)
				// }
			}
			
			// Always acknowledge message to remove it from queue if not auto-ack
			if !c.config.AutoAck {
				msg.Ack(false)
			}
		}
	}
}

// handleMessage processes a single MQTT message
func (c *Consumer) handleMessage(msg amqp.Delivery) error {
	log.Printf("Received message from topic: %s", msg.RoutingKey)

	// Parse the incoming message
	var rawPayload map[string]interface{}
	if err := json.Unmarshal(msg.Body, &rawPayload); err != nil {
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	// Debug: Print raw payload structure to understand the format
	log.Printf("Payload keys: %v", getKeys(rawPayload))
	
	// Log the complete raw payload structure for debugging
	rawPayloadJSON, _ := json.MarshalIndent(rawPayload, "", "  ")
	log.Printf("Complete raw payload:\n%s", string(rawPayloadJSON))

	// Check if there's a base64 encoded payload that needs to be decoded
	var payload map[string]interface{}
	var encodedPayload string
	var found bool
	var foundLocation string
	
	// Check for payload key (JSON stringified)
	if payloadStr, ok := rawPayload["payload"].(string); ok {
		encodedPayload = payloadStr
		found = true
		foundLocation = "payload"
	}
	
	if found {
		// First try to parse as JSON string directly
		var jsonPayload map[string]interface{}
		if err := json.Unmarshal([]byte(encodedPayload), &jsonPayload); err == nil {
			log.Printf("Successfully parsed JSON string payload from %s field", foundLocation)
			log.Printf("JSON payload keys: %v", getKeys(jsonPayload))
			
			// Log the complete JSON payload structure
			jsonPayloadJSON, _ := json.MarshalIndent(jsonPayload, "", "  ")
			log.Printf("Complete JSON payload:\n%s", string(jsonPayloadJSON))
			
			payload = jsonPayload
		} else {
			// If JSON parsing fails, use raw payload
			log.Printf("Failed to parse JSON string payload from %s field, using raw payload", foundLocation)
			payload = rawPayload
		}
	} else {
		// No base64 encoded payload, use raw payload directly
		payload = rawPayload
	}

	// Debug: Print final payload structure and decode raw_data if found
	var locationPayload map[string]interface{}
	locationPayload = payload // Default to original payload
	
	if rawData, ok := payload["raw_data"]; ok {
		if rawDataMap, ok := rawData.(map[string]interface{}); ok {
			log.Printf("Found raw_data at root level: %v", getKeys(rawDataMap))
		} else if rawDataStr, ok := rawData.(string); ok {
			log.Printf("Found raw_data at root level (string), attempting base64 decode")
			if decodedData, err := base64.StdEncoding.DecodeString(rawDataStr); err != nil {
				log.Printf("Failed to decode raw_data: %v", err)
			} else {
				log.Printf("Successfully decoded raw_data: %x (hex)", decodedData)
				// Try to parse as JSON
				jsonStr := string(decodedData)
				log.Printf("Decoded raw_data as string: %s", jsonStr)
				var jsonData interface{}
				if err := json.Unmarshal(decodedData, &jsonData); err != nil {
					log.Printf("Failed to parse decoded raw_data as JSON: %v", err)
					payload["decoded_raw_data"] = decodedData
				} else {
					log.Printf("Successfully parsed decoded raw_data as JSON")
					payload["decoded_raw_data"] = jsonData
					// Use decoded JSON data for location calculation if it's a map
					if jsonMap, ok := jsonData.(map[string]interface{}); ok {
						log.Printf("Using decoded JSON data for location calculation")
						locationPayload = jsonMap
					}
				}
			}
		}
	}

	// Extract devEUI first to check device profile
	var devEUI string
	if endDeviceIDs, ok := payload["end_device_ids"].(map[string]interface{}); ok {
		if eui, ok := endDeviceIDs["dev_eui"].(string); ok {
			devEUI = eui
		}
	}
	
	// If not found in payload, try locationPayload or check for dev_eui directly
	if devEUI == "" {
		if eui, ok := locationPayload["dev_eui"].(string); ok {
			devEUI = eui
		} else if eui, ok := payload["dev_eui"].(string); ok {
			devEUI = eui
		} else if eui, ok := locationPayload["devEui"].(string); ok {
			// LoRaWAN format uses devEui
			devEUI = eui
		} else if deviceInfo, ok := locationPayload["deviceInfo"].(map[string]interface{}); ok {
			// Try deviceInfo.devEui
			if eui, ok := deviceInfo["devEui"].(string); ok {
				devEUI = eui
			}
		}
	}
	
	// Check if device should be skipped
	if devEUI != "" && c.deviceProfileService != nil {
		shouldSkip, skipErr := c.deviceProfileService.ShouldSkipDevice(devEUI)
		if skipErr != nil {
			log.Printf("Warning: Could not check skip status for device %s: %v", devEUI, skipErr)
		} else if shouldSkip {
			log.Printf("Skipping processing for device %s as per configuration", devEUI)
			return nil
		}
	}
	
	// Check if device requires location calculation
	var deviceLocation *models.DeviceLocationData
	err := error(nil)
	
	if devEUI != "" && c.deviceProfileService != nil {
		requiresCalculation, profileErr := c.deviceProfileService.RequiresLocationCalculation(devEUI)
		if profileErr != nil {
			log.Printf("Warning: Could not get device profile for %s: %v. Proceeding with location calculation.", devEUI, profileErr)
			requiresCalculation = true // Default to requiring calculation
		}
		
		if requiresCalculation {
			// Calculate device location using decoded data if available, otherwise original payload
			deviceLocation, err = c.locationService.CalculateDeviceLocation(locationPayload)
			if err == nil && deviceLocation != nil {
				// Set organization from device mapping if available
				if _, mapping, mappingErr := c.deviceProfileService.GetDeviceProfile(devEUI); mappingErr == nil {
					deviceLocation.Organization = mapping.Organization
				}
			}
		} else {
			// Device has GPS, extract coordinates using device-specific parser
			log.Printf("Device %s has GPS capability, using device parser to extract GPS coordinates", devEUI)
			if _, mapping, profileErr := c.deviceProfileService.GetDeviceProfile(devEUI); profileErr == nil {
				deviceLocation, err = c.extractGPSFromDeviceParser(mapping.Profile, locationPayload, mapping.Organization)
			} else {
				log.Printf("Could not get device profile for %s: %v. Falling back to location calculation.", devEUI, profileErr)
				deviceLocation, err = c.locationService.CalculateDeviceLocation(locationPayload)
			}
		}
	} else {
		// No device profile service or devEUI, fall back to standard calculation
		log.Printf("No device profile service available or devEUI not found, proceeding with location calculation")
		deviceLocation, err = c.locationService.CalculateDeviceLocation(locationPayload)
	}
	
	// Prepare processing info for logging
	processingInfo := models.ProcessingInfo{
		LocationCalculated: err == nil,
		HasLocationData:    c.hasLocationData(locationPayload),
		GatewayCount:       c.countGateways(locationPayload),
	}
	
	if err != nil {
		processingInfo.ErrorMessage = err.Error()
		
		// Log the raw data even if location calculation fails
		if c.loggerService != nil {
			if logErr := c.loggerService.LogRawData(payload, locationPayload, processingInfo); logErr != nil {
				log.Printf("Failed to log raw data: %v", logErr)
			}
		}
		
		return fmt.Errorf("failed to calculate device location: %w", err)
	}

	// Add location result to processing info for successful calculations
	processingInfo.LocationResult = &models.LocationResult{
		Latitude:  deviceLocation.Latitude,
		Longitude: deviceLocation.Longitude,
		Accuracy:  fmt.Sprintf("%d_gateways", processingInfo.GatewayCount),
	}

	// Transform data to output format
	transformedData, err := c.transformService.TransformDeviceData(deviceLocation, payload)
	if err != nil {
		return fmt.Errorf("failed to transform device data: %w", err)
	}

	// Publish transformed data to output topic
	if err := c.publishTransformedData(transformedData); err != nil {
		return fmt.Errorf("failed to publish transformed data: %w", err)
	}

	// Log successful processing
	if c.loggerService != nil {
		if logErr := c.loggerService.LogRawData(payload, locationPayload, processingInfo); logErr != nil {
			log.Printf("Failed to log raw data: %v", logErr)
		}
	}

	log.Printf("Successfully processed device: %s", deviceLocation.DevEUI)
	return nil
}

// publishTransformedData publishes the transformed data to the output topic
func (c *Consumer) publishTransformedData(data *models.TransformedDeviceData) error {
	body, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal transformed data: %w", err)
	}

	err = c.channel.Publish(
		"",                    // exchange (use default for MQTT topics)
		c.config.OutputTopic,  // routing key (topic name)
		false,                 // mandatory
		false,                 // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			Timestamp:    time.Now(),
			DeliveryMode: amqp.Persistent, // make message persistent
		},
	)

	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	log.Printf("Published transformed data to topic: %s", c.config.OutputTopic)
	return nil
}

// getKeys returns the keys of a map for debugging
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// hasLocationData checks if the payload contains location data
func (c *Consumer) hasLocationData(payload map[string]interface{}) bool {
	// Try to extract uplink message from different possible locations
	var uplinkMessage map[string]interface{}
	
	if msg, ok := payload["uplink_message"].(map[string]interface{}); ok {
		uplinkMessage = msg
	} else if payloadData, ok := payload["payload"].(map[string]interface{}); ok {
		if msg, ok := payloadData["uplink_message"].(map[string]interface{}); ok {
			uplinkMessage = msg
		} else {
			uplinkMessage = payloadData
		}
	} else {
		uplinkMessage = payload
	}

	// Check for gateway metadata in multiple possible locations
	var rxMetadata []interface{}
	var ok bool
	
	if rxMetadata, ok = uplinkMessage["rx_metadata"].([]interface{}); ok {
		return len(rxMetadata) > 0
	}
	if rxMetadata, ok = uplinkMessage["gateways"].([]interface{}); ok {
		return len(rxMetadata) > 0
	}
	if rxMetadata, ok = uplinkMessage["gateway_info"].([]interface{}); ok {
		return len(rxMetadata) > 0
	}
	if rxMetadata, ok = uplinkMessage["rxInfo"].([]interface{}); ok {
		return len(rxMetadata) > 0
	}
	
	return false
}

// countGateways counts the number of gateways with location data
func (c *Consumer) countGateways(payload map[string]interface{}) int {
	// Try to extract uplink message from different possible locations
	var uplinkMessage map[string]interface{}
	
	if msg, ok := payload["uplink_message"].(map[string]interface{}); ok {
		uplinkMessage = msg
	} else if payloadData, ok := payload["payload"].(map[string]interface{}); ok {
		if msg, ok := payloadData["uplink_message"].(map[string]interface{}); ok {
			uplinkMessage = msg
		} else {
			uplinkMessage = payloadData
		}
	} else {
		uplinkMessage = payload
	}

	// Check for gateway metadata in multiple possible locations
	var rxMetadata []interface{}
	var ok bool
	
	if rxMetadata, ok = uplinkMessage["rx_metadata"].([]interface{}); !ok {
		if rxMetadata, ok = uplinkMessage["gateways"].([]interface{}); !ok {
			if rxMetadata, ok = uplinkMessage["gateway_info"].([]interface{}); !ok {
				rxMetadata, ok = uplinkMessage["rxInfo"].([]interface{})
			}
		}
	}
	
	if !ok {
		return 0
	}

	count := 0
	for _, gw := range rxMetadata {
		gateway, ok := gw.(map[string]interface{})
		if !ok {
			continue
		}

		// Check if gateway has location data
		if location, exists := gateway["location"]; exists && location != nil {
			count++
		} else {
			// Check for direct lat/lon fields
			if _, hasLat := gateway["latitude"]; hasLat {
				if _, hasLon := gateway["longitude"]; hasLon {
					count++
				}
			}
		}
	}

	return count
}

// Stop gracefully stops the consumer
func (c *Consumer) Stop() error {
	close(c.done)

	if c.channel != nil {
		if err := c.channel.Close(); err != nil {
			log.Printf("Error closing channel: %v", err)
		}
	}

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			log.Printf("Error closing connection: %v", err)
		}
	}

	return nil
}

// extractGPSFromDeviceParser extracts GPS coordinates using device-specific parser
func (c *Consumer) extractGPSFromDeviceParser(profile string, payload map[string]interface{}, organization string) (*models.DeviceLocationData, error) {
	switch profile {
	case "RAK4630":
		return c.extractGPSFromRAK4630(payload, organization)
	default:
		return nil, fmt.Errorf("GPS extraction not implemented for device profile: %s", profile)
	}
}

// extractGPSFromRAK4630 extracts GPS coordinates from RAK4630 device using CBOR parsing
func (c *Consumer) extractGPSFromRAK4630(payload map[string]interface{}, organization string) (*models.DeviceLocationData, error) {
	// Create RAK4630 parser
	rak4630Parser := parsers.NewRAK4630Parser()
	
	// Use RAK4630 parser to extract GPS data from payload
	locationData, err := rak4630Parser.ParsePayload(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RAK4630 GPS data: %w", err)
	}
	
	// Set organization
	locationData.Organization = organization
	
	return locationData, nil
}