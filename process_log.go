package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"strings"

	"github.com/Space-DF/transformer-service-go/internal/services"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run process_log.go <log_file_path>")
	}

	logFile := os.Args[1]
	file, err := os.Open(logFile)
	if err != nil {
		log.Fatalf("Error opening log file: %v", err)
	}
	defer file.Close()

	locationService := services.NewLocationService()
	scanner := bufio.NewScanner(file)
	lineNum := 0

	fmt.Println("Processing log file:", logFile)
	fmt.Println(strings.Repeat("=", 80))

	// Statistics tracking
	var stats struct {
		total         int
		singleGateway int
		multiGateway  int
		errors        int
		locationMatch int
		locationDiff  int
	}

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		var logEntry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
			fmt.Printf("Line %d: Error parsing JSON: %v\n", lineNum, err)
			continue
		}

		// Extract the decoded_raw_data which contains the LoRaWAN payload
		decodedData, ok := logEntry["decoded_raw_data"].(map[string]interface{})
		if !ok {
			fmt.Printf("Line %d: No decoded_raw_data found\n", lineNum)
			continue
		}

		// Get device EUI
		deviceEUI := ""
		if deviceInfo, ok := decodedData["deviceInfo"].(map[string]interface{}); ok {
			if devEui, ok := deviceInfo["devEui"].(string); ok {
				deviceEUI = devEui
			}
		}

		stats.total++

		// Get gateway count for comparison
		gatewayCount := 0
		if rxInfo, ok := decodedData["rxInfo"].([]interface{}); ok {
			gatewayCount = len(rxInfo)
		}

		// Process the location calculation
		locationData, err := locationService.CalculateDeviceLocation(decodedData)
		if err != nil {
			stats.errors++
			if gatewayCount > 1 {
				fmt.Printf("Line %d (DevEUI: %s): Location calculation error: %v\n", lineNum, deviceEUI, err)
			}
			continue
		}

		// Get the original processing info for comparison
		originalResult := ""
		if processingInfo, ok := logEntry["processing_info"].(map[string]interface{}); ok {
			if locationResult, ok := processingInfo["location_result"].(map[string]interface{}); ok {
				originalLat, _ := locationResult["latitude"].(float64)
				originalLon, _ := locationResult["longitude"].(float64)
				accuracy, _ := locationResult["accuracy"].(string)
				originalResult = fmt.Sprintf("Lat: %.6f, Lon: %.6f (%s)", originalLat, originalLon, accuracy)
			}
		}

		// Track statistics
		if gatewayCount == 1 {
			stats.singleGateway++
		} else if gatewayCount > 1 {
			stats.multiGateway++
		}

		// Compare with original results
		if processingInfo, ok := logEntry["processing_info"].(map[string]interface{}); ok {
			if locationResult, ok := processingInfo["location_result"].(map[string]interface{}); ok {
				originalLat, _ := locationResult["latitude"].(float64)
				originalLon, _ := locationResult["longitude"].(float64)

				// Check if locations match (within small tolerance)
				latDiff := math.Abs(locationData.Latitude - originalLat)
				lonDiff := math.Abs(locationData.Longitude - originalLon)

				if latDiff < 0.000001 && lonDiff < 0.000001 {
					stats.locationMatch++
				} else {
					stats.locationDiff++
				}
			}
		}

		// Display multi-gateway entries for verification
		if gatewayCount > 1 && lineNum <= 100 {
			fmt.Printf("Line %d - DevEUI: %s\n", lineNum, deviceEUI)
			fmt.Printf("  Gateways: %d\n", gatewayCount)
			fmt.Printf("  New Algorithm: Lat: %.6f, Lon: %.6f\n", locationData.Latitude, locationData.Longitude)
			fmt.Printf("  Original:      %s\n", originalResult)
			fmt.Println("  " + strings.Repeat("-", 60))
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading log file: %v", err)
	}

	// Print summary statistics
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("PROCESSING SUMMARY")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Total entries processed: %d\n", stats.total)
	fmt.Printf("Single gateway entries: %d\n", stats.singleGateway)
	fmt.Printf("Multi-gateway entries: %d\n", stats.multiGateway)
	fmt.Printf("Processing errors: %d\n", stats.errors)
	fmt.Printf("Location matches: %d\n", stats.locationMatch)
	fmt.Printf("Location differences: %d\n", stats.locationDiff)

	if stats.total > 0 {
		successRate := float64(stats.locationMatch) / float64(stats.total-stats.errors) * 100
		fmt.Printf("Algorithm accuracy: %.2f%%\n", successRate)
	}
}
