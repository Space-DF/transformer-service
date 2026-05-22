/*
Copyright 2026 Digital Fortress.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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

	"github.com/Space-DF/transformer-service/internal/api"
	"github.com/Space-DF/transformer-service/internal/config"
	deviceprofile "github.com/Space-DF/transformer-service/internal/device_profiles"
	"github.com/Space-DF/transformer-service/internal/mqtt"
	"github.com/Space-DF/transformer-service/internal/services"
	"github.com/Space-DF/transformer-service/internal/telemetry"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
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
	// Load configuration
	cfg, err := config.New()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize OpenTelemetry tracing
	cleanup := telemetry.InitTracing("transformer-service", cfg.OpenTelemetry)
	defer cleanup()

	// Create logger service
	loggerConfig := services.LoggerConfig{
		LogDir:        cfg.RawDataLog.LogDir,
		EnableFileLog: cfg.RawDataLog.EnableFileLog,
		EnableJSONLog: cfg.RawDataLog.EnableJSONLog,
	}

	loggerService, err := services.NewLoggerService(loggerConfig)
	if err != nil {
		return fmt.Errorf("failed to create logger service: %w", err)
	}
	defer loggerService.Close()

	// Create device profile service
	deviceProfileService, _ := services.NewDeviceProfileService()

	// Create location cache (in-memory)
	locationCache := services.NewLocationCache()
	locationService := services.NewLocationServiceWithCache(locationCache)

	registry := deviceprofile.NewComponentRegistry()
	if err := deviceprofile.RegisterAll(registry, locationService); err != nil {
		return fmt.Errorf("failed to register device profiles: %w", err)
	}
	deviceprofile.SetGlobal(registry)

	// Create MQTT consumer with event-driven organization discovery
	consumer := mqtt.NewConsumer(cfg.AMQP, cfg.OrgEvents, loggerService, deviceProfileService, locationService)

	// Connect to AMQP broker
	if err := consumer.Connect(); err != nil {
		return fmt.Errorf("failed to connect to AMQP broker: %w", err)
	}

	log.Printf("Connected to AMQP broker: %s", cfg.AMQP.BrokerURL)
	log.Printf("Consuming from queue: %s with routing key: %s", cfg.AMQP.Queue, cfg.AMQP.RoutingKey)
	log.Printf("Raw data logging enabled - File: %t, JSON: %t, Dir: %s", cfg.RawDataLog.EnableFileLog, cfg.RawDataLog.EnableJSONLog, cfg.RawDataLog.LogDir)

	// Initialize Echo framework
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{"*"},
	}))

	// Health check endpoint
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"status":    "healthy",
			"service":   "transformer-service",
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	// Setup API routes
	apiGroup := e.Group("/api")
	api.Setup(apiGroup, deviceProfileService)

	// Wrap Echo with OpenTelemetry
	handler := otelhttp.NewHandler(e, "transformer-service")

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

	// Close logger service
	if err := deviceProfileService.Close(); err != nil {
		log.Printf("Error closing device profile service: %v", err)
	}

	log.Println("Transformer service stopped")
	return nil
}
