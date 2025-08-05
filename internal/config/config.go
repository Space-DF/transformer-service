package config

import (
	"log"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	MQTT       MQTTConfig       `mapstructure:"mqtt"`
	RawDataLog RawDataLogConfig `mapstructure:"raw_data_log"`
}

type ServerConfig struct {
	LogLevel string `mapstructure:"log_level" env:"SERVER_LOG_LEVEL"`
}

type MQTTConfig struct {
	BrokerURL     string `mapstructure:"broker_url" env:"AMQP_BROKER_URL"`
	Exchange      string `mapstructure:"exchange" env:"AMQP_EXCHANGE"`
	Queue         string `mapstructure:"queue" env:"AMQP_QUEUE"`
	RoutingKey    string `mapstructure:"routing_key" env:"AMQP_ROUTING_KEY"`
	OutputTopic   string `mapstructure:"output_topic" env:"AMQP_OUTPUT_TOPIC"`
	ConsumerTag   string `mapstructure:"consumer_tag" env:"AMQP_CONSUMER_TAG"`
	PrefetchCount int    `mapstructure:"prefetch_count" env:"AMQP_PREFETCH_COUNT"`
	AutoAck       bool   `mapstructure:"auto_ack" env:"AMQP_AUTO_ACK"`
}

type RawDataLogConfig struct {
	LogDir        string `mapstructure:"log_dir" env:"RAW_DATA_LOG_DIR"`
	EnableFileLog bool   `mapstructure:"enable_file_log" env:"RAW_DATA_ENABLE_FILE_LOG"`
	EnableJSONLog bool   `mapstructure:"enable_json_log" env:"RAW_DATA_ENABLE_JSON_LOG"`
	MaxFileSize   int64  `mapstructure:"max_file_size" env:"RAW_DATA_MAX_FILE_SIZE"`
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
	vp.BindEnv("server.log_level", "SERVER_LOG_LEVEL")
	vp.BindEnv("mqtt.broker_url", "AMQP_BROKER_URL")
	vp.BindEnv("mqtt.exchange", "AMQP_EXCHANGE")
	vp.BindEnv("mqtt.queue", "AMQP_QUEUE")
	vp.BindEnv("mqtt.routing_key", "AMQP_ROUTING_KEY")
	vp.BindEnv("mqtt.output_topic", "AMQP_OUTPUT_TOPIC")
	vp.BindEnv("mqtt.consumer_tag", "AMQP_CONSUMER_TAG")
	vp.BindEnv("mqtt.prefetch_count", "AMQP_PREFETCH_COUNT")
	vp.BindEnv("mqtt.auto_ack", "AMQP_AUTO_ACK")
	vp.BindEnv("raw_data_log.log_dir", "RAW_DATA_LOG_DIR")
	vp.BindEnv("raw_data_log.enable_file_log", "RAW_DATA_ENABLE_FILE_LOG")
	vp.BindEnv("raw_data_log.enable_json_log", "RAW_DATA_ENABLE_JSON_LOG")
	vp.BindEnv("raw_data_log.max_file_size", "RAW_DATA_MAX_FILE_SIZE")

	return config, vp.Unmarshal(&config)
}

func setDefaults(vp *viper.Viper) {
	vp.SetDefault("server.log_level", "info")
	vp.SetDefault("mqtt.broker_url", "amqp://admin:password@rabbitmq:5672/")
	vp.SetDefault("mqtt.exchange", "amq.topic")
	vp.SetDefault("mqtt.queue", "transformer_device_queue")
	vp.SetDefault("mqtt.routing_key", "device.data")
	vp.SetDefault("mqtt.output_topic", "transformed/device/location")
	vp.SetDefault("mqtt.consumer_tag", "transformer-service")
	vp.SetDefault("mqtt.prefetch_count", 10)
	vp.SetDefault("mqtt.auto_ack", false)
	vp.SetDefault("raw_data_log.log_dir", "logs/raw_data")
	vp.SetDefault("raw_data_log.enable_file_log", true)
	vp.SetDefault("raw_data_log.enable_json_log", true)
	vp.SetDefault("raw_data_log.max_file_size", 104857600) // 100MB
}
