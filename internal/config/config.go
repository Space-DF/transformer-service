package config

import (
	"log"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type Config struct {
	Server        ServerConfig        `mapstructure:"server"`
	AMQP          AMQPConfig          `mapstructure:"amqp"`
	OrgEvents     OrgEventsConfig     `mapstructure:"org_events"`
	RawDataLog    RawDataLogConfig    `mapstructure:"raw_data_log"`
	OpenTelemetry OpenTelemetryConfig `mapstructure:"opentelemetry"`
}

type ServerConfig struct {
	LogLevel string `mapstructure:"log_level" env:"SERVER_LOG_LEVEL"`
}

type AMQPConfig struct {
	BrokerURL     string   `mapstructure:"broker_url" env:"AMQP_BROKER_URL"`
	AllowedVhosts []string `mapstructure:"allowed_vhosts" env:"AMQP_ALLOWED_VHOSTS"`
	Exchange      string   `mapstructure:"exchange" env:"AMQP_EXCHANGE"`
	Queue         string   `mapstructure:"queue" env:"AMQP_QUEUE"`
	RoutingKey    string   `mapstructure:"routing_key" env:"AMQP_ROUTING_KEY"`
	OutputTopics  []string `mapstructure:"output_topics" env:"AMQP_OUTPUT_TOPICS"`
	EntityBridgeRoutingKey string `mapstructure:"entity_bridge_routing_key" env:"AMQP_ENTITY_BRIDGE_ROUTING_KEY"`
	ConsumerTag            string `mapstructure:"consumer_tag" env:"AMQP_CONSUMER_TAG"`
	PrefetchCount          int    `mapstructure:"prefetch_count" env:"AMQP_PREFETCH_COUNT"`
	AutoAck                bool   `mapstructure:"auto_ack" env:"AMQP_AUTO_ACK"`
}

type OrgEventsConfig struct {
	Exchange    string `mapstructure:"exchange" env:"ORG_EVENTS_EXCHANGE"`
	Queue       string `mapstructure:"queue" env:"ORG_EVENTS_QUEUE"`
	RoutingKey  string `mapstructure:"routing_key" env:"ORG_EVENTS_ROUTING_KEY"`
	ConsumerTag string `mapstructure:"consumer_tag" env:"ORG_EVENTS_CONSUMER_TAG"`
}

type RawDataLogConfig struct {
	LogDir        string `mapstructure:"log_dir" env:"RAW_DATA_LOG_DIR"`
	EnableFileLog bool   `mapstructure:"enable_file_log" env:"RAW_DATA_ENABLE_FILE_LOG"`
	EnableJSONLog bool   `mapstructure:"enable_json_log" env:"RAW_DATA_ENABLE_JSON_LOG"`
	MaxFileSize   int64  `mapstructure:"max_file_size" env:"RAW_DATA_MAX_FILE_SIZE"`
}

type OpenTelemetryConfig struct {
	Endpoint      string  `mapstructure:"endpoint" env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	Environment   string  `mapstructure:"environment" env:"OTEL_ENVIRONMENT"`
	SamplingRatio float64 `mapstructure:"sampling_ratio" env:"OTEL_TRACES_SAMPLER_ARG"`
}

func New() (Config, error) {
	var config Config

	vp := viper.New()

	// Set defaults first (lowest priority)
	setDefaults(vp)

	// Load config file (medium priority)
	vp.SetConfigFile("configs/config.yaml")
	if err := vp.ReadInConfig(); err != nil {
		log.Printf("Config file not found, using defaults and environment variables")
	}

	// Load .env file (higher priority)
	if err := godotenv.Load(".env"); err != nil {
		log.Printf("No .env file found")
	}

	// Enable OS environment variables (highest priority)
	vp.AutomaticEnv()
	vp.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	vp.SetEnvPrefix("")

	// Manually bind environment variables to ensure they're read
	_ = vp.BindEnv("server.log_level", "SERVER_LOG_LEVEL")
	_ = vp.BindEnv("amqp.broker_url", "AMQP_BROKER_URL")
	_ = vp.BindEnv("amqp.output_topics", "AMQP_OUTPUT_TOPICS")
	_ = vp.BindEnv("amqp.prefetch_count", "AMQP_PREFETCH_COUNT")
	_ = vp.BindEnv("amqp.auto_ack", "AMQP_AUTO_ACK")
	_ = vp.BindEnv("amqp.allowed_vhosts", "AMQP_ALLOWED_VHOSTS")
	_ = vp.BindEnv("amqp.entity_bridge_routing_key", "AMQP_ENTITY_BRIDGE_ROUTING_KEY")

	// Bind org events environment variables
	_ = vp.BindEnv("org_events.exchange", "ORG_EVENTS_EXCHANGE")
	_ = vp.BindEnv("org_events.queue", "ORG_EVENTS_QUEUE")
	_ = vp.BindEnv("org_events.routing_key", "ORG_EVENTS_ROUTING_KEY")
	_ = vp.BindEnv("org_events.consumer_tag", "ORG_EVENTS_CONSUMER_TAG")

	_ = vp.BindEnv("raw_data_log.log_dir", "RAW_DATA_LOG_DIR")
	_ = vp.BindEnv("raw_data_log.enable_file_log", "RAW_DATA_ENABLE_FILE_LOG")
	_ = vp.BindEnv("raw_data_log.enable_json_log", "RAW_DATA_ENABLE_JSON_LOG")
	_ = vp.BindEnv("raw_data_log.max_file_size", "RAW_DATA_MAX_FILE_SIZE")

	// OpenTelemetry environment variables
	_ = vp.BindEnv("opentelemetry.endpoint", "OTEL_EXPORTER_OTLP_ENDPOINT")
	_ = vp.BindEnv("opentelemetry.environment", "OTEL_ENVIRONMENT")
	_ = vp.BindEnv("opentelemetry.sampling_ratio", "OTEL_TRACES_SAMPLER_ARG")

	if err := vp.Unmarshal(&config); err != nil {
		return config, err
	}

	if raw := vp.GetString("amqp.allowed_vhosts"); raw != "" {
		config.AMQP.AllowedVhosts = splitAndTrim(raw)
	}

	return config, nil
}

func setDefaults(vp *viper.Viper) {
	vp.SetDefault("server.log_level", "info")
	vp.SetDefault("amqp.broker_url", "amqp://default:${RABBITMQ_DEFAULT_PASS}@rabbitmq:5672/")
	vp.SetDefault("amqp.output_topics", []string{"tenant.*.transformed.device.location", "tenant.*.transformed.telemetry.device.location"})
	vp.SetDefault("amqp.consumer_tag", "transformer-service")
	vp.SetDefault("amqp.prefetch_count", 10)
	vp.SetDefault("amqp.auto_ack", false)
	vp.SetDefault("amqp.allowed_vhosts", "")
	vp.SetDefault("amqp.entity_bridge_routing_key", "tenant.%s.space.%s.entity.%s.telemetry")

	// Org events defaults
	vp.SetDefault("org_events.exchange", "org.events")
	vp.SetDefault("org_events.queue", "transformer.org.events.queue")
	vp.SetDefault("org_events.routing_key", "org.#")
	vp.SetDefault("org_events.consumer_tag", "transformer-org-events")
	vp.SetDefault("raw_data_log.log_dir", "logs/raw_data")
	vp.SetDefault("raw_data_log.enable_file_log", true)
	vp.SetDefault("raw_data_log.enable_json_log", true)
	vp.SetDefault("raw_data_log.max_file_size", 104857600) // 100MB

	// OpenTelemetry defaults
	vp.SetDefault("opentelemetry.endpoint", "signoz-otel-collector:4317")
	vp.SetDefault("opentelemetry.environment", "development")
	vp.SetDefault("opentelemetry.sampling_ratio", 1.0)
}

func splitAndTrim(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
