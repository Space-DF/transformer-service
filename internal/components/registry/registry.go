package registry

import (
	"context"
	"fmt"
	"sync"

	"github.com/Space-DF/transformer-service/internal/components"
)

// ComponentRegistry manages registered device components
// This follows a component registry pattern
type ComponentRegistry struct {
	components map[string]components.DeviceComponent
	deviceMap  map[components.DeviceType][]string // Maps device type to component names
	mutex      sync.RWMutex
}

// NewComponentRegistry creates a new component registry
func NewComponentRegistry() *ComponentRegistry {
	return &ComponentRegistry{
		components: make(map[string]components.DeviceComponent),
		deviceMap:  make(map[components.DeviceType][]string),
	}
}

// RegisterComponent registers a new device component
func (r *ComponentRegistry) RegisterComponent(name string, component components.DeviceComponent) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, exists := r.components[name]; exists {
		return fmt.Errorf("component %s already registered", name)
	}

	r.components[name] = component

	// Update device type mapping
	for _, deviceType := range component.GetSupportedDevices() {
		r.deviceMap[deviceType] = append(r.deviceMap[deviceType], name)
	}

	// Setup component if it supports it
	if setupComponent, ok := component.(components.ComponentWithSetup); ok {
		if err := setupComponent.Setup(context.Background()); err != nil {
			// Rollback registration on setup failure
			delete(r.components, name)
			r.removeFromDeviceMap(name, component.GetSupportedDevices())
			return fmt.Errorf("failed to setup component %s: %w", name, err)
		}
	}

	return nil
}

// UnregisterComponent removes a component from the registry
func (r *ComponentRegistry) UnregisterComponent(name string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	component, exists := r.components[name]
	if !exists {
		return fmt.Errorf("component %s not found", name)
	}

	// Teardown component if it supports it
	if teardownComponent, ok := component.(components.ComponentWithSetup); ok {
		if err := teardownComponent.Teardown(context.Background()); err != nil {
			return fmt.Errorf("failed to teardown component %s: %w", name, err)
		}
	}

	delete(r.components, name)
	r.removeFromDeviceMap(name, component.GetSupportedDevices())

	return nil
}

// GetComponent returns a registered component by name
func (r *ComponentRegistry) GetComponent(name string) (components.DeviceComponent, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	component, exists := r.components[name]
	return component, exists
}

// GetComponentsForDevice returns all components that support a specific device type
func (r *ComponentRegistry) GetComponentsForDevice(deviceType components.DeviceType) []components.DeviceComponent {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	var result []components.DeviceComponent
	if componentNames, exists := r.deviceMap[deviceType]; exists {
		for _, name := range componentNames {
			if component, found := r.components[name]; found {
				result = append(result, component)
			}
		}
	}

	return result
}

// FindComponentForPayload finds the best component to handle a specific payload
func (r *ComponentRegistry) FindComponentForPayload(deviceType components.DeviceType, payload *components.RawPayload) components.DeviceComponent {
	components := r.GetComponentsForDevice(deviceType)

	// Return the first component that can handle this payload
	for _, component := range components {
		if component.CanHandle(deviceType, payload) {
			return component
		}
	}

	return nil
}

// ListComponents returns all registered component names
func (r *ComponentRegistry) ListComponents() []string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	var names []string
	for name := range r.components {
		names = append(names, name)
	}

	return names
}

// GetComponentInfo returns information about all registered components
func (r *ComponentRegistry) GetComponentInfo() map[string]components.ComponentInfo {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	info := make(map[string]components.ComponentInfo)
	for name, component := range r.components {
		info[name] = component.GetInfo()
	}

	return info
}

// removeFromDeviceMap removes a component from the device type mapping
func (r *ComponentRegistry) removeFromDeviceMap(componentName string, deviceTypes []components.DeviceType) {
	for _, deviceType := range deviceTypes {
		if componentNames, exists := r.deviceMap[deviceType]; exists {
			// Remove the component name from the slice
			for i, name := range componentNames {
				if name == componentName {
					r.deviceMap[deviceType] = append(componentNames[:i], componentNames[i+1:]...)
					break
				}
			}

			// Clean up empty slices
			if len(r.deviceMap[deviceType]) == 0 {
				delete(r.deviceMap, deviceType)
			}
		}
	}
}

// Global registry instance
var globalRegistry = NewComponentRegistry()

// GetGlobalRegistry returns the global component registry instance
func GetGlobalRegistry() *ComponentRegistry {
	return globalRegistry
}

// RegisterComponent registers a component in the global registry
func RegisterComponent(name string, component components.DeviceComponent) error {
	return globalRegistry.RegisterComponent(name, component)
}

// FindComponent finds a component for the given device type and payload
func FindComponent(deviceType components.DeviceType, payload *components.RawPayload) components.DeviceComponent {
	return globalRegistry.FindComponentForPayload(deviceType, payload)
}
