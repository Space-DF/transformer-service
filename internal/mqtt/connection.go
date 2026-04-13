package mqtt

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Space-DF/transformer-service/internal/circuitbreaker"
	"github.com/Space-DF/transformer-service/internal/mqtt/logging"
	"github.com/Space-DF/transformer-service/internal/mqtt/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Connect establishes connection to AMQP broker
func (c *Consumer) Connect() error {
	var err error

	// Use circuit breaker for connection attempts
	err = c.circuitBreaker.Execute(func() error {
		// Connect to AMQP broker
		c.orgEventsConn, err = amqp.Dial(c.config.BrokerURL)
		if err != nil {
			return fmt.Errorf("failed to connect to AMQP broker: %w", err)
		}

		// Create separate channel for org events
		c.orgEventsChannel, err = c.orgEventsConn.Channel()
		if err != nil {
			defer func() {
				_ = c.orgEventsConn.Close()
			}()
			return fmt.Errorf("failed to open org events channel: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	c.eventManager = rabbitmq.NewEventManager(
		c.orgEventsChannel,
		c.orgEventsConfig,
		c.subscribeToOrganizationForEvent,
		c.unsubscribeFromOrganizationForEvent,
	)

	// Register close notifiers for connection monitoring
	c.setupConnectionMonitoring()

	log.Println("Successfully connected to AMQP broker")
	return nil
}

// setupConnectionMonitoring sets up close notifiers for the connection and channel
func (c *Consumer) setupConnectionMonitoring() {
	// Register connection close notifier
	c.connCloseNotifier = make(chan *amqp.Error, 1)
	c.orgEventsConn.NotifyClose(c.connCloseNotifier)

	// Register channel close notifier
	c.channelCloseNotifier = make(chan *amqp.Error, 1)
	c.orgEventsChannel.NotifyClose(c.channelCloseNotifier)

	// Start monitoring goroutine
	go c.monitorConnection()
}

// monitorConnection monitors the connection and channel for unexpected closures
func (c *Consumer) monitorConnection() {
	for {
		select {
		case <-c.done:
			return

		case err, ok := <-c.connCloseNotifier:
			if !ok {
				// Channel closed, expected during shutdown
				return
			}
			c.handleConnectionClosed(err)

		case err, ok := <-c.channelCloseNotifier:
			if !ok {
				return
			}
			c.handleChannelClosed(err)
		}
	}
}

// handleConnectionClosed handles unexpected connection closure
func (c *Consumer) handleConnectionClosed(err *amqp.Error) {
	log.Printf("AMQP connection closed unexpectedly: %v (code: %d)", err, err.Code)

	// Invalidate all pooled vhost connections since they're also closed
	c.vhostPool.InvalidateAll()

	// Notify reconnection goroutine
	select {
	case c.reconnectChan <- struct{}{}:
	default:
	}
}

// handleChannelClosed handles unexpected channel closure
func (c *Consumer) handleChannelClosed(err *amqp.Error) {
	log.Printf("AMQP channel closed unexpectedly: %v (code: %d)", err, err.Code)

	// Just trigger full reconnection - it's safer and simpler
	select {
	case c.reconnectChan <- struct{}{}:
	default:
	}
}

// reconnectConnection attempts to reconnect to the AMQP broker
func (c *Consumer) reconnectConnection(ctx context.Context) error {
	log.Println("Attempting to reconnect to AMQP broker")

	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second
	maxAttempts := 30

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Check circuit breaker
		if c.circuitBreaker.State() == circuitbreaker.StateOpen {
			cbTimeout := c.circuitBreaker.ResetTimeout()
			log.Printf("Circuit breaker is open, waiting %v", cbTimeout)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(cbTimeout):
			}
		}

		// Close existing connection if any
		if c.orgEventsConn != nil && !c.orgEventsConn.IsClosed() {
			_ = c.orgEventsConn.Close()
		}

		// Attempt to reconnect
		err := func() error {
			conn, err := amqp.Dial(c.config.BrokerURL)
			if err != nil {
				return err
			}

			ch, err := conn.Channel()
			if err != nil {
				defer func() {
					_ = conn.Close()
				}()
				return err
			}

			c.orgEventsConn = conn
			c.orgEventsChannel = ch

			// Re-register close notifiers
			c.connCloseNotifier = make(chan *amqp.Error, 1)
			c.channelCloseNotifier = make(chan *amqp.Error, 1)
			c.orgEventsConn.NotifyClose(c.connCloseNotifier)
			c.orgEventsChannel.NotifyClose(c.channelCloseNotifier)

			// Recreate event manager
			c.eventManager = rabbitmq.NewEventManager(
				c.orgEventsChannel,
				c.orgEventsConfig,
				c.subscribeToOrganizationForEvent,
				c.unsubscribeFromOrganizationForEvent,
			)

			return nil
		}()

		if err == nil {
			log.Printf("Successfully reconnected to AMQP broker (attempt %d)", attempt)

			// Reset circuit breaker
			c.circuitBreaker.Reset()

			// Wait a moment for connection to stabilize before re-establishing tenants
			log.Println("Waiting for connection to stabilize before re-establishing tenants...")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}

			// Re-establish all tenant connections (with reconnecting flag still set)
			c.reestablishTenantConnections(ctx)

			log.Printf("Re-established %d tenant connections", len(c.tenantConsumers))

			// Restart the connection monitor goroutine to watch the new notifier channels
			go c.monitorConnection()

			// Cancel old org events listener (stuck retrying on dead channel)
			if c.orgEventsCancel != nil {
				c.orgEventsCancel()
			}

			// Restart org events listener on the new EventManager
			orgEventsCtx, orgEventsCancel := context.WithCancel(ctx)
			c.orgEventsCancel = orgEventsCancel
			go func() {
				if err := c.eventManager.ListenToOrgEvents(orgEventsCtx, c.handleOrgEvent); err != nil {
					log.Printf("Error listening to org events after reconnection: %v", err)
				}
			}()

			return nil
		}

		log.Printf("Reconnection attempt %d failed: %v", attempt, err)

		// Record failure in circuit breaker
		c.circuitBreaker.RecordFailure()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		// Exponential backoff
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	return fmt.Errorf("failed to reconnect after %d attempts", maxAttempts)
}

// reestablishTenantConnections re-establishes connections for all active tenants
func (c *Consumer) reestablishTenantConnections(ctx context.Context) {
	c.tenantMu.Lock()
	tenants := make([]*TenantConsumer, 0, len(c.tenantConsumers))
	for _, consumer := range c.tenantConsumers {
		tenants = append(tenants, consumer)
	}
	c.tenantMu.Unlock()

	successCount := 0
	for _, tenant := range tenants {
		// Remove from map so subscribeToOrganization can add it back
		c.tenantMu.Lock()
		delete(c.tenantConsumers, tenant.OrgSlug)
		c.tenantMu.Unlock()

		// Bypass reconnecting check since we're already in reconnection flow
		err := c.subscribeToOrganization(ctx, tenant.OrgSlug, tenant.Vhost, tenant.QueueName, tenant.Exchange, true)

		if err == nil {
			successCount++
			logging.Tenant(tenant.OrgSlug, tenant.Vhost, "✅", "Successfully re-established connection")
		} else {
			logging.Tenant(tenant.OrgSlug, tenant.Vhost, "❌", "Failed to resubscribe: %v", err)
		}
	}

	log.Printf("Re-established %d/%d tenant connections", successCount, len(tenants))
}
