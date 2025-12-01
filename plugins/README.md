# Device Parser Plugins

This directory contains dynamically loadable device parser plugins (`.so` or `.dylib` files).

## How It Works

The transformer service automatically loads all `.so` (Linux) and `.dylib` (macOS) files from this directory at startup.

## Plugin Requirements

Each plugin must export a `NewParser` function with this signature:

```go
func NewParser() devices.DeviceParser
```

## Creating a Plugin

### Step 1: Create Plugin Source

Create a separate Go module for your plugin:

```bash
mkdir -p my-device-plugin
cd my-device-plugin
go mod init my-device-plugin
```

Create `main.go`:

```go
package main

import (
    "context"
    "github.com/Space-DF/transformer-service/internal/devices"
)

// Parser implements devices.DeviceParser
type Parser struct{}

func (p *Parser) GetDeviceType() devices.DeviceType {
    return "MY_DEVICE"
}

func (p *Parser) CanParse(payload *devices.RawPayload) bool {
    return payload.FPort == 3 // Example
}

func (p *Parser) Parse(ctx context.Context, payload *devices.RawPayload) (*devices.ParsedData, error) {
    // Implementation here
    return &devices.ParsedData{
        DeviceEUI:  payload.DeviceEUI,
        DeviceType: "MY_DEVICE",
        Timestamp:  payload.Timestamp,
        SensorData: make(map[string]interface{}),
    }, nil
}

func (p *Parser) Validate(data *devices.ParsedData) error {
    return nil
}

// NewParser is the exported symbol that the plugin loader looks for
func NewParser() devices.DeviceParser {
    return &Parser{}
}
```

### Step 2: Build the Plugin

**On macOS:**
```bash
go build -buildmode=plugin -o my-device.dylib main.go
```

**On Linux:**
```bash
go build -buildmode=plugin -o my-device.so main.go
```

### Step 3: Deploy the Plugin

Copy the built plugin to this directory:

```bash
cp my-device.dylib /path/to/transformer-service/plugins/
# or
cp my-device.so /path/to/transformer-service/plugins/
```

### Step 4: Restart the Service

The transformer service will automatically load the plugin on startup.

## Example: RAK2270 as a Plugin

While RAK2270 is currently built-in, here's how it would look as a plugin:

```bash
# In rak2270-plugin directory
cat > main.go << 'EOF'
package main

import (
    "github.com/Space-DF/transformer-service/internal/components/rakwireless"
    "github.com/Space-DF/transformer-service/internal/components"
)

func NewComponent() components.DeviceComponent {
    return rakwireless.NewComponent()
}
EOF

# Build plugin
go build -buildmode=plugin -o rak2270.dylib main.go

# Deploy
cp rak2270.dylib ../transformer-service/plugins/
```

## Integration in Service

In your main service initialization:

```go
import "github.com/Space-DF/transformer-service/internal/devices"

func main() {
    // Create registry
    registry := devices.NewRegistry()

    // Load plugins from ./plugins directory
    loader := devices.NewPluginLoader("./plugins", registry)
    if err := loader.LoadAll(); err != nil {
        log.Fatalf("Failed to load plugins: %v", err)
    }

    // Registry now contains all loaded parsers
    fmt.Printf("Loaded parsers: %v\n", registry.ListRegisteredParsers())
}
```
