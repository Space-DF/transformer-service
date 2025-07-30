package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "transformer",
	Short: "Transformer Service - LoRaWAN device location processing service",
	Long: `Transformer Service is a Go-based microservice that processes 
LoRaWAN device data and calculates device locations using RSSI-based trilateration.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}