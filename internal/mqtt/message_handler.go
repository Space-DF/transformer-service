package mqtt

import (
	"context"
	"errors"
	"fmt"

	"github.com/Space-DF/transformer-service/internal/components"
	"github.com/Space-DF/transformer-service/internal/lns"
	"github.com/Space-DF/transformer-service/internal/models"
	"github.com/Space-DF/transformer-service/internal/mqtt/logging"
	"github.com/Space-DF/transformer-service/internal/mqtt/resolver"
	"github.com/Space-DF/transformer-service/internal/telemetry"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otellog "go.opentelemetry.io/otel/log"
)

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
				logging.Tenant(tenant.OrgSlug, tenant.Vhost, "📪", "Message channel closed for org: %s, draining remaining messages and triggering resubscription", tenant.OrgSlug)

				// Drain any remaining messages in the channel (they will be requeued by RabbitMQ)
				for range messages {
					// Just drain, don't process or ACK
				}

				// Trigger resubscription for this tenant
				go c.resubscribeTenant(ctx, tenant)
				return
			}

			// Check if the delivery channel is still valid before processing
			// When the AMQP connection closes, the delivery becomes invalid
			if tenant.Channel == nil || tenant.Channel.IsClosed() {
				logging.Tenant(tenant.OrgSlug, tenant.Vhost, "⚠️", "Tenant channel closed, skipping message processing")
				go c.resubscribeTenant(ctx, tenant)
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

// handleMessage processes a single MQTT message
func (c *Consumer) handleMessage(msg amqp.Delivery, tenant *TenantConsumer) error {
	ctx := context.Background()
	orgSlug := tenant.OrgSlug
	vhost := tenant.Vhost

	// Create a span for the entire message processing
	tracer := telemetry.GetTracer("transformer-service")
	ctx, span := tracer.Start(ctx, "process_device_message")
	defer span.End()

	// Add span attributes
	span.SetAttributes(
		attribute.String("tenant", orgSlug),
		attribute.String("vhost", vhost),
		attribute.String("routing_key", msg.RoutingKey),
	)

	logging.Tenant(orgSlug, vhost, "⚙️", "Processing message from topic: %s", msg.RoutingKey)

	var err error
	var payload map[string]interface{}
	var locationPayload map[string]interface{}

	// Parse the incoming message
	payload, locationPayload, err = c.parser.Parse(msg)
	if err != nil {
		logging.Tenant(orgSlug, vhost, "❌", "Failed to parse message: %v", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to parse message")
		telemetry.LogError(ctx, fmt.Sprintf("[%s][VHOST:%s] Failed to parse message from routing key %s", orgSlug, vhost, msg.RoutingKey),
			otellog.String("tenant", orgSlug),
			otellog.String("vhost", vhost),
			otellog.String("routing_key", msg.RoutingKey),
			otellog.String("error", err.Error()),
		)
		return fmt.Errorf("failed to parse message: %w", err)
	}

	// Extract LNS type from metadata (must be explicitly set by MPA service)
	lnsType := c.parser.ExtractLNSSource(payload)

	// Log with LNS type for debugging
	if lnsType == lns.LNSTypeUnknown || lnsType == "" {
		logging.Tenant(orgSlug, vhost, "⚠️", "Unknown or empty LNS type, using fallback extraction")
	} else {
		logging.Tenant(orgSlug, vhost, "📡", "Processing message from LNS: %s", lnsType)
	}

	// Extract devEUI using LNS-aware extraction (efficient, single lookup)
	var devEUI string = components.ExtractDevEUI(payload, lnsType)

	// Check if device should be skipped
	deviceLocation, processingInfo, err := c.resolver.Resolve(orgSlug, vhost, devEUI, payload, locationPayload, lnsType)
	if errors.Is(err, resolver.ErrDeviceSkipped) {
		span.AddEvent("device_skipped")
		telemetry.LogInfo(ctx, fmt.Sprintf("[%s][VHOST:%s] Device %s skipped by resolver (should_skip=true)", orgSlug, vhost, devEUI),
			otellog.String("tenant", orgSlug),
			otellog.String("vhost", vhost),
			otellog.String("device_eui", devEUI),
		)
		return nil
	}
	if err != nil {
		logging.Tenant(orgSlug, vhost, "❌", "Failed to resolve device %s: %v", devEUI, err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to resolve device")
		telemetry.LogError(ctx, fmt.Sprintf("[%s][VHOST:%s] Failed to resolve device %s", orgSlug, vhost, devEUI),
			otellog.String("tenant", orgSlug),
			otellog.String("vhost", vhost),
			otellog.String("device_eui", devEUI),
			otellog.String("error", err.Error()),
		)
		if c.loggerService != nil {
			_ = c.loggerService.LogRawData(payload, locationPayload, *processingInfo)
		}
		return err
	}

	// Transform data to output format
	transformedData, err := c.transformService.TransformDeviceData(deviceLocation, processingInfo.GatewayCount, payload)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to transform device data")
		telemetry.LogError(ctx, fmt.Sprintf("[%s][VHOST:%s] Failed to transform data for device %s", orgSlug, vhost, devEUI),
			otellog.String("tenant", orgSlug),
			otellog.String("vhost", vhost),
			otellog.String("device_eui", devEUI),
			otellog.String("error", err.Error()),
		)
		return fmt.Errorf("failed to transform device data: %w", err)
	}

	// Add processing info to span
	span.SetAttributes(
		attribute.Bool("location_calculated", processingInfo.LocationCalculated),
		attribute.Int("gateway_count", processingInfo.GatewayCount),
	)
	span.AddEvent("device_data_transformed")

	locationValid := components.ValidateCoordinates(deviceLocation.Latitude, deviceLocation.Longitude) == nil

	// Try to parse sensor entities and publish telemetry payload
	var entityLocation *components.Location
	if locationValid {
		entityLocation = &components.Location{
			Latitude:  deviceLocation.Latitude,
			Longitude: deviceLocation.Longitude,
		}
	} else {
		logging.Tenant(orgSlug, vhost, "Invalid location for device %s (lat=%f, lon=%f): location entity will be excluded from telemetry", devEUI, deviceLocation.Latitude, deviceLocation.Longitude)
	}

	if parseResult, mapping, perr := c.parseEntities(orgSlug, devEUI, payload, entityLocation, lnsType); perr == nil && parseResult != nil {
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

	// Only publish transformed data (location-based) and set LocationResult when coordinates are valid
	if locationValid {
		processingInfo.LocationResult = &models.LocationResult{
			Latitude:  deviceLocation.Latitude,
			Longitude: deviceLocation.Longitude,
			Accuracy:  transformedData.Location.Accuracy,
		}

		logging.Tenant(orgSlug, vhost, "📤", "Publishing transformed data for device %s", deviceLocation.DevEUI)
		span.AddEvent("publishing_transformed_data")
		if err := c.publishTransformedData(tenant.Channel, transformedData, tenant); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to publish transformed data")
			return fmt.Errorf("failed to publish transformed data: %w", err)
		}
	}

	if c.loggerService != nil {
		logging.Tenant(orgSlug, vhost, "📝", "Logging raw data for device: %s, location calculated: %v", devEUI, processingInfo.LocationCalculated)
		if logErr := c.loggerService.LogRawData(payload, locationPayload, *processingInfo); logErr != nil {
			logging.Tenant(orgSlug, vhost, "❌", "Failed to log raw data: %v", logErr)
			telemetry.LogError(ctx, fmt.Sprintf("[%s][VHOST:%s] Failed to log raw data for device %s", orgSlug, vhost, devEUI),
				otellog.String("tenant", orgSlug),
				otellog.String("vhost", vhost),
				otellog.String("device_eui", devEUI),
				otellog.String("error", logErr.Error()),
			)
		}
	}

	logging.Tenant(orgSlug, vhost, "✅", "Successfully processed device: %s", deviceLocation.DevEUI)
	span.SetStatus(codes.Ok, "device message processed successfully")
	span.AddEvent("processing_completed")
	telemetry.LogInfo(ctx, fmt.Sprintf("[%s][VHOST:%s] Successfully processed device %s (gateways: %d, location: %v)", orgSlug, vhost, deviceLocation.DevEUI, processingInfo.GatewayCount, processingInfo.LocationCalculated),
		otellog.String("tenant", orgSlug),
		otellog.String("vhost", vhost),
		otellog.String("device_eui", deviceLocation.DevEUI),
		otellog.Bool("location_calculated", processingInfo.LocationCalculated),
		otellog.Int("gateway_count", processingInfo.GatewayCount),
	)
	return nil
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
		if d, ok := decoded["data"].(string); ok && d != "" {
			return d
		}
	}

	if d, ok := payload["data"].(string); ok && d != "" {
		return d
	}

	return ""
}
