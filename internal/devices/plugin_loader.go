package devices

import (
	"fmt"
	"os"
	"path/filepath"
	"plugin"
	"strings"
)

// PluginLoader handles dynamic loading of device parser plugins
type PluginLoader struct {
	pluginDir string
	registry  *Registry
}

// NewPluginLoader creates a new plugin loader
func NewPluginLoader(pluginDir string, registry *Registry) *PluginLoader {
	return &PluginLoader{
		pluginDir: pluginDir,
		registry:  registry,
	}
}

// LoadAll loads all plugins from the plugin directory
func (pl *PluginLoader) LoadAll() error {
	// Check if plugin directory exists
	if _, err := os.Stat(pl.pluginDir); os.IsNotExist(err) {
		// Create directory if it doesn't exist
		if err := os.MkdirAll(pl.pluginDir, 0750); err != nil {
			return fmt.Errorf("failed to create plugin directory: %w", err)
		}
		return nil // No plugins to load
	}

	// Read all files in plugin directory
	entries, err := os.ReadDir(pl.pluginDir)
	if err != nil {
		return fmt.Errorf("failed to read plugin directory: %w", err)
	}

	loadedCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		// Only load .so (Linux) and .dylib (macOS) files
		if !strings.HasSuffix(filename, ".so") && !strings.HasSuffix(filename, ".dylib") {
			continue
		}

		pluginPath := filepath.Join(pl.pluginDir, filename)
		if err := pl.LoadPlugin(pluginPath); err != nil {
			// Log error but continue loading other plugins
			fmt.Printf("Warning: failed to load plugin %s: %v\n", filename, err)
			continue
		}

		loadedCount++
	}

	fmt.Printf("Loaded %d device parser plugins from %s\n", loadedCount, pl.pluginDir)
	return nil
}

// LoadPlugin loads a single plugin file
func (pl *PluginLoader) LoadPlugin(pluginPath string) error {
	// Open the plugin
	p, err := plugin.Open(pluginPath)
	if err != nil {
		return fmt.Errorf("failed to open plugin: %w", err)
	}

	// Look for the NewParser symbol
	newParserSymbol, err := p.Lookup("NewParser")
	if err != nil {
		return fmt.Errorf("plugin does not export NewParser: %w", err)
	}

	// Type assert to the correct function signature
	newParserFunc, ok := newParserSymbol.(func() DeviceParser)
	if !ok {
		return fmt.Errorf("NewParser has incorrect signature, expected func() DeviceParser")
	}

	// Create parser instance
	parser := newParserFunc()
	if parser == nil {
		return fmt.Errorf("NewParser returned nil")
	}

	// Register the parser
	if err := pl.registry.Register(parser); err != nil {
		return fmt.Errorf("failed to register parser: %w", err)
	}

	fmt.Printf("Successfully loaded plugin: %s (device type: %s)\n",
		filepath.Base(pluginPath), parser.GetDeviceType())

	return nil
}

// LoadPluginFromPath loads a plugin from an explicit path (outside plugin directory)
func (pl *PluginLoader) LoadPluginFromPath(pluginPath string) error {
	return pl.LoadPlugin(pluginPath)
}
