package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/Space-DF/transformer-service-go/internal/config"
	"github.com/Space-DF/transformer-service-go/internal/models"
	"github.com/Space-DF/transformer-service-go/internal/services"
)

// Consumer handles MQTT message consumption via RabbitMQ
type Consumer struct {
	config          config.MQTTConfig
	conn            *amqp.Connection
	channel         *amqp.Channel
	locationService *services.LocationService
	transformService *services.TransformService
	done            chan bool
}

// NewConsumer creates a new MQTT consumer
func NewConsumer(cfg config.MQTTConfig) *Consumer {
	return &Consumer{
		config:           cfg,
		locationService:  services.NewLocationService(),
		transformService: services.NewTransformService(),
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
				if !c.config.AutoAck {
					msg.Nack(false, true)
				}
			} else {
				// Acknowledge message if not auto-ack
				if !c.config.AutoAck {
					msg.Ack(false)
				}
			}
		}
	}
}

// handleMessage processes a single MQTT message
func (c *Consumer) handleMessage(msg amqp.Delivery) error {
	log.Printf("Received message from topic: %s", msg.RoutingKey)

	// Parse the incoming message
	var payload map[string]interface{}
	if err := json.Unmarshal(msg.Body, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	// Calculate device location
	deviceLocation, err := c.locationService.CalculateDeviceLocation(payload)
	if err != nil {
		return fmt.Errorf("failed to calculate device location: %w", err)
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