package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/Space-DF/transformer-service-go/internal/config"
	"github.com/Space-DF/transformer-service-go/internal/mqtt"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the transformer service MQTT consumer",
	Long:  `Starts the transformer service MQTT consumer to process LoRaWAN device data from RabbitMQ.`,
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

	// Create MQTT consumer
	consumer := mqtt.NewConsumer(cfg.MQTT)

	// Connect to RabbitMQ
	if err := consumer.Connect(); err != nil {
		return fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	log.Printf("Connected to RabbitMQ: %s", cfg.MQTT.BrokerURL)
	log.Printf("Consuming from topic: %s", cfg.MQTT.InputTopic)
	log.Printf("Publishing to topic: %s", cfg.MQTT.OutputTopic)

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