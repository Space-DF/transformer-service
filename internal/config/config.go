package config

import (
	"log"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type Config struct {
	Server ServerConfig `mapstructure:"server"`
	MQTT   MQTTConfig   `mapstructure:"mqtt"`
}

type ServerConfig struct {
	LogLevel string `mapstructure:"log_level" env:"SERVER_LOG_LEVEL"`
}

type MQTTConfig struct {
	BrokerURL     string `mapstructure:"broker_url" env:"MQTT_BROKER_URL"`
	InputTopic    string `mapstructure:"input_topic" env:"MQTT_INPUT_TOPIC"`
	OutputTopic   string `mapstructure:"output_topic" env:"MQTT_OUTPUT_TOPIC"`
	ConsumerTag   string `mapstructure:"consumer_tag" env:"MQTT_CONSUMER_TAG"`
	PrefetchCount int    `mapstructure:"prefetch_count" env:"MQTT_PREFETCH_COUNT"`
	AutoAck       bool   `mapstructure:"auto_ack" env:"MQTT_AUTO_ACK"`
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

	return config, vp.Unmarshal(&config)
}

func setDefaults(vp *viper.Viper) {
	vp.SetDefault("server.log_level", "info")
	vp.SetDefault("mqtt.broker_url", "amqp://admin:password@rabbitmq:5672/")
	vp.SetDefault("mqtt.input_topic", "device/data")
	vp.SetDefault("mqtt.output_topic", "transformed/device/location")
	vp.SetDefault("mqtt.consumer_tag", "transformer-service")
	vp.SetDefault("mqtt.prefetch_count", 10)
	vp.SetDefault("mqtt.auto_ack", false)
}