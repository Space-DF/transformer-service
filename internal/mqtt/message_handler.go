package mqtt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
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
	}

	logging.Tenant(orgSlug, vhost, "📡", "Processing message from LNS: %s", lnsType)

	var devEUI string = lns.ExtractDevEUI(payload, lnsType)

	eventType := lns.ExtractEventType(payload, lnsType)
	switch eventType {
	case lns.EventJoin:
		return c.handleJoinEvent(ctx, orgSlug, vhost, tenant, payload, lnsType, devEUI)
	case lns.EventAlert:
		return c.handleAlertEvent(ctx, orgSlug, vhost, tenant, payload, lnsType, devEUI)
	case lns.EventStatus, lns.EventAck:
		logging.Tenant(orgSlug, vhost, "ℹ️", "Received %s event for device %s, skipping processing", eventType, devEUI)
		return nil
	case lns.EventUplink, lns.EventUnknown:
	}

	// Check if device should be skipped
	deviceLocation, processingInfo, err := c.resolver.Resolve(ctx, orgSlug, vhost, devEUI, payload, locationPayload, lnsType)
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

	// Skip unpublished devices that are not assigned to a space.
	if transformedData.SpaceSlug == "" && !transformedData.IsPublished {
		logging.Tenant(orgSlug, vhost, "⏭️", "Skipping publish for unpublished unassigned device %s", devEUI)
		span.SetStatus(codes.Ok, "device message skipped: unpublished and unassigned")
		span.AddEvent("publish_skipped_unpublished_unassigned")
		return nil
	}

	locationValid := common.ValidateCoordinates(deviceLocation.Latitude, deviceLocation.Longitude) == nil

	// Try to parse sensor entities and publish telemetry payload
	var entityLocation *common.Location
	if locationValid {
		entityLocation = &common.Location{
			Latitude:  deviceLocation.Latitude,
			Longitude: deviceLocation.Longitude,
		}
		if deviceLocation.Bearing != nil {
			entityLocation.Bearing = *deviceLocation.Bearing
		}
	} else {
		logging.Tenant(orgSlug, vhost, "⚠️", "Invalid location for device %s (lat=%f, lon=%f): location entity will be excluded from telemetry", devEUI, deviceLocation.Latitude, deviceLocation.Longitude)
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

	processingInfo.LocationResult = &models.LocationResult{
		Latitude:  deviceLocation.Latitude,
		Longitude: deviceLocation.Longitude,
		Accuracy:  transformedData.Location.Accuracy,
		Bearing:   transformedData.Location.Bearing,
	}

	// Publish transformed data to output topic
	logging.Tenant(orgSlug, vhost, "📤", "Publishing transformed data for device %s", deviceLocation.DevEUI)
	span.AddEvent("publishing_transformed_data")
	if err := c.publishTransformedData(tenant.Channel, transformedData, tenant); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to publish transformed data")
		return fmt.Errorf("failed to publish transformed data: %w", err)
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

func (c *Consumer) handleJoinEvent(ctx context.Context, orgSlug string, vhost string, tenant *TenantConsumer, payload map[string]interface{}, lnsType lns.LNSType, devEUI string) error {
	logging.Tenant(orgSlug, vhost, "🔗", "Received join event for device %s from %s", devEUI, lnsType)

	deviceID, spaceSlug := c.lookupDeviceForEvent(orgSlug, vhost, devEUI)

	event := map[string]interface{}{
		"event_type":    "lns_join",
		"event_level":   "lns_join",
		"organization":  orgSlug,
		"space_slug":    spaceSlug,
		"device_id":     deviceID,
		"title":         fmt.Sprintf("Device joined via %s", lnsType),
		"time_fired_ts": time.Now().UnixMilli(),
		"source":        string(lnsType),
	}

	if err := c.publishLNSEvent(tenant, event, orgSlug, spaceSlug, deviceID); err != nil {
		logging.Tenant(orgSlug, vhost, "❌", "Failed to publish join event for device %s: %v", devEUI, err)
		return err
	}

	logging.Tenant(orgSlug, vhost, "✅", "Published join event for device %s", devEUI)
	return nil
}

func (c *Consumer) handleAlertEvent(ctx context.Context, orgSlug string, vhost string, tenant *TenantConsumer, payload map[string]interface{}, lnsType lns.LNSType, devEUI string) error {
	alert := lns.ExtractAlert(payload, lnsType)
	if alert == nil {
		logging.Tenant(orgSlug, vhost, "⚠️", "Alert event detected but no alert extracted for device %s", devEUI)
		return nil
	}

	logging.Tenant(orgSlug, vhost, "🚨", "Received alert from %s for device %s: [%s] %s - %s", lnsType, devEUI, alert.Level, alert.Code, alert.Message)

	deviceID, spaceSlug := c.lookupDeviceForEvent(orgSlug, vhost, devEUI)

	event := map[string]interface{}{
		"event_type":    "lns_alert",
		"event_level":   "lns_alert",
		"organization":  orgSlug,
		"space_slug":    spaceSlug,
		"device_id":     deviceID,
		"title":         fmt.Sprintf("[%s] %s: %s", alert.Level, alert.Code, alert.Message),
		"time_fired_ts": time.Now().UnixMilli(),
		"lns_alert": map[string]interface{}{
			"code":    alert.Code,
			"level":   alert.Level,
			"message": alert.Message,
			"source":  alert.Source,
		},
	}

	if err := c.publishLNSEvent(tenant, event, orgSlug, spaceSlug, deviceID); err != nil {
		logging.Tenant(orgSlug, vhost, "❌", "Failed to publish alert event for device %s: %v", devEUI, err)
		return err
	}

	logging.Tenant(orgSlug, vhost, "✅", "Published alert event for device %s: [%s] %s", devEUI, alert.Level, alert.Code)
	return nil
}

func (c *Consumer) lookupDeviceForEvent(orgSlug string, vhost string, devEUI string) (deviceID string, spaceSlug string) {
	if c.deviceProfileService == nil || devEUI == "" {
		return "", ""
	}

	mapping, err := c.deviceProfileService.GetDeviceMapping(orgSlug, devEUI)
	if err != nil {
		logging.Tenant(orgSlug, vhost, "⚠️", "Could not lookup device %s for LNS event: %v", devEUI, err)
		return "", ""
	}

	return mapping.DeviceID, mapping.SpaceSlug
}
