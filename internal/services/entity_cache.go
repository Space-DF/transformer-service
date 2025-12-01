package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/Space-DF/transformer-service/internal/components"
)

// EntityCacheService handles entity storage and retrieval with Redis
type EntityCacheService struct {
	redis       *redis.Client
	defaultTTL  time.Duration
}

// NewEntityCacheService creates a new entity cache service
func NewEntityCacheService(redisClient *redis.Client) *EntityCacheService {
	return &EntityCacheService{
		redis:      redisClient,
		defaultTTL: 24 * time.Hour, // Default 24-hour entity TTL
	}
}

// StoreEntities stores multiple entities for a device in Redis
func (e *EntityCacheService) StoreEntities(ctx context.Context, orgSlug string, entities []components.Entity) error {
	pipe := e.redis.Pipeline()

	for _, entity := range entities {
		// Store by unique_id (primary key)
		uniqueKey := fmt.Sprintf("tenant:%s:entity:%s", orgSlug, entity.UniqueID)
		entityJSON, err := json.Marshal(entity)
		if err != nil {
			return fmt.Errorf("failed to marshal entity %s: %v", entity.UniqueID, err)
		}
		pipe.Set(ctx, uniqueKey, entityJSON, e.defaultTTL)

		// Create index by entity_id for lookups
		entityIDKey := fmt.Sprintf("tenant:%s:entity_id:%s", orgSlug, entity.EntityID)
		pipe.Set(ctx, entityIDKey, entity.UniqueID, e.defaultTTL)

		// Create index by device EUI for device-level queries
		deviceKey := fmt.Sprintf("tenant:%s:device_entities:%s", orgSlug, extractDevEUIFromUniqueID(entity.UniqueID))
		pipe.SAdd(ctx, deviceKey, entity.UniqueID)
		pipe.Expire(ctx, deviceKey, e.defaultTTL)
	}

	_, err := pipe.Exec(ctx)
	return err
}

// GetEntity retrieves a single entity by unique ID
func (e *EntityCacheService) GetEntity(ctx context.Context, orgSlug, uniqueID string) (*components.Entity, error) {
	key := fmt.Sprintf("tenant:%s:entity:%s", orgSlug, uniqueID)
	
	result := e.redis.Get(ctx, key)
	if result.Err() != nil {
		if result.Err() == redis.Nil {
			return nil, fmt.Errorf("entity not found: %s", uniqueID)
		}
		return nil, result.Err()
	}

	var entity components.Entity
	if err := json.Unmarshal([]byte(result.Val()), &entity); err != nil {
		return nil, fmt.Errorf("failed to unmarshal entity: %v", err)
	}

	return &entity, nil
}

// GetEntityByID retrieves an entity by entity_id
func (e *EntityCacheService) GetEntityByID(ctx context.Context, orgSlug, entityID string) (*components.Entity, error) {
	// First get the unique_id from entity_id index
	entityIDKey := fmt.Sprintf("tenant:%s:entity_id:%s", orgSlug, entityID)
	uniqueID := e.redis.Get(ctx, entityIDKey).Val()
	if uniqueID == "" {
		return nil, fmt.Errorf("entity not found by entity_id: %s", entityID)
	}

	return e.GetEntity(ctx, orgSlug, uniqueID)
}

// GetDeviceEntities retrieves all entities for a specific device
func (e *EntityCacheService) GetDeviceEntities(ctx context.Context, orgSlug, deviceEUI string) ([]components.Entity, error) {
	deviceKey := fmt.Sprintf("tenant:%s:device_entities:%s", orgSlug, deviceEUI)
	
	uniqueIDs := e.redis.SMembers(ctx, deviceKey).Val()
	if len(uniqueIDs) == 0 {
		return []components.Entity{}, nil
	}

	var entities []components.Entity
	for _, uniqueID := range uniqueIDs {
		entity, err := e.GetEntity(ctx, orgSlug, uniqueID)
		if err != nil {
			// Log error but continue with other entities
			continue
		}
		entities = append(entities, *entity)
	}

	return entities, nil
}

// GetEntitiesByType retrieves all entities of a specific type for an organization
func (e *EntityCacheService) GetEntitiesByType(ctx context.Context, orgSlug, entityType string) ([]components.Entity, error) {
	pattern := fmt.Sprintf("tenant:%s:entity:*_%s", orgSlug, entityType)
	keys := e.redis.Keys(ctx, pattern).Val()

	var entities []components.Entity
	for _, key := range keys {
		result := e.redis.Get(ctx, key)
		if result.Err() != nil {
			continue
		}

		var entity components.Entity
		if err := json.Unmarshal([]byte(result.Val()), &entity); err != nil {
			continue
		}
		entities = append(entities, entity)
	}

	return entities, nil
}

// UpdateEntityState updates only the state and timestamp of an entity
func (e *EntityCacheService) UpdateEntityState(ctx context.Context, orgSlug, uniqueID string, state interface{}, attributes map[string]interface{}) error {
	entity, err := e.GetEntity(ctx, orgSlug, uniqueID)
	if err != nil {
		return err
	}

	// Update state and timestamp
	entity.State = state
	entity.Timestamp = time.Now().UTC()
	
	// Merge attributes if provided
	if attributes != nil {
		if entity.Attributes == nil {
			entity.Attributes = make(map[string]interface{})
		}
		for k, v := range attributes {
			entity.Attributes[k] = v
		}
	}

	// Store updated entity
	return e.StoreEntities(ctx, orgSlug, []components.Entity{*entity})
}

// DeleteEntity removes an entity and its indices
func (e *EntityCacheService) DeleteEntity(ctx context.Context, orgSlug, uniqueID string) error {
	pipe := e.redis.Pipeline()

	// Remove main entity
	entityKey := fmt.Sprintf("tenant:%s:entity:%s", orgSlug, uniqueID)
	pipe.Del(ctx, entityKey)

	// Remove from device set
	deviceEUI := extractDevEUIFromUniqueID(uniqueID)
	deviceKey := fmt.Sprintf("tenant:%s:device_entities:%s", orgSlug, deviceEUI)
	pipe.SRem(ctx, deviceKey, uniqueID)

	// Note: entity_id index will expire naturally

	_, err := pipe.Exec(ctx)
	return err
}

// DeleteDeviceEntities removes all entities for a device
func (e *EntityCacheService) DeleteDeviceEntities(ctx context.Context, orgSlug, deviceEUI string) error {
	entities, err := e.GetDeviceEntities(ctx, orgSlug, deviceEUI)
	if err != nil {
		return err
	}

	pipe := e.redis.Pipeline()
	for _, entity := range entities {
		entityKey := fmt.Sprintf("tenant:%s:entity:%s", orgSlug, entity.UniqueID)
		pipe.Del(ctx, entityKey)
		
		entityIDKey := fmt.Sprintf("tenant:%s:entity_id:%s", orgSlug, entity.EntityID)
		pipe.Del(ctx, entityIDKey)
	}

	// Remove device set
	deviceKey := fmt.Sprintf("tenant:%s:device_entities:%s", orgSlug, deviceEUI)
	pipe.Del(ctx, deviceKey)

	_, err = pipe.Exec(ctx)
	return err
}

// GetEntityStats returns statistics about cached entities
func (e *EntityCacheService) GetEntityStats(ctx context.Context, orgSlug string) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Count total entities
	entityPattern := fmt.Sprintf("tenant:%s:entity:*", orgSlug)
	totalKeys := e.redis.Keys(ctx, entityPattern).Val()
	stats["total_entities"] = len(totalKeys)

	// Count by entity type
	entityTypes := make(map[string]int)
	for _, key := range totalKeys {
		result := e.redis.Get(ctx, key)
		if result.Err() != nil {
			continue
		}

		var entity components.Entity
		if err := json.Unmarshal([]byte(result.Val()), &entity); err != nil {
			continue
		}
		entityTypes[entity.EntityType]++
	}
	stats["entity_types"] = entityTypes

	// Count devices
	devicePattern := fmt.Sprintf("tenant:%s:device_entities:*", orgSlug)
	deviceKeys := e.redis.Keys(ctx, devicePattern).Val()
	stats["total_devices"] = len(deviceKeys)

	return stats, nil
}

// extractDevEUIFromUniqueID extracts DevEUI from unique_id format: "orgslug_deveui_entitytype"
func extractDevEUIFromUniqueID(uniqueID string) string {
	// Split by underscore and get the middle part (DevEUI)
	parts := splitUniqueID(uniqueID)
	if len(parts) >= 2 {
		return parts[1] // DevEUI is the second part
	}
	return ""
}

// splitUniqueID splits unique_id into [orgSlug, devEUI, entityType]
func splitUniqueID(uniqueID string) []string {
	// This is a simple implementation - in practice you might need more robust parsing
	// Format: "orgslug_deveui_entitytype"
	parts := make([]string, 0, 3)
	currentPart := ""
	underscoreCount := 0
	
	for _, char := range uniqueID {
		if char == '_' {
			underscoreCount++
			if underscoreCount <= 2 { // Only split on first 2 underscores
				parts = append(parts, currentPart)
				currentPart = ""
				continue
			}
		}
		currentPart += string(char)
	}
	parts = append(parts, currentPart) // Add the last part
	
	return parts
}