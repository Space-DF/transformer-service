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
	amqp "github.com/rabbitmq/amqp091-go"
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

			if err := c.handleMessage(ctx, msg, tenant); err != nil {
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
func (c *Consumer) handleMessage(ctx context.Context, msg amqp.Delivery, tenant *TenantConsumer) error {
	logging.Tenant(tenant.OrgSlug, tenant.Vhost, "⚙️", "Processing message from topic: %s", msg.RoutingKey)

	payload, locationPayload, lnsType, devEUI, err := c.parseMessage(msg, tenant)
	if err != nil {
		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "❌", "Failed to parse message: %v", err)
		return fmt.Errorf("failed to parse message: %w", err)
	}

	// If LNS event message, parse and route accordingly
	handled, err := c.parseAndRouteLNSEventMessage(ctx, tenant, payload, lnsType, devEUI)

	if err != nil {
		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "❌", "Failed to parse and route LNS event message: %v", err)
		return fmt.Errorf("failed to parse and route LNS event message: %w", err)
	}
	if handled {
		return nil
	}

	// Check if device should be skipped
	deviceLocation, processingInfo, err := c.resolveMessage(ctx, tenant, devEUI, payload, locationPayload, lnsType)
	if err != nil {
		if errors.Is(err, resolver.ErrDeviceSkipped) {
			return nil // not an error, just skip
		}
		return fmt.Errorf("failed to resolve message: %w", err)
	}

	// Transform data to output format and publish to output topic
	if err := c.transformAndPublish(tenant, deviceLocation, payload, processingInfo); err != nil {
		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "❌", "Failed to transform and publish data for device %s: %v", deviceLocation.DevEUI, err)
		return fmt.Errorf("failed to transform and publish data: %w", err)
	}

	// process entities and publish telemetry
	c.processEntities(tenant, deviceLocation, payload, lnsType)

	c.logAndPublishRaw(tenant, payload, deviceLocation.DevEUI, processingInfo)

	logging.Tenant(tenant.OrgSlug, tenant.Vhost, "✅", "Successfully processed device: %s", deviceLocation.DevEUI)
	return nil
}

func (c *Consumer) parseMessage(msg amqp.Delivery, tenant *TenantConsumer) (payload map[string]interface{}, locationPayload map[string]interface{}, lnsType lns.LNSType, devEUI string, err error) {
	// Parse the incoming message
	payload, locationPayload, err = c.parser.Parse(msg)
	if err != nil {
		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "❌", "Failed to parse message: %v", err)
		return nil, nil, "", "", fmt.Errorf("failed to parse message: %w", err)
	}

	// Extract LNS type from metadata
	lnsType = c.parser.ExtractLNSSource(payload)
	if lnsType == lns.LNSTypeUnknown || lnsType == "" {
		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "⚠️", "Unknown or empty LNS type, using fallback extraction")
	}

	logging.Tenant(tenant.OrgSlug, tenant.Vhost, "📡", "Processing message from LNS: %s", lnsType)

	// Extract DevEUI using LNS-specific logic
	devEUI = lns.ExtractDevEUI(payload, lnsType)

	return payload, locationPayload, lnsType, devEUI, nil
}

func (c *Consumer) processEntities(tenant *TenantConsumer, deviceLocation *models.DeviceLocationData, payload map[string]interface{}, lnsType lns.LNSType) {
	// Try to parse sensor entities and publish telemetry payload if location valid
	// otherwise skip entity parsing and just publish transformed data without location entity
	locationValid := common.ValidateCoordinates(deviceLocation.Latitude, deviceLocation.Longitude) == nil

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
		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "⚠️", "Invalid location for device %s (lat=%f, lon=%f): location entity will be excluded from telemetry", deviceLocation.DevEUI, deviceLocation.Latitude, deviceLocation.Longitude)
	}

	parseResult, mapping, perr := c.parseEntities(tenant.OrgSlug, deviceLocation.DevEUI, payload, entityLocation, lnsType)
	if perr == nil && parseResult != nil {
		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "", "Parsed %d telemetry entities for device %s", len(parseResult.Entities), deviceLocation.DevEUI)
	}
	if perr != nil {
		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "⚠️", "Failed to parse telemetry entities: %v", perr)
		return
	}
	if parseResult == nil {
		return
	}

	telemetryPayload, terr := c.buildTelemetryPayload(parseResult, tenant.OrgSlug, mapping)
	if terr == nil && telemetryPayload != nil {
		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "📊", "Built telemetry payload with %d entities for device %s", len(parseResult.Entities), deviceLocation.DevEUI)
	}
	if terr != nil {
		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "⚠️", "Failed to build telemetry payload: %v", terr)
		return
	}
	if telemetryPayload == nil {
		return
	}

	err := c.publishTelemetry(tenant.Channel, telemetryPayload, tenant)
	if err != nil {
		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "⚠️", "Failed to publish telemetry payload: %v", err)
		return
	}
	logging.Tenant(tenant.OrgSlug, tenant.Vhost, "✅", "Published telemetry payload with %d entities", len(parseResult.Entities))
}

func (c *Consumer) logAndPublishRaw(tenant *TenantConsumer, payload map[string]interface{}, devEUI string, processingInfo *models.ProcessingInfo) {
	if c.loggerService == nil {
		return
	}

	logging.Tenant(tenant.OrgSlug, tenant.Vhost, "📝", "Logging raw data for device: %s, location calculated: %v", devEUI, processingInfo.LocationCalculated)

	logEntry, logErr := c.loggerService.LogRawData(payload, devEUI, *processingInfo)
	if err := c.publishRawLog(tenant, logEntry); err != nil {
		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "⚠️", "Failed to publish raw log to telemetry service: %v", err)
	}

	if logErr != nil {
		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "⚠️", "Failed to log raw data for device %s: %v", devEUI, logErr)
	}
}

func (c *Consumer) transformAndPublish(tenant *TenantConsumer, deviceLocation *models.DeviceLocationData, payload map[string]interface{}, processingInfo *models.ProcessingInfo) error {
	transformedData, err := c.transformService.TransformDeviceData(deviceLocation, processingInfo.GatewayCount, payload)
	if err != nil {
		return fmt.Errorf("failed to transform device data: %w", err)
	}

	processingInfo.LocationResult = &models.LocationResult{
		Latitude:  deviceLocation.Latitude,
		Longitude: deviceLocation.Longitude,
		Accuracy:  transformedData.Location.Accuracy,
		Bearing:   transformedData.Location.Bearing,
	}

	// Publish transformed data to output topic
	logging.Tenant(tenant.OrgSlug, tenant.Vhost, "📤", "Publishing transformed data for device %s", deviceLocation.DevEUI)
	if err := c.publishTransformedData(tenant.Channel, transformedData, tenant); err != nil {
		return fmt.Errorf("failed to publish transformed data: %w", err)
	}

	return nil
}

func (c *Consumer) resolveMessage(ctx context.Context, tenant *TenantConsumer, devEUI string, payload map[string]interface{}, locationPayload map[string]interface{}, lnsType lns.LNSType) (*models.DeviceLocationData, *models.ProcessingInfo, error) {
	deviceLocation, processingInfo, err := c.resolver.Resolve(ctx, tenant.OrgSlug, tenant.Vhost, devEUI, payload, locationPayload, lnsType)
	if errors.Is(err, resolver.ErrDeviceSkipped) {
		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "⏭️", "Device %s is marked to be skipped, skipping processing", devEUI)
		return nil, nil, err
	}
	if err != nil {
		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "❌", "Failed to resolve device %s: %v", devEUI, err)
		if c.loggerService != nil {
			c.logAndPublishRaw(tenant, payload, devEUI, processingInfo)
		}
		return nil, nil, fmt.Errorf("failed to resolve device: %w", err)
	}

	return deviceLocation, processingInfo, nil
}

func (c *Consumer) parseAndRouteLNSEventMessage(ctx context.Context, tenant *TenantConsumer, payload map[string]interface{}, lnsType lns.LNSType, devEUI string) (bool, error) {
	eventType := lns.ExtractEventType(payload, lnsType)

	switch eventType {
	case lns.EventJoin:
		return true, c.handleJoinEvent(ctx, tenant, payload, lnsType, devEUI)
	case lns.EventAlert:
		return true, c.handleAlertEvent(ctx, tenant, payload, lnsType, devEUI)
	case lns.EventStatus, lns.EventAck:
		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "ℹ️", "Received %s event for device %s, skipping processing", eventType, devEUI)
		return true, nil
	case lns.EventUplink, lns.EventUnknown:
		return false, nil
	}

	return false, nil
}

func (c *Consumer) handleJoinEvent(ctx context.Context, tenant *TenantConsumer, payload map[string]interface{}, lnsType lns.LNSType, devEUI string) error {
	logging.Tenant(tenant.OrgSlug, tenant.Vhost, "🔗", "Received join event for device %s from %s", devEUI, lnsType)

	deviceID, spaceSlug := c.lookupDeviceForEvent(tenant.OrgSlug, tenant.Vhost, devEUI)

	event := map[string]interface{}{
		"event_type":    "lns_join",
		"event_level":   "lns_join",
		"organization":  tenant.OrgSlug,
		"space_slug":    spaceSlug,
		"device_id":     deviceID,
		"title":         fmt.Sprintf("Device joined via %s", lnsType),
		"time_fired_ts": time.Now().UnixMilli(),
		"source":        string(lnsType),
	}

	if err := c.publishLNSEvent(tenant, event, tenant.OrgSlug, spaceSlug, deviceID); err != nil {
		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "❌", "Failed to publish join event for device %s: %v", devEUI, err)
		return fmt.Errorf("failed to publish join event: %w", err)
	}

	logging.Tenant(tenant.OrgSlug, tenant.Vhost, "✅", "Published join event for device %s", devEUI)
	return nil
}

func (c *Consumer) handleAlertEvent(ctx context.Context, tenant *TenantConsumer, payload map[string]interface{}, lnsType lns.LNSType, devEUI string) error {
	alert := lns.ExtractAlert(payload, lnsType)
	if alert == nil {
		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "⚠️", "Alert event detected but no alert extracted for device %s", devEUI)
		return nil
	}

	logging.Tenant(tenant.OrgSlug, tenant.Vhost, "🚨", "Received alert from %s for device %s: [%s] %s - %s", lnsType, devEUI, alert.Level, alert.Code, alert.Message)

	deviceID, spaceSlug := c.lookupDeviceForEvent(tenant.OrgSlug, tenant.Vhost, devEUI)

	event := map[string]interface{}{
		"event_type":    "lns_alert",
		"event_level":   "lns_alert",
		"organization":  tenant.OrgSlug,
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

	if err := c.publishLNSEvent(tenant, event, tenant.OrgSlug, spaceSlug, deviceID); err != nil {
		logging.Tenant(tenant.OrgSlug, tenant.Vhost, "❌", "Failed to publish alert event for device %s: %v", devEUI, err)
		return err
	}

	logging.Tenant(tenant.OrgSlug, tenant.Vhost, "✅", "Published alert event for device %s: [%s] %s", devEUI, alert.Level, alert.Code)
	return nil
}

func (c *Consumer) shouldSkipUnpublishedUnassigned(orgSlug string, vhost string, devEUI string) bool {
	mapping := c.lookupDeviceMapping(orgSlug, vhost, devEUI)
	return mapping != nil && mapping.SpaceSlug == "" && !mapping.IsPublished
}

func (c *Consumer) lookupDeviceMapping(orgSlug string, vhost string, devEUI string) *models.DeviceMapping {
	if c.deviceProfileService == nil || devEUI == "" {
		return nil
	}

	mapping, err := c.deviceProfileService.GetDeviceMapping(orgSlug, devEUI)
	if err != nil {
		logging.Tenant(orgSlug, vhost, "⚠️", "Could not lookup device %s for LNS event: %v", devEUI, err)
		return nil
	}

	return mapping
}

func (c *Consumer) lookupDeviceForEvent(orgSlug string, vhost string, devEUI string) (deviceID string, spaceSlug string) {
	mapping := c.lookupDeviceMapping(orgSlug, vhost, devEUI)
	if mapping == nil {
		return "", ""
	}

	return mapping.DeviceID, mapping.SpaceSlug
}
