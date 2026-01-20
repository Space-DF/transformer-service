package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/Space-DF/transformer-service/internal/config"
	"github.com/Space-DF/transformer-service/internal/models"
	amqp "github.com/rabbitmq/amqp091-go"
)

type EventManager struct {
	channel *amqp.Channel
	cfg     config.OrgEventsConfig
	// Callbacks for tenant subscribe/unsubscibe
	subscribeFn   func(ctx context.Context, slug, vhost, queue, exchange string) error
	unsubscribeFn func(slug string)
}

func NewEventManager(ch *amqp.Channel, cfg config.OrgEventsConfig,
	subscribeToOrganization func(context.Context, string, string, string, string) error,
	unsubscribeFromOrganization func(string)) *EventManager {

	return &EventManager{channel: ch, cfg: cfg, subscribeFn: subscribeToOrganization, unsubscribeFn: unsubscribeFromOrganization}
}

type OrgEventProcessor func(context.Context, amqp.Delivery) error

func (m *EventManager) SendDiscoveryRequest(ctx context.Context) error {
	log.Println("Sending discovery request to console service for existing organizations...")

	request := models.OrgDiscoveryRequest{
		EventType:   models.OrgDiscoveryReq,
		EventID:     fmt.Sprintf("discovery-%d", time.Now().Unix()),
		Timestamp:   time.Now(),
		ServiceName: "transformer-service",
		ReplyTo:     m.cfg.Queue, // We'll receive response on our org events queue
	}

	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal discovery request: %w", err)
	}

	err = m.channel.PublishWithContext(
		ctx,
		m.cfg.Exchange,          // "org.events"
		"org.discovery.request", // routing key
		false,                   // mandatory
		false,                   // immediate
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

// listenToOrgEvents listens for organization lifecycle events (org.created, org.deleted)
func (m *EventManager) ListenToOrgEvents(ctx context.Context, process OrgEventProcessor) error {
	var (
		messages <-chan amqp.Delivery
		err      error
		attempt  = 1
	)

	for {
		if err = m.ensureOrgEventsTopology(); err == nil {
			messages, err = m.channel.Consume(
				m.cfg.Queue,
				m.cfg.ConsumerTag,
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

			if err := process(ctx, msg); err != nil {
				log.Printf("Error handling org event: %v", err)
				_ = msg.Nack(false, true) // Requeue
			} else {
				_ = msg.Ack(false)
			}
		}
	}
}

func (m *EventManager) ensureOrgEventsTopology() error {
	if err := m.channel.ExchangeDeclare(
		m.cfg.Exchange,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("exchange declare failed: %w", err)
	}

	if _, err := m.channel.QueueDeclare(
		m.cfg.Queue,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("queue declare failed: %w", err)
	}

	if err := m.channel.QueueBind(
		m.cfg.Queue,
		m.cfg.RoutingKey,
		m.cfg.Exchange,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("queue bind failed: %w", err)
	}

	return nil
}
