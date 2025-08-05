package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/Space-DF/transformer-service/internal/config"
	"github.com/Space-DF/transformer-service/internal/mqtt"
	"github.com/Space-DF/transformer-service/internal/services"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the transformer service MQTT consumer",
	Long:  `Starts the transformer service MQTT consumer to process LoRaWAN device data from AMQP broker.`,
	RunE:  runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := config.New()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Create logger service
	loggerConfig := services.LoggerConfig{
		LogDir:        cfg.RawDataLog.LogDir,
		EnableFileLog: cfg.RawDataLog.EnableFileLog,
		EnableJSONLog: cfg.RawDataLog.EnableJSONLog,
		MaxFileSize:   cfg.RawDataLog.MaxFileSize,
	}

	loggerService, err := services.NewLoggerService(loggerConfig)
	if err != nil {
		return fmt.Errorf("failed to create logger service: %w", err)
	}
	defer loggerService.Close()

	// Create device profile service
	deviceProfileService, err := services.NewDeviceProfileService("configs/device_profiles.json")
	if err != nil {
		log.Printf("Warning: Failed to load device profiles: %v. Proceeding without device profile mapping.", err)
		deviceProfileService = nil
	} else {
		log.Printf("Device profiles loaded successfully")
	}

	// Create MQTT consumer
	consumer := mqtt.NewConsumer(cfg.MQTT, loggerService, deviceProfileService)

	// Connect to AMQP broker
	if err := consumer.Connect(); err != nil {
		return fmt.Errorf("failed to connect to AMQP broker: %w", err)
	}

	log.Printf("Connected to AMQP broker: %s", cfg.MQTT.BrokerURL)
	log.Printf("Consuming from queue: %s with routing key: %s", cfg.MQTT.Queue, cfg.MQTT.RoutingKey)
	log.Printf("Publishing to topic: %s", cfg.MQTT.OutputTopic)
	log.Printf("Raw data logging enabled - File: %t, JSON: %t, Dir: %s", cfg.RawDataLog.EnableFileLog, cfg.RawDataLog.EnableJSONLog, cfg.RawDataLog.LogDir)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start consumer in a goroutine
	go func() {
		if err := consumer.Start(ctx); err != nil {
			log.Fatalf("Consumer failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down transformer service...")

	// Cancel context to stop consumer
	cancel()

	// Stop consumer
	if err := consumer.Stop(); err != nil {
		log.Printf("Error stopping consumer: %v", err)
	}

	log.Println("Transformer service stopped")
	return nil
}
