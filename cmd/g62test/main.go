package main

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
	g62 "github.com/Space-DF/transformer-service/internal/device_profiles/digital_matter/g62"
)

func main() {
	tests := []struct {
		name string
		port int
		b64  string
	}{
		{"Port 1 - Full data", 1, "HtFyBiG4kz8tPLTUMNIEGQ8="},
		{"Port 2 - Data part 1", 2, "HtFyBiG4kz9aHqA="},
		{"Port 3 - Data part 2", 3, "3DT0ARwK"},
		{"Port 4 - Odometer", 4, "7V8BADkwAAA="},
		{"Port 5 - Downlink ACK", 5, "hQcB"},
	}

	for _, t := range tests {
		raw, _ := base64.StdEncoding.DecodeString(t.b64)
		payload := &common.RawPayload{
			FPort:     t.port,
			Data:      t.b64,
			Timestamp: time.Now(),
		}
		sensors, loc := g62.Decode(payload)
		fmt.Printf("\n=== %s (port %d, %d bytes) ===\n", t.name, t.port, len(raw))
		for k, v := range sensors {
			fmt.Printf("  %-20s = %v\n", k, v)
		}
		if loc != nil {
			fmt.Printf("  %-20s = lat=%.7f, lon=%.7f\n", "LOCATION", loc.Latitude, loc.Longitude)
		}
	}
}
