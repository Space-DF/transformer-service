package mqtt

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Space-DF/transformer-service/internal/circuitbreaker"
	"github.com/Space-DF/transformer-service/internal/config"
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

	// Connection monitoring
	circuitBreaker       *circuitbreaker.CircuitBreaker
	reconnectChan        chan struct{}
	connCloseNotifier    chan *amqp.Error
	channelCloseNotifier chan *amqp.Error
	reconnecting         atomic.Bool
	orgEventsCancel      context.CancelFunc
	monitorCancel        context.CancelFunc
	monitorWg            sync.WaitGroup
}

// NewConsumer creates a new MQTT consumer
func NewConsumer(cfg config.AMQPConfig, orgEventsCfg config.OrgEventsConfig, loggerService *services.LoggerService, deviceProfileService *services.DeviceProfileService, locationService *services.LocationService) *Consumer {
	parser := payload.NewParser()

	// Create circuit breaker
	cbConfig := circuitbreaker.Config{
		MaxFailures:      5,
		ResetTimeout:     30 * time.Second,
		SuccessThreshold: 2,
	}
	cb := circuitbreaker.New(cbConfig)

	return &Consumer{
		parser:               parser,
		config:               cfg,
		orgEventsConfig:      orgEventsCfg,
		locationService:      locationService,
		transformService:     services.NewTransformService(deviceProfileService),
		loggerService:        loggerService,
		deviceProfileService: deviceProfileService,
		done:                 make(chan bool, 1),
		tenantConsumers:      make(map[string]*TenantConsumer),
		vhostPool:            pool.New(cfg.BrokerURL),
		resolver:             resolver.New(locationService, deviceProfileService, logging.Tenant),
		circuitBreaker:       cb,
		reconnectChan:        make(chan struct{}, 1),
		connCloseNotifier:    make(chan *amqp.Error, 1),
		channelCloseNotifier: make(chan *amqp.Error, 1),
	}
}

// Start begins consuming messages from AMQP broker with routing
func (c *Consumer) Start(ctx context.Context) error {
	// Note: amq.topic is a built-in exchange, no need to declare it
	log.Println("Starting transformer service with pure event-driven architecture...")
	log.Println("Waiting for organization events to discover active tenants...")

	// Start reconnection monitor goroutine
	go c.reconnectionMonitor(ctx)

	// Start listening to organization events in a separate goroutine
	orgEventsCtx, orgEventsCancel := context.WithCancel(ctx) //#nosec G118
	c.orgEventsCancel = orgEventsCancel
	go func() {
		if err := c.eventManager.ListenToOrgEvents(orgEventsCtx, c.handleOrgEvent); err != nil {
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

// reconnectionMonitor monitors for reconnection requests
func (c *Consumer) reconnectionMonitor(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case <-c.reconnectChan:
			if c.reconnecting.Load() {
				log.Println("Already reconnecting, skipping duplicate request")
				continue
			}

			c.reconnecting.Store(true)

			log.Println("Reconnection triggered, attempting to reconnect...")
			if err := c.reconnectConnection(ctx); err != nil {
				c.reconnecting.Store(false)
				log.Printf("Failed to reconnect: %v", err)
				go func() {
					time.Sleep(10 * time.Second)
					select {
					case c.reconnectChan <- struct{}{}:
					default:
					}
				}()
			} else {
				c.reconnecting.Store(false)
				// Drain stale reconnection requests (from resubscribeTenant, close handlers, etc.)
				for {
					select {
					case _, ok := <-c.reconnectChan:
						if !ok {
							goto done
						}
					default:
						goto done
					}
				}
			done:
				log.Println("Successfully reconnected and re-established all tenants")
			}
		}
	}
}

// Stop gracefully stops the consumer
func (c *Consumer) Stop() error {
	close(c.done)

	// Cancel monitor goroutine
	if c.monitorCancel != nil {
		c.monitorCancel()
	}
	c.monitorWg.Wait()

	c.stopAllConsumers()

	if c.orgEventsChannel != nil {
		_ = c.orgEventsChannel.Close()
	}

	if c.orgEventsConn != nil {
		_ = c.orgEventsConn.Close()
	}

	return nil
}
