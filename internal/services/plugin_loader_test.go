package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewPluginLoader(t *testing.T) {
	registry := NewRegistry()
	loader := NewPluginLoader("./plugins", registry)

	if loader == nil {
		t.Fatal("NewPluginLoader returned nil")
	}

	if loader.pluginDir != "./plugins" {
		t.Errorf("Expected plugin dir './plugins', got '%s'", loader.pluginDir)
	}
}

func TestPluginLoader_LoadAll_CreatesDirectory(t *testing.T) {
	// Use temp directory for testing
	tempDir := t.TempDir()
	pluginDir := filepath.Join(tempDir, "plugins")

	registry := NewRegistry()
	loader := NewPluginLoader(pluginDir, registry)

	err := loader.LoadAll()
	if err != nil {
		t.Errorf("LoadAll failed: %v", err)
	}

	// Check directory was created
	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		t.Error("Plugin directory was not created")
	}
}

func TestPluginLoader_LoadAll_EmptyDirectory(t *testing.T) {
	tempDir := t.TempDir()

	registry := NewRegistry()
	loader := NewPluginLoader(tempDir, registry)

	err := loader.LoadAll()
	if err != nil {
		t.Errorf("LoadAll failed on empty directory: %v", err)
	}

	// Should have no parsers registered
	parsers := registry.ListRegisteredParsers()
	if len(parsers) != 0 {
		t.Errorf("Expected 0 parsers, got %d", len(parsers))
	}
}

// Note: Testing actual plugin loading requires building .so/.dylib files
// which is complex in unit tests. Integration tests should cover this.
