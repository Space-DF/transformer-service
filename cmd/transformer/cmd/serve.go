package cmd

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/Space-DF/transformer-service/internal/config"
	"github.com/Space-DF/transformer-service/internal/mqtt"
	"github.com/Space-DF/transformer-service/internal/services"
	"github.com/Space-DF/transformer-service/internal/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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
	// Initialize OpenTelemetry tracing
	cleanup := telemetry.InitTracing("transformer-service")
	defer cleanup()
	
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
	consumer := mqtt.NewConsumer(cfg.AMQP, loggerService, deviceProfileService)

	// Connect to AMQP broker
	if err := consumer.Connect(); err != nil {
		return fmt.Errorf("failed to connect to AMQP broker: %w", err)
	}

	log.Printf("Connected to AMQP broker: %s", cfg.AMQP.BrokerURL)
	log.Printf("Consuming from queue: %s with routing key: %s", cfg.AMQP.Queue, cfg.AMQP.RoutingKey)
	log.Printf("Publishing to topic: %s", cfg.AMQP.OutputTopic)
	log.Printf("Raw data logging enabled - File: %t, JSON: %t, Dir: %s", cfg.RawDataLog.EnableFileLog, cfg.RawDataLog.EnableJSONLog, cfg.RawDataLog.LogDir)

	// Setup HTTP server for health check endpoint
	mux := http.NewServeMux()
	
	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := fmt.Fprintf(w, `{"status": "healthy", "service": "transformer-service", "timestamp": "%s"}`, time.Now().Format(time.RFC3339)); err != nil {
			log.Printf("Error writing health check response: %v", err)
		}
	})

	// Wrap the handler with OpenTelemetry middleware
	handler := otelhttp.NewHandler(mux, "transformer-service")

	// Create HTTP server
	server := &http.Server{
		Addr:         ":8080",
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Start HTTP server in goroutine
	go func() {
		log.Printf("HTTP server starting on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create error channel for consumer
	consumerErr := make(chan error, 1)

	// Start consumer in a goroutine
	go func() {
		if err := consumer.Start(ctx); err != nil {
			consumerErr <- err
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	
	select {
	case <-quit:
		log.Println("Received shutdown signal")
	case err := <-consumerErr:
		log.Printf("Consumer failed: %v", err)
	}

	log.Println("Shutting down transformer service...")

	// Cancel context to stop consumer
	cancel()

	// Stop HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	// Stop consumer
	if err := consumer.Stop(); err != nil {
		log.Printf("Error stopping consumer: %v", err)
	}

	log.Println("Transformer service stopped")
	return nil
}
