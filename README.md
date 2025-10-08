# Transformer Service

A Go-based microservice that consumes LoRaWAN device data from MQTT topics via RabbitMQ, calculates device locations using RSSI-based trilateration algorithms, and publishes transformed data.

## Features

- **MQTT Consumer**: Consumes device data from RabbitMQ MQTT topics
- **Location Calculation**: Supports 1, 2, 3, and multiple gateway trilateration
- **RSSI-based Distance Estimation**: Uses path loss models to estimate distances
- **Data Transformation**: Converts raw LoRaWAN data to standardized format
- **Configurable**: YAML and environment variable configuration
- **Docker Support**: Containerized deployment ready

## Architecture

The service is built using:
- **RabbitMQ AMQP**: Message queue consumer for MQTT topics
- **Viper**: Configuration management
- **Cobra**: CLI interface
- **Gonum**: Mathematical computations for trilateration

## Data Flow

```
LoRaWAN Device → MPA Service → EMQX → RabbitMQ → Transformer Service → Output Topic
```

1. **Input**: Consumes from `device/data` topic via RabbitMQ
2. **Processing**: Calculates device location using gateway RSSI data
3. **Output**: Publishes transformed data to `transformed/device/location` topic

## Installation

### Prerequisites
- Go 1.24.4 or later
- Docker (optional)

### Local Development

1. Clone the repository
2. Install dependencies:
   ```bash
   make deps
   ```

3. Run the service:
   ```bash
   make run
   ```

### Docker

1. Build the Docker image:
   ```bash
   make docker-build
   ```

2. Run the container:
   ```bash
   make docker-run
   ```

## Configuration

The service can be configured via:
1. `configs/config.yaml` (default configuration)
2. Environment variables
3. `.env` file

### Configuration Options

```yaml
server:
  log_level: "info"

amqp:
  broker_url: "amqp://admin:password@rabbitmq:5672/"
  exchange: "device_exchange"
  queue: "device_data"
  routing_key: "device.data"
  output_topic: "transformed/device/location"
  consumer_tag: "transformer-service"
  prefetch_count: 10
  auto_ack: false

raw_data_log:
  log_dir: "logs/raw_data"
  enable_file_log: false
  enable_json_log: false
  max_file_size: 104857600  # 100MB
```

### Raw Data Logging

The service supports logging raw data for training and debugging purposes. Configure via environment variables:

```bash
RAW_DATA_LOG_DIR="logs/raw_data"           # Directory for log files
RAW_DATA_ENABLE_FILE_LOG=true              # Enable file logging
RAW_DATA_ENABLE_JSON_LOG=false             # Enable JSON stdout logging
RAW_DATA_MAX_FILE_SIZE=104857600           # 100MB file size limit
```

### Log Management with Logrotate

To prevent raw logs from consuming too much disk space, use the included logrotate configuration:

1. **Install logrotate configuration:**
   ```bash
   sudo cp logrotate.conf /etc/logrotate.d/transformer-raw-logs
   ```

2. **Test configuration:**
   ```bash
   sudo logrotate -d /etc/logrotate.d/transformer-raw-logs
   ```

3. **Manual rotation:**
   ```bash
   sudo logrotate -f /etc/logrotate.d/transformer-raw-logs
   ```

The logrotate configuration:
- Rotates logs daily
- Keeps 7 days of logs
- Compresses old files (saves ~90% disk space)
- Handles active log files safely with `copytruncate`

## Message Formats

### Input Message (from MPA Service)
```json
{
  "uplink_message": {
    "rx_metadata": [
      {
        "location": {
          "latitude": 10.762622,
          "longitude": 106.660172
        },
        "rssi": -80,
        "snr": 9.5
      }
    ],
    "settings": {
      "frequency": 923200000
    },
    "f_cnt": 123,
    "f_port": 1
  },
  "end_device_ids": {
    "dev_eui": "1234567890ABCDEF",
    "application_ids": {
      "application_id": "my-app"
    }
  },
  "received_at": "2023-01-01T12:00:00Z"
}
```

### Output Message (Transformed)
```json
{
  "device_eui": "1234567890ABCDEF",
  "location": {
    "latitude": 10.762622,
    "longitude": 106.660172,
    "accuracy": "triangulated"
  },
  "timestamp": "2023-01-01T12:00:00Z",
  "organization": "example-org",
  "source": "transformer-service",
  "metadata": {
    "frequency": 923200000,
    "gateways": [
      {
        "rssi": -80,
        "snr": 9.5,
        "location": {
          "latitude": 10.762622,
          "longitude": 106.660172
        }
      }
    ],
    "frame_counter": 123,
    "port": 1
  }
}
```

## Location Calculation

The service supports different trilateration methods based on the number of available gateways:

1. **Single Gateway**: Returns gateway location
2. **Two Gateways**: Circle intersection method
3. **Three Gateways**: Linear system solving
4. **Multiple Gateways**: Least squares optimization

The distance estimation uses the path loss model:
```
d = d0 * 10^((PL - PL_d0) / (10 * n))
```

Where:
- `d`: estimated distance
- `d0`: reference distance (1m)
- `PL`: path loss (TX_power - RSSI)
- `PL_d0`: path loss at reference distance
- `n`: path loss exponent (4.0)

## Development

### Available Make Commands

- `make build`: Build the binary
- `make test`: Run tests
- `make clean`: Clean build artifacts
- `make deps`: Download dependencies
- `make run`: Build and run the service
- `make fmt`: Format code
- `make lint`: Lint code (requires golangci-lint)
- `make security`: Security scan (requires gosec)

### Project Structure

```
├── cmd/transformer/          # Application entry point
│   ├── cmd/                 # CLI commands
│   └── main.go             # Main function
├── internal/               # Private application code
│   ├── config/            # Configuration management
│   ├── handlers/          # HTTP handlers
│   ├── logger/            # Logging setup
│   ├── models/            # Data models
│   └── services/          # Business logic
├── configs/               # Configuration files
├── bin/                   # Built binaries
└── Dockerfile             # Docker configuration
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Run `make test` and `make lint`
6. Submit a pull request

## License

This project is part of the Space-DF platform.

