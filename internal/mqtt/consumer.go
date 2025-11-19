package mqtt

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Space-DF/transformer-service/internal/config"
	"github.com/Space-DF/transformer-service/internal/models"
	"github.com/Space-DF/transformer-service/internal/parsers"
	"github.com/Space-DF/transformer-service/internal/services"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Tenant logging helper for clean, filterable logs
func logTenant(tenant, vhost, emoji, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	log.Printf("[TENANT:%s][VHOST:%s] %s %s", tenant, vhost, emoji, message)
}

// TenantConsumer represents a consumer for a specific tenant
type TenantConsumer struct {
	OrgSlug     string
	Vhost       string
	QueueName   string
	Exchange    string
	ConsumerTag string
	Channel     *amqp.Channel
	Cancel      context.CancelFunc
}

type pooledConnection struct {
	conn     *amqp.Connection
	refCount int
}

// Consumer handles MQTT message consumption via AMQP
type Consumer struct {
	config               config.AMQPConfig
	orgEventsConfig      config.OrgEventsConfig
	orgEventsConn        *amqp.Connection
	orgEventsChannel     *amqp.Channel
	locationService      *services.LocationService
	transformService     *services.TransformService
	loggerService        *services.LoggerService
	deviceProfileService *services.DeviceProfileService
	done                 chan bool

	tenantMu        sync.RWMutex
	tenantConsumers map[string]*TenantConsumer

	vhostMu          sync.Mutex
	vhostConnections map[string]*pooledConnection
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
		tenantConsumers:      make(map[string]*TenantConsumer),
		vhostConnections:     make(map[string]*pooledConnection),
	}
}

// Connect establishes connection to AMQP broker
func (c *Consumer) Connect() error {
	var err error

	// Connect to AMQP broker
	c.orgEventsConn, err = amqp.Dial(c.config.BrokerURL)
	if err != nil {
		return fmt.Errorf("failed to connect to AMQP broker: %w", err)
	}

	// Create separate channel for org events
	c.orgEventsChannel, err = c.orgEventsConn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open org events channel: %w", err)
	}

	return nil
}

// Start begins consuming messages from AMQP broker with routing
func (c *Consumer) Start(ctx context.Context) error {
	// Note: amq.topic is a built-in exchange, no need to declare it
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
		c.orgEventsConfig.Exchange, // "org.events"
		"org.discovery.request",    // routing key
		false,                      // mandatory
		false,                      // immediate
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

func (c *Consumer) ensureOrgEventsTopology() error {
	if err := c.orgEventsChannel.ExchangeDeclare(
		c.orgEventsConfig.Exchange,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("exchange declare failed: %w", err)
	}

	if _, err := c.orgEventsChannel.QueueDeclare(
		c.orgEventsConfig.Queue,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("queue declare failed: %w", err)
	}

	if err := c.orgEventsChannel.QueueBind(
		c.orgEventsConfig.Queue,
		c.orgEventsConfig.RoutingKey,
		c.orgEventsConfig.Exchange,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("queue bind failed: %w", err)
	}

	return nil
}

func (c *Consumer) shouldHandleVhost(vhost string) bool {
	if len(c.config.AllowedVhosts) == 0 {
		return true
	}

	for _, allowed := range c.config.AllowedVhosts {
		if allowed == vhost {
			return true
		}
	}

	return false
}

func (c *Consumer) buildVhostURL(vhost string) (string, error) {
	baseURL := c.config.BrokerURL
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse broker url: %w", err)
	}

	if vhost == "" {
		parsed.Path = "/"
		parsed.RawPath = ""
	} else {
		encoded := "/" + url.PathEscape(vhost)
		parsed.Path = encoded
		parsed.RawPath = encoded
	}

	return parsed.String(), nil
}

func (c *Consumer) getOrCreateVhostConnection(vhost string) (*amqp.Connection, error) {
	c.vhostMu.Lock()
	defer c.vhostMu.Unlock()

	if pooled, exists := c.vhostConnections[vhost]; exists {
		pooled.refCount++
		return pooled.conn, nil
	}

	vhostURL, err := c.buildVhostURL(vhost)
	if err != nil {
		return nil, err
	}

	conn, err := amqp.Dial(vhostURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to vhost %s: %w", vhost, err)
	}

	c.vhostConnections[vhost] = &pooledConnection{
		conn:     conn,
		refCount: 1,
	}

	return conn, nil
}

func (c *Consumer) releaseVhostConnection(vhost string) {
	c.vhostMu.Lock()
	defer c.vhostMu.Unlock()

	pooled, exists := c.vhostConnections[vhost]
	if !exists {
		return
	}

	pooled.refCount--
	if pooled.refCount <= 0 {
		_ = pooled.conn.Close()
		delete(c.vhostConnections, vhost)
	}
}

func (c *Consumer) makeConsumerTag(orgSlug, vhost string) string {
	safeVhost := strings.NewReplacer("/", "_", ".", "_", ":", "_").Replace(vhost)
	return fmt.Sprintf("%s-%s-%d", orgSlug, safeVhost, time.Now().UnixNano())
}

// subscribeToOrganization starts consuming from an organization's existing queue
func (c *Consumer) subscribeToOrganization(parentCtx context.Context, orgSlug, vhost, queueName, exchange string) error {
	if queueName == "" {
		queueName = fmt.Sprintf("%s.transformer.queue", orgSlug)
	}
	if exchange == "" {
		exchange = fmt.Sprintf("%s.exchange", orgSlug)
	}

	if !c.shouldHandleVhost(vhost) {
		logTenant(orgSlug, vhost, "⚠️", "Skipping subscription; vhost not assigned to this transformer")
		return nil
	}

	c.tenantMu.RLock()
	if _, exists := c.tenantConsumers[orgSlug]; exists {
		c.tenantMu.RUnlock()
		logTenant(orgSlug, vhost, "ℹ️", "Subscription already active")
		return nil
	}
	c.tenantMu.RUnlock()

	conn, err := c.getOrCreateVhostConnection(vhost)
	if err != nil {
		return err
	}

	channel, err := conn.Channel()
	if err != nil {
		c.releaseVhostConnection(vhost)
		return fmt.Errorf("failed to open channel for vhost %s: %w", vhost, err)
	}

	if err := channel.Qos(c.config.PrefetchCount, 0, false); err != nil {
		_ = channel.Close()
		c.releaseVhostConnection(vhost)
		return fmt.Errorf("failed to set QoS for vhost %s: %w", vhost, err)
	}

	consumerTag := c.makeConsumerTag(orgSlug, vhost)

	messages, err := channel.Consume(
		queueName,
		consumerTag,
		c.config.AutoAck,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		_ = channel.Close()
		c.releaseVhostConnection(vhost)
		return fmt.Errorf("failed to start consuming from queue '%s' in vhost '%s': %w", queueName, vhost, err)
	}

	tenantCtx, cancel := context.WithCancel(parentCtx)
	consumer := &TenantConsumer{
		OrgSlug:     orgSlug,
		Vhost:       vhost,
		QueueName:   queueName,
		Exchange:    exchange,
		ConsumerTag: consumerTag,
		Channel:     channel,
		Cancel:      cancel,
	}

	c.tenantMu.Lock()
	c.tenantConsumers[orgSlug] = consumer
	c.tenantMu.Unlock()

	go c.processTenantMessages(tenantCtx, consumer, messages)

	logTenant(orgSlug, vhost, "▶️", "Started consuming from %s", queueName)
	return nil
}

// unsubscribeFromOrganization stops consuming from an organization's queue
func (c *Consumer) unsubscribeFromOrganization(orgSlug string) {
	c.tenantMu.Lock()
	consumer, exists := c.tenantConsumers[orgSlug]
	if exists {
		delete(c.tenantConsumers, orgSlug)
	}
	c.tenantMu.Unlock()

	if !exists || consumer == nil {
		return
	}

	consumer.Cancel() // Cancel the context to stop the goroutine

	if consumer.Channel != nil {
		_ = consumer.Channel.Cancel(consumer.ConsumerTag, false)
		_ = consumer.Channel.Close()
	}

	c.releaseVhostConnection(consumer.Vhost)

	logTenant(consumer.OrgSlug, consumer.Vhost, "⏹️", "Stopped consuming from %s", consumer.QueueName)
}

// stopAllConsumers stops all active tenant consumers
func (c *Consumer) stopAllConsumers() {
	log.Println("Stopping all tenant consumers...")
	c.tenantMu.Lock()
	for slug, consumer := range c.tenantConsumers {
		if consumer != nil {
			consumer.Cancel()
			if consumer.Channel != nil {
				_ = consumer.Channel.Cancel(consumer.ConsumerTag, false)
				_ = consumer.Channel.Close()
			}
			c.releaseVhostConnection(consumer.Vhost)
			logTenant(consumer.OrgSlug, consumer.Vhost, "⏹️", "Stopped consuming from %s", consumer.QueueName)
		}
		delete(c.tenantConsumers, slug)
	}
	c.tenantMu.Unlock()

	c.vhostMu.Lock()
	for vhost, pooled := range c.vhostConnections {
		if pooled != nil && pooled.conn != nil {
			_ = pooled.conn.Close()
		}
		delete(c.vhostConnections, vhost)
	}
	c.vhostMu.Unlock()
	log.Println("All tenant consumers stopped")
}

// listenToOrgEvents listens for organization lifecycle events (org.created, org.deleted)
func (c *Consumer) listenToOrgEvents(ctx context.Context) error {
	var (
		messages <-chan amqp.Delivery
		err      error
		attempt  = 1
	)

	for {
		if err = c.ensureOrgEventsTopology(); err == nil {
			messages, err = c.orgEventsChannel.Consume(
				c.orgEventsConfig.Queue,
				c.orgEventsConfig.ConsumerTag,
				false, // manual ack for reliability
				false,
				false,
				false,
				nil,
			)
			if err == nil {
				break
			}
		}

		backoff := time.Duration(attempt)
		if backoff > 10 {
			backoff = 10
		}

		log.Printf("Transformer org events setup retry (attempt %d, next in %ds): %v", attempt, int(backoff), err)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff * time.Second):
		}

		if attempt < 10 {
			attempt++
		}
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
	// if msg.RoutingKey == "org.discovery.response" {
	// 	var response models.OrgDiscoveryResponse
	// 	if err := json.Unmarshal(msg.Body, &response); err != nil {
	// 		return fmt.Errorf("failed to unmarshal discovery response: %w", err)
	// 	}

	// 	log.Printf("Received discovery response with %d active organizations", response.TotalCount)

	// 	// Subscribe to each active organization
	// 	for _, org := range response.Organizations {
	// 		if org.IsActive {
	// 			log.Printf("Bootstrapping subscription for org: %s", org.Slug)
	// 			if err := c.subscribeToOrganization(ctx, org.Slug); err != nil {
	// 				log.Printf("Failed to subscribe to org '%s': %v", org.Slug, err)
	// 				continue
	// 			}
	// 		}
	// 	}

	// 	log.Printf("Bootstrap complete: subscribed to %d organizations", response.TotalCount)
	// 	return nil
	// }

	// Handle regular org lifecycle events
	var event models.OrgEvent

	if err := json.Unmarshal(msg.Body, &event); err != nil {
		return fmt.Errorf("failed to unmarshal org event: %w", err)
	}

	log.Printf("Processing org event: %s for org: %s", event.EventType, event.Payload.Slug)

	orgSlug := event.Payload.Slug
	vhost := event.Payload.Vhost
	queueName := event.Payload.TransformerQueue
	exchange := event.Payload.Exchange

	if queueName == "" && orgSlug != "" {
		queueName = fmt.Sprintf("%s.transformer.queue", orgSlug)
	}
	if exchange == "" && orgSlug != "" {
		exchange = fmt.Sprintf("%s.exchange", orgSlug)
	}

	switch event.EventType {

	case models.OrgCreated:
		// New org created - subscribe to its queue
		if vhost == "" {
			log.Printf("Org created event missing vhost for org: %s", orgSlug)
			return nil
		}
		return c.subscribeToOrganization(ctx, orgSlug, vhost, queueName, exchange)

	case models.OrgDeactivated, models.OrgDeleted:
		// Org deleted/deactivated - unsubscribe
		c.unsubscribeFromOrganization(orgSlug)
		return nil

	default:
		log.Printf("Unknown event type: %s", event.EventType)
		return nil
	}
}

// processTenantMessages processes messages for a specific tenant
func (c *Consumer) processTenantMessages(ctx context.Context, tenant *TenantConsumer, messages <-chan amqp.Delivery) {
	logTenant(tenant.OrgSlug, tenant.Vhost, "🔄", "Processing messages for org: %s", tenant.OrgSlug)

	for {
		select {
		case <-ctx.Done():
			logTenant(tenant.OrgSlug, tenant.Vhost, "⏹️", "Stopping message processing for org: %s", tenant.OrgSlug)
			return

		case msg, ok := <-messages:
			if !ok {
				logTenant(tenant.OrgSlug, tenant.Vhost, "📪", "Message channel closed for org: %s", tenant.OrgSlug)
				return
			}

			logTenant(tenant.OrgSlug, tenant.Vhost, "📨", "Received message from routing key: %s", msg.RoutingKey)

			if err := c.handleMessage(msg, tenant); err != nil {
				logTenant(tenant.OrgSlug, tenant.Vhost, "❌", "Error processing message: %v", err)
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

	if c.orgEventsConn != nil {
		_ = c.orgEventsConn.Close()
	}

	return nil
}

// handleMessage processes a single MQTT message
func (c *Consumer) handleMessage(msg amqp.Delivery, tenant *TenantConsumer) error {
	orgSlug := tenant.OrgSlug
	vhost := tenant.Vhost

	logTenant(orgSlug, vhost, "⚙️", "Processing message from topic: %s", msg.RoutingKey)

	// Parse the incoming message
	var rawPayload map[string]interface{}
	if err := json.Unmarshal(msg.Body, &rawPayload); err != nil {
		logTenant(orgSlug, vhost, "❌", "Failed to unmarshal message: %v", err)
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	// Use the orgSlug passed as parameter (we already know it from the queue name)
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
		shouldSkip, skipErr := c.deviceProfileService.ShouldSkipDevice(orgSlug, devEUI)
		if skipErr != nil {
			logTenant(orgSlug, vhost, "⚠️", "Could not check skip status for device %s: %v", devEUI, skipErr)
		} else if shouldSkip {
			logTenant(orgSlug, vhost, "⏭️", "Skipping device %s as per configuration", devEUI)
			return nil
		}
	}

	// Check if device requires location calculation
	var deviceLocation *models.DeviceLocationData
	var err error

	if devEUI != "" && c.deviceProfileService != nil {
		requiresCalculation, profileErr := c.deviceProfileService.RequiresLocationCalculation(orgSlug, devEUI)
		if profileErr != nil {
			logTenant(orgSlug, vhost, "⚠️", "Could not get device profile for %s: %v. Proceeding with location calculation.", devEUI, profileErr)
			requiresCalculation = true // Default to requiring calculation
		}

		if requiresCalculation {
			logTenant(orgSlug, vhost, "📍", "Calculating location for device %s using trilateration", devEUI)
			// Calculate device location using decoded data if available, otherwise original payload
			deviceLocation, err = c.locationService.CalculateDeviceLocation(locationPayload)
			if err == nil && deviceLocation != nil {
				// Set organization from device mapping if available
				if _, mapping, mappingErr := c.deviceProfileService.GetDeviceProfile(orgSlug, devEUI); mappingErr == nil {
					deviceLocation.Organization = mapping.Organization
				}
			}
		} else {
			// Device has GPS, extract coordinates using device-specific parser
			logTenant(orgSlug, vhost, "🛰️", "Device %s has GPS capability, extracting GPS coordinates", devEUI)
			if _, mapping, profileErr := c.deviceProfileService.GetDeviceProfile(orgSlug, devEUI); profileErr == nil {
				deviceLocation, err = c.extractGPSFromDeviceParser(mapping.Profile, locationPayload, mapping.Organization)
			} else {
				logTenant(orgSlug, vhost, "⚠️", "Could not get device profile for %s: %v. Falling back to location calculation.", devEUI, profileErr)
				deviceLocation, err = c.locationService.CalculateDeviceLocation(locationPayload)
			}
		}
	} else {
		// No device profile service or devEUI, fall back to standard calculation
		logTenant(orgSlug, vhost, "⚠️", "No device profile service or devEUI, proceeding with location calculation")
		deviceLocation, err = c.locationService.CalculateDeviceLocation(locationPayload)
	}

	if deviceLocation != nil && deviceLocation.Organization == "" {
		deviceLocation.Organization = orgSlug
	}
	gatewayCount := c.countGateways(locationPayload)

	// Prepare processing info for logging
	processingInfo := models.ProcessingInfo{
		LocationCalculated: err == nil,
		HasLocationData:    c.hasLocationData(locationPayload),
		GatewayCount:       gatewayCount,
	}

	if err != nil {
		processingInfo.ErrorMessage = err.Error()

		// Log the raw data even if location calculation fails
		if c.loggerService != nil {
			logTenant(orgSlug, vhost, "📝", "Logging raw data for device: %s (error path), location calculated: %v, error: %v", devEUI, processingInfo.LocationCalculated, processingInfo.ErrorMessage)
			if logErr := c.loggerService.LogRawData(payload, locationPayload, processingInfo); logErr != nil {
				logTenant(orgSlug, vhost, "❌", "Failed to log raw data: %v", logErr)
			}
		}

		return fmt.Errorf("failed to calculate device location: %w", err)
	}

	// Transform data to output format
	transformedData, err := c.transformService.TransformDeviceData(deviceLocation, processingInfo.GatewayCount, payload)
	if err != nil {
		return fmt.Errorf("failed to transform device data: %w", err)
	}

	processingInfo.LocationResult = &models.LocationResult{
		Latitude:  deviceLocation.Latitude,
		Longitude: deviceLocation.Longitude,
		Accuracy:  transformedData.Location.Accuracy,
	}

	// Publish transformed data to output topic
	logTenant(orgSlug, vhost, "📤", "Publishing transformed data for device %s", deviceLocation.DevEUI)
	if err := c.publishTransformedData(tenant.Channel, transformedData, tenant); err != nil {
		return fmt.Errorf("failed to publish transformed data: %w", err)
	}

	// Log successful processing
	if c.loggerService != nil {
		logTenant(orgSlug, vhost, "📝", "Logging raw data for device: %s, location calculated: %v", devEUI, processingInfo.LocationCalculated)
		if logErr := c.loggerService.LogRawData(payload, locationPayload, processingInfo); logErr != nil {
			logTenant(orgSlug, vhost, "❌", "Failed to log raw data: %v", logErr)
		}
	}

	logTenant(orgSlug, vhost, "✅", "Successfully processed device: %s", deviceLocation.DevEUI)
	return nil
}

// publishTransformedData publishes the transformed data to the output topic
func (c *Consumer) publishTransformedData(channel *amqp.Channel, data *models.TransformedDeviceData, tenant *TenantConsumer) error {
	if channel == nil {
		return fmt.Errorf("tenant channel is nil")
	}

	body, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal transformed data: %w", err)
	}

	// Create tenant-specific output topics
	var topics []string
	for _, topic := range c.config.OutputTopics {
		if strings.Contains(topic, ".*.") {
			topic = strings.ReplaceAll(topic, "*", tenant.OrgSlug)
			topics = append(topics, topic)
		}
	}

	exchange := tenant.Exchange
	if exchange == "" {
		exchange = fmt.Sprintf("%s.exchange", tenant.OrgSlug)
	}

	for _, topic := range topics {
		logTenant(tenant.OrgSlug, tenant.Vhost, "📡", "Publishing to topic: %s", topic)
		err = channel.Publish(
			exchange,
			topic,
			false, // mandatory
			false, // immediate
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

		logTenant(tenant.OrgSlug, tenant.Vhost, "✅", "Published transformed data to topic: %s", topic)
	}
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
