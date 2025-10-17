package mqtt

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Space-DF/transformer-service/internal/config"
	"github.com/Space-DF/transformer-service/internal/models"
	"github.com/Space-DF/transformer-service/internal/parsers"
	"github.com/Space-DF/transformer-service/internal/services"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Tenant logging helper for clean, filterable logs
func logTenant(tenant, emoji, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	log.Printf("[TENANT:%s] %s %s", tenant, emoji, message)
}

// TenantConsumer represents a consumer for a specific tenant
type TenantConsumer struct {
	OrgSlug   string
	QueueName string
	Cancel    context.CancelFunc
}

// Consumer handles MQTT message consumption via AMQP
type Consumer struct {
	config               config.AMQPConfig
	orgEventsConfig      config.OrgEventsConfig
	conn                 *amqp.Connection
	channel              *amqp.Channel
	orgEventsChannel     *amqp.Channel
	locationService      *services.LocationService
	transformService     *services.TransformService
	loggerService        *services.LoggerService
	deviceProfileService *services.DeviceProfileService
	done                 chan bool

	// Dynamic queue management
	activeConsumers      sync.Map // map[string]*TenantConsumer
	queueMutex           sync.RWMutex
	isRunning            bool
}

// NewConsumer creates a new MQTT consumer
func NewConsumer(cfg config.AMQPConfig, orgEventsCfg config.OrgEventsConfig, loggerService *services.LoggerService, deviceProfileService *services.DeviceProfileService) *Consumer {
	return &Consumer{
		config:               cfg,
		orgEventsConfig:      orgEventsCfg,
		locationService:      services.NewLocationService(),
		transformService:     services.NewTransformService(deviceProfileService),
		loggerService:        loggerService,
		deviceProfileService: deviceProfileService,
		done:                 make(chan bool, 1),
	}
}

// Connect establishes connection to AMQP broker
func (c *Consumer) Connect() error {
	var err error

	// Connect to AMQP broker
	c.conn, err = amqp.Dial(c.config.BrokerURL)
	if err != nil {
		return fmt.Errorf("failed to connect to AMQP broker: %w", err)
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

	// Create separate channel for org events
	c.orgEventsChannel, err = c.conn.Channel()
	if err != nil {
			return fmt.Errorf("failed to open org events channel: %w", err)
	}

	return nil
}

// Start begins consuming messages from AMQP broker with routing
func (c *Consumer) Start(ctx context.Context) error {
	// Note: amq.topic is a built-in exchange, no need to declare it
    c.queueMutex.Lock()
    c.isRunning = true
    c.queueMutex.Unlock()

    log.Println("Starting transformer service with pure event-driven architecture...")
    log.Println("Waiting for organization events to discover active tenants...")


		// Start listening to organization events in a separate goroutine
    go func() {
        if err := c.listenToOrgEvents(ctx); err != nil {
            log.Printf("Org events listener error: %v", err)
        }
    }()

    // Send bootstrap discovery request after a small delay (let listener setup first)
    go func() {
        time.Sleep(2 * time.Second)
        if err := c.sendDiscoveryRequest(ctx); err != nil {
            log.Printf("Failed to send discovery request: %v", err)
        }
    }()

    // Wait for context cancellation or done signal
    select {
    case <-ctx.Done():
        log.Println("Context cancelled, stopping consumer")
        c.stopAllConsumers()
    case <-c.done:
        log.Println("Consumer stopped")
        c.stopAllConsumers()
    }

    return nil
}

// sendDiscoveryRequest sends a request to console service to get all active orgs
func (c *Consumer) sendDiscoveryRequest(ctx context.Context) error {
    log.Println("Sending discovery request to console service for existing organizations...")

    request := models.OrgDiscoveryRequest{
        EventType:   models.OrgDiscoveryReq,
        EventID:     fmt.Sprintf("discovery-%d", time.Now().Unix()),
        Timestamp:   time.Now(),
        ServiceName: "transformer-service",
        ReplyTo:     c.orgEventsConfig.Queue, // We'll receive response on our org events queue
    }

    body, err := json.Marshal(request)
    if err != nil {
        return fmt.Errorf("failed to marshal discovery request: %w", err)
    }

    err = c.orgEventsChannel.PublishWithContext(
        ctx,
        c.orgEventsConfig.Exchange,    // "org.events"
        "org.discovery.request",        // routing key
        false,                          // mandatory
        false,                          // immediate
        amqp.Publishing{
            ContentType: "application/json",
            Body:        body,
            Timestamp:   time.Now(),
        },
    )

    if err != nil {
        return fmt.Errorf("failed to publish discovery request: %w", err)
    }

    log.Println("Discovery request sent successfully")
    return nil
}

// subscribeToOrganization starts consuming from an organization's existing queue
func (c *Consumer) subscribeToOrganization(parentCtx context.Context, orgSlug string) error {
    // Queue name format: {org_slug}.transformer.queue
    // This queue will be created by console/device service!
		defaultExchange := "amq.topic"
    queueName := fmt.Sprintf("%s.transformer.queue", orgSlug)

    log.Printf("Attempting to consume from pre-existing queue: %s", queueName)

		exchangeName := fmt.Sprintf("%s.exchange", orgSlug)
		// Declare exchange for this tenant
		err := c.channel.ExchangeDeclare(
			exchangeName,
			"topic",
			true,  // durable
			false, // auto-delete
			false, // internal
			false, // no-wait
			nil,
		)
		if err != nil {
			c.channel.Close()
			return fmt.Errorf("failed to declare exchange for org %s: %w", orgSlug, err)
		}

		routingKey := fmt.Sprintf("tenant.%s.device.data", orgSlug)
		err = c.channel.ExchangeBind(
			exchangeName,
			routingKey,
			defaultExchange,  // durable
			false, // no-wait
			nil,
		)
		if err != nil {
			c.channel.Close()
			return fmt.Errorf("failed to bind exchange to exchange %s: %w", orgSlug, err)
		}

		queueInfo, err := c.channel.QueueDeclare(
			queueName, // name
			true,      // durablePublishes back to specific exchange
			false,     // delete when unused
			false,     // exclusivePublishes back to specific exchange
			false,     // no-wait
			nil,       // arguments
		)
    if err != nil {
        return fmt.Errorf("queue '%s' does not exist (should be created by console service): %w", queueName, err)
    }

    log.Printf("Queue exists: %s (messages: %d, consumers: %d)", 
        queueName, queueInfo.Messages, queueInfo.Consumers)
		
		// Bind queue to exchange
		err = c.channel.QueueBind(
			queueName,
			routingKey,
			exchangeName,
			false,
			nil,
		)
		if err != nil {
			c.channel.Close()
			return fmt.Errorf("failed to bind queue for org %s: %w", orgSlug, err)
		}
    // Start consuming from the existing queue
    // Generate a unique consumer tag with timestamp to avoid conflicts after restart
    consumerTag := fmt.Sprintf("%s-transformer-%d", orgSlug, time.Now().Unix())
    
    messages, err := c.channel.Consume(
        queueName,                           // queue name (already exists!)
        consumerTag,                         // unique consumer tag
        c.config.AutoAck,                    // auto-ack
        false,                               // exclusive
        false,                               // no-local
        false,                               // no-wait
        nil,                                 // args
    )
    if err != nil {
        return fmt.Errorf("failed to start consuming from queue '%s': %w", queueName, err)
    }

    // Create a cancellable context for this tenant
    tenantCtx, cancel := context.WithCancel(parentCtx)

    // Store the consumer
    consumer := &TenantConsumer{
        OrgSlug:   orgSlug,
        QueueName: queueName,
        Cancel:    cancel,
    }
    c.activeConsumers.Store(orgSlug, consumer)

    // Start processing messages for this tenant in a dedicated goroutine
    go c.processTenantMessages(tenantCtx, orgSlug, messages)

    log.Printf("Started consuming from %s (org: %s)", queueName, orgSlug)
    return nil
}

// unsubscribeFromOrganization stops consuming from an organization's queue
func (c *Consumer) unsubscribeFromOrganization(orgSlug string) {
    value, exists := c.activeConsumers.Load(orgSlug)
    if !exists {
        return
    }

    consumer := value.(*TenantConsumer)
    consumer.Cancel() // Cancel the context to stop the goroutine
    c.activeConsumers.Delete(orgSlug)

    log.Printf("Stopped consuming from org: %s", orgSlug)
}

// stopAllConsumers stops all active tenant consumers
func (c *Consumer) stopAllConsumers() {
    log.Println("Stopping all tenant consumers...")
    c.activeConsumers.Range(func(key, value interface{}) bool {
        consumer := value.(*TenantConsumer)
        consumer.Cancel()
        return true
    })
    c.activeConsumers = sync.Map{}
    log.Println("All tenant consumers stopped")
}

// listenToOrgEvents listens for organization lifecycle events (org.created, org.deleted)
func (c *Consumer) listenToOrgEvents(ctx context.Context) error {
    log.Println("Setting up organization events listener...")

    // Declare the org.events exchange
    err := c.orgEventsChannel.ExchangeDeclare(
        c.orgEventsConfig.Exchange, // "org.events"
        "topic",
        true,  // durable
        false, // auto-deleted
        false, // internal
        false, // no-wait
        nil,
    )
    if err != nil {
        return fmt.Errorf("Failed to declare org events exchange: %w", err)
    }

    // Declare queue for org events
    orgQueue, err := c.orgEventsChannel.QueueDeclare(
        c.orgEventsConfig.Queue, // "transformer.org.events.queue"
        true,                    // durable
        false,                   // delete when unused
        false,                   // exclusive
        false,                   // no-wait
        nil,
    )
    if err != nil {
        return fmt.Errorf("Failed to declare org events queue: %w", err)
    }

    // Bind to org events
    err = c.orgEventsChannel.QueueBind(
        orgQueue.Name,
        c.orgEventsConfig.RoutingKey, // "org.#"
        c.orgEventsConfig.Exchange,
        false,
        nil,
    )
    if err != nil {
        return fmt.Errorf("Failed to bind org events queue: %w", err)
    }

    log.Printf("Listening for org events on: %s", orgQueue.Name)

    // Start consuming org events
    messages, err := c.orgEventsChannel.Consume(
        orgQueue.Name,
        c.orgEventsConfig.ConsumerTag,
        false, // manual ack for reliability
        false,
        false,
        false,
        nil,
    )
    if err != nil {
        return fmt.Errorf("failed to consume org events: %w", err)
    }

    // Process org events
    for {
        select {
        case <-ctx.Done():
            return nil
        case msg, ok := <-messages:
            if !ok {
                return nil
            }

            log.Printf("Received org event: %s", msg.RoutingKey)

            if err := c.handleOrgEvent(ctx, msg); err != nil {
                log.Printf("Error handling org event: %v", err)
                _ = msg.Nack(false, true) // Requeue
            } else {
                _ = msg.Ack(false)
            }
        }
    }
}

// handleOrgEvent processes organization lifecycle events
func (c *Consumer) handleOrgEvent(ctx context.Context, msg amqp.Delivery) error {
    log.Printf("Received org event with routing key: %s", msg.RoutingKey)

    // Handle discovery response (contains all active orgs)
    if msg.RoutingKey == "org.discovery.response" {
        var response models.OrgDiscoveryResponse
        if err := json.Unmarshal(msg.Body, &response); err != nil {
            return fmt.Errorf("failed to unmarshal discovery response: %w", err)
        }

        log.Printf("Received discovery response with %d active organizations", response.TotalCount)

        // Subscribe to each active organization
        for _, org := range response.Organizations {
            if org.IsActive {
                log.Printf("Bootstrapping subscription for org: %s", org.Slug)
                if err := c.subscribeToOrganization(ctx, org.Slug); err != nil {
                    log.Printf("Failed to subscribe to org '%s': %v", org.Slug, err)
                    continue
                }
            }
        }

        log.Printf("Bootstrap complete: subscribed to %d organizations", response.TotalCount)
        return nil
    }

    // Handle regular org lifecycle events
    var event models.OrgEvent

    if err := json.Unmarshal(msg.Body, &event); err != nil {
        return fmt.Errorf("failed to unmarshal org event: %w", err)
    }

    log.Printf("Processing org event: %s for org: %s", event.EventType, event.Organization.Slug)

    switch event.EventType {

    case models.OrgCreated:
        // New org created - subscribe to its queue
        return c.subscribeToOrganization(ctx, event.Organization.Slug)

    case models.OrgDeactivated:
        // Org deleted/deactivated - unsubscribe
        c.unsubscribeFromOrganization(event.Organization.Slug)
        return nil

    default:
        log.Printf("Unknown event type: %s", event.EventType)
        return nil
    }
}

// processTenantMessages processes messages for a specific tenant
func (c *Consumer) processTenantMessages(ctx context.Context, orgSlug string, messages <-chan amqp.Delivery) {
    logTenant(orgSlug, "🔄", "Processing messages for org: %s", orgSlug)

    for {
        select {
        case <-ctx.Done():
            logTenant(orgSlug, "⏹️", "Stopping message processing for org: %s", orgSlug)
            return

        case msg, ok := <-messages:
            if !ok {
                logTenant(orgSlug, "📪", "Message channel closed for org: %s", orgSlug)
                return
            }

            logTenant(orgSlug, "📨", "Received message from routing key: %s", msg.RoutingKey)

            if err := c.handleMessage(msg, orgSlug); err != nil {
                logTenant(orgSlug, "❌", "Error processing message: %v", err)
                // if !c.config.AutoAck {
                //    _ = msg.Nack(false, true) // Requeue on error
                // }
            } 
            
            // Always acknowledge message to remove it from queue if not auto-ack
            if !c.config.AutoAck {
                _ = msg.Ack(false)
            }
            
        }
    }
}

// Stop gracefully stops the consumer
func (c *Consumer) Stop() error {
    close(c.done)
    c.stopAllConsumers()

    if c.orgEventsChannel != nil {
      _ = c.orgEventsChannel.Close()
    }

    if c.channel != nil {
      _ = c.channel.Close()
    }

    if c.conn != nil {
      _ = c.conn.Close()
    }

    return nil
}

// handleMessage processes a single MQTT message
func (c *Consumer) handleMessage(msg amqp.Delivery, orgSlug string) error {
	logTenant(orgSlug, "⚙️", "Processing message from topic: %s", msg.RoutingKey)

	// Parse the incoming message
	var rawPayload map[string]interface{}
	if err := json.Unmarshal(msg.Body, &rawPayload); err != nil {
		logTenant(orgSlug, "❌", "Failed to unmarshal message: %v", err)
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	// Use the orgSlug passed as parameter (we already know it from the queue name)
	tenant := orgSlug

	// Check if there's a base64 encoded payload that needs to be decoded
	var payload map[string]interface{}
	var encodedPayload string
	var found bool

	// Check for payload key (JSON stringified)
	if payloadStr, ok := rawPayload["payload"].(string); ok {
		encodedPayload = payloadStr
		found = true
	}

	if found {
		// First try to parse as JSON string directly
		var jsonPayload map[string]interface{}
		if err := json.Unmarshal([]byte(encodedPayload), &jsonPayload); err == nil {

			payload = jsonPayload
		} else {
			// If JSON parsing fails, use raw payload
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
		if _, ok := rawData.(map[string]interface{}); ok {
			// rawDataMap found
		} else if rawDataStr, ok := rawData.(string); ok {
			if decodedData, err := base64.StdEncoding.DecodeString(rawDataStr); err == nil {
				// Try to parse as JSON
				var jsonData interface{}
				if err := json.Unmarshal(decodedData, &jsonData); err == nil {
					payload["decoded_raw_data"] = jsonData
					// Use decoded JSON data for location calculation if it's a map
					if jsonMap, ok := jsonData.(map[string]interface{}); ok {
						locationPayload = jsonMap
					}
				} else {
					payload["decoded_raw_data"] = decodedData
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
			logTenant(orgSlug, "⚠️", "Could not check skip status for device %s: %v", devEUI, skipErr)
		} else if shouldSkip {
			logTenant(orgSlug, "⏭️", "Skipping device %s as per configuration", devEUI)
			return nil
		}
	}

	// Check if device requires location calculation
	var deviceLocation *models.DeviceLocationData
	var err error

	if devEUI != "" && c.deviceProfileService != nil {
		requiresCalculation, profileErr := c.deviceProfileService.RequiresLocationCalculation(devEUI)
		if profileErr != nil {
			logTenant(orgSlug, "⚠️", "Could not get device profile for %s: %v. Proceeding with location calculation.", devEUI, profileErr)
			requiresCalculation = true // Default to requiring calculation
		}

		if requiresCalculation {
			logTenant(orgSlug, "📍", "Calculating location for device %s using trilateration", devEUI)
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
			logTenant(orgSlug, "🛰️", "Device %s has GPS capability, extracting GPS coordinates", devEUI)
			if _, mapping, profileErr := c.deviceProfileService.GetDeviceProfile(devEUI); profileErr == nil {
				deviceLocation, err = c.extractGPSFromDeviceParser(mapping.Profile, locationPayload, mapping.Organization)
			} else {
				logTenant(orgSlug, "⚠️", "Could not get device profile for %s: %v. Falling back to location calculation.", devEUI, profileErr)
				deviceLocation, err = c.locationService.CalculateDeviceLocation(locationPayload)
			}
		}
	} else {
		// No device profile service or devEUI, fall back to standard calculation
		logTenant(orgSlug, "⚠️", "No device profile service or devEUI, proceeding with location calculation")
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
			logTenant(orgSlug, "📝", "Logging raw data for device: %s (error path), location calculated: %v, error: %v", devEUI, processingInfo.LocationCalculated, processingInfo.ErrorMessage)
			if logErr := c.loggerService.LogRawData(payload, locationPayload, processingInfo); logErr != nil {
				logTenant(orgSlug, "❌", "Failed to log raw data: %v", logErr)
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
	logTenant(orgSlug, "📤", "Publishing transformed data for device %s", deviceLocation.DevEUI)
	if err := c.publishTransformedData(transformedData, tenant); err != nil {
		return fmt.Errorf("failed to publish transformed data: %w", err)
	}

	// Log successful processing
	if c.loggerService != nil {
		logTenant(orgSlug, "📝", "Logging raw data for device: %s, location calculated: %v", devEUI, processingInfo.LocationCalculated)
		if logErr := c.loggerService.LogRawData(payload, locationPayload, processingInfo); logErr != nil {
			logTenant(orgSlug, "❌", "Failed to log raw data: %v", logErr)
		}
	}

	logTenant(orgSlug, "✅", "Successfully processed device: %s", deviceLocation.DevEUI)
	return nil
}

// publishTransformedData publishes the transformed data to the output topic
func (c *Consumer) publishTransformedData(data *models.TransformedDeviceData, tenant string) error {
	body, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal transformed data: %w", err)
	}

	// Create tenant-specific output topic
	tenantOutputTopic := fmt.Sprintf("tenant.%s.transformed.device.location", tenant)
	logTenant(tenant, "📡", "Publishing to topic: %s", tenantOutputTopic)
	err = c.channel.Publish(
		fmt.Sprintf("%s.exchange", tenant),    // exchange (use amq.topic)
		tenantOutputTopic, 	  								 // routing key (topic name)
		false,                                 // mandatory
		false,                                 // immediate
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

	logTenant(tenant, "✅", "Published transformed data to topic: %s", tenantOutputTopic)
	return nil
}

// getKeys returns the keys of a map for debugging
// func getKeys(m map[string]interface{}) []string {
// 	keys := make([]string, 0, len(m))
// 	for k := range m {
// 		keys = append(keys, k)
// 	}
// 	return keys
// }

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
	} else if uplinkEvent, ok := payload["uplinkEvent"].(map[string]interface{}); ok {
		// Check uplinkEvent for rxInfo (custom format)
		uplinkMessage = uplinkEvent
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
	} else if uplinkEvent, ok := payload["uplinkEvent"].(map[string]interface{}); ok {
		// Check uplinkEvent for rxInfo (custom format)
		uplinkMessage = uplinkEvent
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