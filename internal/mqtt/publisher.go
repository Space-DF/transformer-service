package mqtt

import (
	"context"
	"fmt"
	"strings"
	"time"

	segmentjson "github.com/segmentio/encoding/json"

	"github.com/Space-DF/transformer-service/internal/models"
	"github.com/Space-DF/transformer-service/internal/mqtt/logging"
	amqp "github.com/rabbitmq/amqp091-go"
)

// publishTransformedData publishes the transformed data to the output topic
func (c *Consumer) publishTransformedData(channel *amqp.Channel, data *models.TransformedDeviceData, tenant *TenantConsumer) error {
	if channel == nil {
		return fmt.Errorf("tenant channel is nil")
	}

	body, err := segmentjson.Marshal(data)
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

	body, err := segmentjson.Marshal(data)
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

// publishEntityTelemetry publishes individual entity telemetry messages
func (c *Consumer) publishEntityTelemetry(channel *amqp.Channel, data *models.TelemetryPayload, tenant *TenantConsumer) error {
	spaceSlug := data.SpaceSlug
	if spaceSlug == "" {
		spaceSlug = "unknown"
	}

	exchange := tenant.Exchange
	if exchange == "" {
		return fmt.Errorf("exchange not configured for entity telemetry")
	}

	// Log summary of all entities first
	var entitySummary []string
	for _, entity := range data.Entities {
		stateStr := fmt.Sprintf("%v", entity.State)
		if entity.UnitOfMeas != "" {
			stateStr += entity.UnitOfMeas
		}
		entitySummary = append(entitySummary, fmt.Sprintf("%s=%s", entity.Name, stateStr))
	}
	logging.Tenant(tenant.OrgSlug, tenant.Vhost, "📊", "Entities [%s]: %s", data.DeviceEUI, strings.Join(entitySummary, " | "))

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

		body, err := segmentjson.Marshal(entityPayload)
		if err != nil {
			return fmt.Errorf("failed to marshal entity telemetry payload for %s: %w", entityID, err)
		}

		if c.config.EntityBridgeRoutingKey == "" {
			return fmt.Errorf("entity_bridge_routing_key not configured")
		}

		routingKey := fmt.Sprintf(c.config.EntityBridgeRoutingKey, tenant.OrgSlug, spaceSlug, entityID)

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

func (c *Consumer) publishLNSEvent(tenant *TenantConsumer, event map[string]interface{}, orgSlug string, spaceSlug string, deviceID string) error {
	if tenant.Channel == nil {
		return fmt.Errorf("tenant channel is nil")
	}

	body, err := segmentjson.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal LNS event: %w", err)
	}

	exchange := tenant.Exchange
	if exchange == "" {
		exchange = fmt.Sprintf("%s.exchange", tenant.OrgSlug)
	}

	if spaceSlug == "" || deviceID == "" {
		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "⚠️", "Cannot publish LNS event: missing spaceSlug or deviceID (space=%s, device=%s), skipping", spaceSlug, deviceID)
		return nil
	}
	routingKey := fmt.Sprintf("tenant.%s.transformed.telemetry.device.location", orgSlug)

	logging.Tenant(tenant.OrgSlug, tenant.Vhost, "📡", "Publishing LNS event to routing key: %s", routingKey)
	if err := tenant.Channel.Publish(
		exchange,
		routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			Timestamp:    time.Now(),
			DeliveryMode: amqp.Persistent,
		},
	); err != nil {
		return fmt.Errorf("failed to publish LNS event to %s: %w", routingKey, err)
	}

	return nil
}
