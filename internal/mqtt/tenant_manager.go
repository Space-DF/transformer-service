package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/Space-DF/transformer-service/internal/models"
	"github.com/Space-DF/transformer-service/internal/mqtt/helpers"
	"github.com/Space-DF/transformer-service/internal/mqtt/logging"
	amqp "github.com/rabbitmq/amqp091-go"
)

// subscribeToOrganizationForEvent is a wrapper for EventManager callback
func (c *Consumer) subscribeToOrganizationForEvent(parentCtx context.Context, orgSlug, vhost, queueName, exchange string) error {
	return c.subscribeToOrganization(parentCtx, orgSlug, vhost, queueName, exchange, false)
}

// unsubscribeFromOrganizationForEvent is a wrapper for EventManager callback
func (c *Consumer) unsubscribeFromOrganizationForEvent(orgSlug string) {
	c.unsubscribeFromOrganization(orgSlug)
}

// subscribeToOrganization starts consuming from an organization's existing queue
// bypassReconnectingCheck: if true, skip the reconnecting guard (used during reestablish)
func (c *Consumer) subscribeToOrganization(parentCtx context.Context, orgSlug, vhost, queueName, exchange string, bypassReconnectingCheck bool) error {
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

	// Don't allow tenant subscriptions while main connection is reconnecting
	// The main reconnection will handle all tenants together
	if !bypassReconnectingCheck && c.reconnecting {
		logging.Tenant(orgSlug, vhost, "⏳", "Main connection reconnecting, skipping individual tenant subscription")
		return fmt.Errorf("main connection is reconnecting, tenant subscription deferred")
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

	tenantCtx, cancel := context.WithCancel(parentCtx) //#nosec G118
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
		return c.subscribeToOrganization(ctx, orgSlug, vhost, queueName, exchange, false)

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

// resubscribeTenant resubscribes to a tenant's queue after a channel closure
func (c *Consumer) resubscribeTenant(ctx context.Context, oldTenant *TenantConsumer) {
	// Check if main connection is down - if so, don't try individual resubscription
	// The main reconnection will handle all tenants together
	if c.orgEventsConn == nil || c.orgEventsConn.IsClosed() {
		logging.Tenant(oldTenant.OrgSlug, oldTenant.Vhost, "⏳", "Main connection down, waiting for centralized reconnection")
		return
	}

	// Cancel the old consumer goroutine
	oldTenant.Cancel()
	if oldTenant.Channel != nil {
		_ = oldTenant.Channel.Cancel(oldTenant.ConsumerTag, false)
		_ = oldTenant.Channel.Close()
	}

	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second
	maxAttempts := 5 // Reduced attempts since we should rely on main reconnection

	for attempt := 0; attempt < maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		// Check main connection again before each attempt
		if c.orgEventsConn == nil || c.orgEventsConn.IsClosed() {
			logging.Tenant(oldTenant.OrgSlug, oldTenant.Vhost, "⏳", "Main connection went down, aborting individual resubscription")
			return
		}

		// Remove from map first so subscribeToOrganization can create a new subscription
		c.tenantMu.Lock()
		delete(c.tenantConsumers, oldTenant.OrgSlug)
		c.tenantMu.Unlock()

		// Try to resubscribe
		err := c.subscribeToOrganization(ctx, oldTenant.OrgSlug, oldTenant.Vhost, oldTenant.QueueName, oldTenant.Exchange, false)
		if err == nil {
			logging.Tenant(oldTenant.OrgSlug, oldTenant.Vhost, "✅", "Successfully resubscribed (attempt %d)", attempt+1)
			return
		}

		logging.Tenant(oldTenant.OrgSlug, oldTenant.Vhost, "⚠️", "Failed to resubscribe (attempt %d): %v", attempt+1, err)

		// Exponential backoff
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	// If all attempts failed, remove from map so main reconnection can handle it
	c.tenantMu.Lock()
	delete(c.tenantConsumers, oldTenant.OrgSlug)
	c.tenantMu.Unlock()

	logging.Tenant(oldTenant.OrgSlug, oldTenant.Vhost, "❌", "Failed to resubscribe after %d attempts, removed from map for main reconnection", maxAttempts)
}
