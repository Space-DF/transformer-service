package mqtt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Space-DF/transformer-service/internal/components"
	"github.com/Space-DF/transformer-service/internal/components/registry"
	"github.com/Space-DF/transformer-service/internal/config"
	"github.com/Space-DF/transformer-service/internal/models"
	"github.com/Space-DF/transformer-service/internal/mqtt/helpers"
	"github.com/Space-DF/transformer-service/internal/mqtt/logging"
	"github.com/Space-DF/transformer-service/internal/mqtt/payload"
	"github.com/Space-DF/transformer-service/internal/mqtt/rabbitmq"
	"github.com/Space-DF/transformer-service/internal/mqtt/resolver"
	pool "github.com/Space-DF/transformer-service/internal/mqtt/vhostpool"
	"github.com/Space-DF/transformer-service/internal/services"
	amqp "github.com/rabbitmq/amqp091-go"
)

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

// Consumer handles MQTT message consumption via AMQP
type Consumer struct {
	parser               *payload.Parser
	config               config.AMQPConfig
	orgEventsConfig      config.OrgEventsConfig
	orgEventsConn        *amqp.Connection
	orgEventsChannel     *amqp.Channel
	locationService      *services.LocationService
	transformService     *services.TransformService
	loggerService        *services.LoggerService
	deviceProfileService *services.DeviceProfileService
	resolver             *resolver.Resolver
	done                 chan bool

	tenantMu        sync.RWMutex
	tenantConsumers map[string]*TenantConsumer

	vhostPool *pool.Pool

	eventManager *rabbitmq.EventManager
}

// NewConsumer creates a new MQTT consumer
func NewConsumer(cfg config.AMQPConfig, orgEventsCfg config.OrgEventsConfig, loggerService *services.LoggerService, deviceProfileService *services.DeviceProfileService) *Consumer {
	locationSvc := services.NewLocationService()
	parser := payload.NewParser()

	return &Consumer{
		parser:               parser,
		config:               cfg,
		orgEventsConfig:      orgEventsCfg,
		locationService:      locationSvc,
		transformService:     services.NewTransformService(deviceProfileService),
		loggerService:        loggerService,
		deviceProfileService: deviceProfileService,
		done:                 make(chan bool, 1),
		tenantConsumers:      make(map[string]*TenantConsumer),
		vhostPool:            pool.New(cfg.BrokerURL),
		resolver:             resolver.New(locationSvc, deviceProfileService, logging.Tenant),
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

	c.eventManager = rabbitmq.NewEventManager(
		c.orgEventsChannel,
		c.orgEventsConfig,
		c.subscribeToOrganization,
		c.unsubscribeFromOrganization,
	)

	return nil
}

// Start begins consuming messages from AMQP broker with routing
func (c *Consumer) Start(ctx context.Context) error {
	// Note: amq.topic is a built-in exchange, no need to declare it
	log.Println("Starting transformer service with pure event-driven architecture...")
	log.Println("Waiting for organization events to discover active tenants...")

	// Start listening to organization events in a separate goroutine
	go func() {
		if err := c.eventManager.ListenToOrgEvents(ctx, c.handleOrgEvent); err != nil {
			log.Printf("Org events listener error: %v", err)
		}
	}()

	// Send bootstrap discovery request after a small delay (let listener setup first)
	go func() {
		time.Sleep(2 * time.Second)
		if err := c.eventManager.SendDiscoveryRequest(ctx); err != nil {
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

// subscribeToOrganization starts consuming from an organization's existing queue
func (c *Consumer) subscribeToOrganization(parentCtx context.Context, orgSlug, vhost, queueName, exchange string) error {
	if queueName == "" {
		queueName = fmt.Sprintf("%s.transformer.queue", orgSlug)
	}
	if exchange == "" {
		exchange = fmt.Sprintf("%s.exchange", orgSlug)
	}

	if !helpers.ShouldHandleVhost(vhost, c.config.AllowedVhosts) {
		logging.Tenant(orgSlug, vhost, "⚠️", "Skipping subscription; vhost not assigned to this transformer")
		return nil
	}

	c.tenantMu.Lock()
	if _, exists := c.tenantConsumers[orgSlug]; exists {
		c.tenantMu.Unlock()
		logging.Tenant(orgSlug, vhost, "ℹ️", "Subscription already active")
		return nil
	}

	conn, err := c.vhostPool.Acquire(vhost)
	if err != nil {
		c.tenantMu.Unlock()
		return err
	}

	channel, err := conn.Channel()
	if err != nil {
		c.vhostPool.Release(vhost)
		c.tenantMu.Unlock()
		return fmt.Errorf("failed to open channel for vhost %s: %w", vhost, err)
	}

	if err := channel.Qos(c.config.PrefetchCount, 0, false); err != nil {
		_ = channel.Close()
		c.vhostPool.Release(vhost)
		c.tenantMu.Unlock()
		return fmt.Errorf("failed to set QoS for vhost %s: %w", vhost, err)
	}

	consumerTag := helpers.MakeConsumerTag(orgSlug, vhost)

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
		c.vhostPool.Release(vhost)
		c.tenantMu.Unlock()
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

	c.tenantConsumers[orgSlug] = consumer
	c.tenantMu.Unlock()

	go c.processTenantMessages(tenantCtx, consumer, messages)

	logging.Tenant(orgSlug, vhost, "▶️", "Started consuming from %s", queueName)
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

	c.vhostPool.Release(consumer.Vhost)

	logging.Tenant(consumer.OrgSlug, consumer.Vhost, "⏹️", "Stopped consuming from %s", consumer.QueueName)
}

// handleOrgEvent processes organization lifecycle events
func (c *Consumer) handleOrgEvent(ctx context.Context, msg amqp.Delivery) error {
	log.Printf("Received org event with routing key: %s", msg.RoutingKey)

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
			c.vhostPool.Release(consumer.Vhost)
			logging.Tenant(consumer.OrgSlug, consumer.Vhost, "⏹️", "Stopped consuming from %s", consumer.QueueName)
		}
		delete(c.tenantConsumers, slug)
	}
	c.tenantMu.Unlock()

	c.vhostPool.CloseAll()
	log.Println("All tenant consumers stopped")
}

// processTenantMessages processes messages for a specific tenant
func (c *Consumer) processTenantMessages(ctx context.Context, tenant *TenantConsumer, messages <-chan amqp.Delivery) {
	logging.Tenant(tenant.OrgSlug, tenant.Vhost, "🔄", "Processing messages for org: %s", tenant.OrgSlug)

	for {
		select {
		case <-ctx.Done():
			logging.Tenant(tenant.OrgSlug, tenant.Vhost, "⏹️", "Stopping message processing for org: %s", tenant.OrgSlug)
			return

		case msg, ok := <-messages:
			if !ok {
				logging.Tenant(tenant.OrgSlug, tenant.Vhost, "📪", "Message channel closed for org: %s", tenant.OrgSlug)
				return
			}

			logging.Tenant(tenant.OrgSlug, tenant.Vhost, "📨", "Received message from routing key: %s", msg.RoutingKey)

			if err := c.handleMessage(msg, tenant); err != nil {
				logging.Tenant(tenant.OrgSlug, tenant.Vhost, "❌", "Error processing message: %v", err)
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

	logging.Tenant(orgSlug, vhost, "⚙️", "Processing message from topic: %s", msg.RoutingKey)

	var err error
	var payload map[string]interface{}
	var locationPayload map[string]interface{}

	// Parse the incoming message
	payload, locationPayload, err = c.parser.Parse(msg)
	if err != nil {
		logging.Tenant(orgSlug, vhost, "❌", "Failed to parse message: %v", err)
		return fmt.Errorf("failed to parse message: %w", err)
	}

	// Extract devEUI first to check device profile
	var devEUI string = c.parser.ExtractDevEUI(payload, locationPayload)

	// Check if device should be skipped
	deviceLocation, processingInfo, err := c.resolver.Resolve(orgSlug, vhost, devEUI, payload, locationPayload)
	if errors.Is(err, resolver.ErrDeviceSkipped) {
		return nil
	}
	if err != nil {
		logging.Tenant(orgSlug, vhost, "❌", "Failed to resolve device %s: %v", devEUI, err)
		if c.loggerService != nil {
			_ = c.loggerService.LogRawData(payload, locationPayload, *processingInfo)
		}
		return err
	}

	// Transform data to output format
	transformedData, err := c.transformService.TransformDeviceData(deviceLocation, processingInfo.GatewayCount, payload)
	if err != nil {
		return fmt.Errorf("failed to transform device data: %w", err)
	}

	// Try to parse sensor entities and publish telemetry payload
	if parseResult, mapping, perr := c.parseEntities(orgSlug, devEUI, payload); perr == nil && parseResult != nil {
		logging.Tenant(orgSlug, vhost, "", "Parsed %d telemetry entities for device %s", len(parseResult.Entities), devEUI)
		if telemetryPayload, terr := c.buildTelemetryPayload(parseResult, orgSlug, mapping); terr == nil {
			if err := c.publishTelemetry(tenant.Channel, telemetryPayload, tenant); err != nil {
				logging.Tenant(orgSlug, vhost, "⚠️", "Failed to publish telemetry payload: %v", err)
			} else {
				logging.Tenant(orgSlug, vhost, "✅", "Published telemetry payload with %d entities", len(parseResult.Entities))
			}
		} else {
			logging.Tenant(orgSlug, vhost, "⚠️", "Failed to build telemetry payload: %v", terr)
		}
	} else if perr != nil {
		logging.Tenant(orgSlug, vhost, "⚠️", "Failed to parse telemetry entities: %v", perr)
	}

	processingInfo.LocationResult = &models.LocationResult{
		Latitude:  deviceLocation.Latitude,
		Longitude: deviceLocation.Longitude,
		Accuracy:  transformedData.Location.Accuracy,
	}

	// Publish transformed data to output topic
	logging.Tenant(orgSlug, vhost, "📤", "Publishing transformed data for device %s", deviceLocation.DevEUI)
	if err := c.publishTransformedData(tenant.Channel, transformedData, tenant); err != nil {
		return fmt.Errorf("failed to publish transformed data: %w", err)
	}

	if c.loggerService != nil {
		logging.Tenant(orgSlug, vhost, "📝", "Logging raw data for device: %s, location calculated: %v", devEUI, processingInfo.LocationCalculated)
		if logErr := c.loggerService.LogRawData(payload, locationPayload, *processingInfo); logErr != nil {
			logging.Tenant(orgSlug, vhost, "❌", "Failed to log raw data: %v", logErr)
		}
	}

	logging.Tenant(orgSlug, vhost, "✅", "Successfully processed device: %s", deviceLocation.DevEUI)
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
		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "📡", "Publishing to topic: %s", topic)
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

		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "✅", "Published transformed data to topic: %s", topic)
	}
	return nil
}

// publishTelemetry publishes telemetry payloads (entities) to the telemetry queue
func (c *Consumer) publishTelemetry(channel *amqp.Channel, data *models.TelemetryPayload, tenant *TenantConsumer) error {
	if channel == nil {
		return fmt.Errorf("tenant channel is nil")
	}

	body, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal telemetry payload: %w", err)
	}
	logging.Tenant(tenant.OrgSlug, tenant.Vhost, "🔍", "Telemetry payload body: %s", string(body))

	telemetryRoutingKey := fmt.Sprintf("tenant.%s.transformed.telemetry.device.location", tenant.OrgSlug)
	logging.Tenant(tenant.OrgSlug, tenant.Vhost, "📡", "Publishing telemetry payload to %s", telemetryRoutingKey)

	if err := channel.PublishWithContext(
		context.Background(),
		tenant.Exchange,
		telemetryRoutingKey,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
		},
	); err != nil {
		return err
	}

	return c.publishEntityTelemetry(channel, data, tenant)
}

func (c *Consumer) publishEntityTelemetry(channel *amqp.Channel, data *models.TelemetryPayload, tenant *TenantConsumer) error {
	spaceSlug := data.SpaceSlug
	if spaceSlug == "" {
		spaceSlug = "unknown"
	}

	exchange := tenant.Exchange
	if exchange == "" {
		return fmt.Errorf("exchange not configured for entity telemetry")
	}

	for _, entity := range data.Entities {
		entityID := entity.UniqueID
		if entityID == "" {
			entityID = "unknown"
		}

		entityPayload := models.EntityTelemetryPayload{
			Organization: data.Organization,
			DeviceEUI:    data.DeviceEUI,
			DeviceID:     data.DeviceID,
			SpaceSlug:    spaceSlug,
			Entity:       entity,
			Timestamp:    data.Timestamp,
			Source:       data.Source,
			Metadata:     data.Metadata,
		}

		body, err := json.Marshal(entityPayload)
		if err != nil {
			return fmt.Errorf("failed to marshal entity telemetry payload for %s: %w", entityID, err)
		}

		if c.config.EntityBridgeRoutingKey == "" {
			return fmt.Errorf("entity_bridge_routing_key not configured")
		}

		routingKey := fmt.Sprintf(c.config.EntityBridgeRoutingKey, tenant.OrgSlug, spaceSlug, entityID)
		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "🔍", "Entity telemetry payload for %s: %s", entityID, string(body))
		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "📡", "Publishing entity telemetry to %s", routingKey)

		if err := channel.PublishWithContext(
			context.Background(),
			exchange,
			routingKey,
			false,
			false,
			amqp.Publishing{
				ContentType:  "application/json",
				Body:         body,
				DeliveryMode: amqp.Persistent,
				Timestamp:    time.Now(),
			},
		); err != nil {
			return fmt.Errorf("failed to publish entity telemetry for %s: %w", entityID, err)
		}
	}

	return nil
}

// parseEntities attempts to parse entities for telemetry and returns the device mapping
func (c *Consumer) parseEntities(orgSlug, devEUI string, payload map[string]interface{}) (*components.ParseResult, *models.DeviceMapping, error) {
	if devEUI == "" {
		return nil, nil, fmt.Errorf("dev_eui missing")
	}

	mapping, err := c.deviceProfileService.GetDeviceMapping(orgSlug, devEUI)
	if err != nil || mapping == nil {
		return nil, nil, fmt.Errorf("device mapping not found: %w", err)
	}

	deviceType := components.DeviceType(strings.ToUpper(mapping.Profile))
	raw := &components.RawPayload{
		DeviceEUI: devEUI,
		Timestamp: time.Now(),
		Metadata:  payload,
	}

	// Extract data field from the most likely location first
	raw.Data = c.extractDataField(payload)

	component := registry.FindComponent(deviceType, raw)
	if component == nil {
		return nil, nil, fmt.Errorf("no component found for device type: %s", deviceType)
	}

	parseResult, err := component.ParseToEntities(context.Background(), orgSlug, mapping.Profile, deviceType, raw)
	return parseResult, mapping, err
}

// buildTelemetryPayload converts parse result to telemetry payload
func (c *Consumer) buildTelemetryPayload(parseResult *components.ParseResult, orgSlug string, mapping *models.DeviceMapping) (*models.TelemetryPayload, error) {
	if parseResult == nil {
		return nil, fmt.Errorf("parse result is nil")
	}

	// Get device identifiers from mapping (prefer mapping over payload)
	deviceID := "unknown"
	spaceSlug := ""
	if mapping != nil {
		deviceID = mapping.DeviceID
		spaceSlug = mapping.SpaceSlug
	}

	deviceInfo := models.TelemetryDeviceInfo{
		Identifiers:  parseResult.DeviceInfo.Identifiers,
		Name:         parseResult.DeviceInfo.Name,
		Manufacturer: parseResult.DeviceInfo.Manufacturer,
		Model:        parseResult.DeviceInfo.Model,
		ModelID:      parseResult.DeviceInfo.ModelID,
	}

	var telemetryEntities []models.TelemetryEntity
	for _, entity := range parseResult.Entities {
		telemetryEntities = append(telemetryEntities, models.TelemetryEntity{
			UniqueID:    entity.UniqueID,
			EntityID:    entity.EntityID,
			EntityType:  entity.EntityType,
			DeviceClass: entity.DeviceClass,
			Name:        entity.Name,
			State:       entity.State,
			Attributes:  entity.Attributes,
			DisplayType: entity.DisplayType,
			UnitOfMeas:  entity.UnitOfMeas,
			Timestamp:   entity.Timestamp.Format(time.RFC3339),
		})
	}

	return &models.TelemetryPayload{
		Organization: orgSlug,
		DeviceEUI:    parseResult.DeviceEUI,
		DeviceID:     deviceID,
		SpaceSlug:    spaceSlug,
		DeviceInfo:   deviceInfo,
		Entities:     telemetryEntities,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Source:       "transformer-service",
	}, nil
}

// extractDataField extracts the data field from nested payload format
func (c *Consumer) extractDataField(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}

	// Get data from decoded_raw_data.uplinkEvent.data
	if decoded, ok := payload["decoded_raw_data"].(map[string]interface{}); ok {
		if uplinkEvent, ok := decoded["uplinkEvent"].(map[string]interface{}); ok {
			if d, ok := uplinkEvent["data"].(string); ok {
				return d
			}
		}
	}

	return ""
}

