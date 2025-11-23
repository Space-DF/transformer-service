package helpers

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
)

func BuildVhostURL(vhost string, brokerURL string) (string, error) {
	baseURL := brokerURL
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse broker url: %w", err)
	}

	if vhost == "" {
		parsed.Path = "/"
		parsed.RawPath = ""
	} else {
		encoded := "/" + url.PathEscape(vhost)
		parsed.Path = encoded
		parsed.RawPath = encoded
	}

	return parsed.String(), nil
}

func ShouldHandleVhost(vhost string, allowedVhosts []string) bool {
	if len(allowedVhosts) == 0 {
		return true
	}

	return slices.Contains(allowedVhosts, vhost)
}

func MakeConsumerTag(orgSlug, vhost string) string {
	safeVhost := strings.NewReplacer("/", "_", ".", "_", ":", "_").Replace(vhost)
	return fmt.Sprintf("%s-%s-%d", orgSlug, safeVhost, time.Now().UnixNano())
}

// CountGateways counts the number of gateways with location data
func CountGateways(payload map[string]interface{}) int {
	// Try to extract uplink message from different possible locations
	var uplinkMessage map[string]interface{}

	if msg, ok := payload["uplink_message"].(map[string]interface{}); ok {
		uplinkMessage = msg
	} else if payloadData, ok := payload["payload"].(map[string]interface{}); ok {
		if msg, ok := payloadData["uplink_message"].(map[string]interface{}); ok {
			uplinkMessage = msg
		} else {
			uplinkMessage = payloadData
		}
	} else if uplinkEvent, ok := payload["uplinkEvent"].(map[string]interface{}); ok {
		// Check uplinkEvent for rxInfo (custom format)
		uplinkMessage = uplinkEvent
	} else {
		uplinkMessage = payload
	}

	// Check for gateway metadata in multiple possible locations
	var rxMetadata []interface{}
	var ok bool

	if rxMetadata, ok = uplinkMessage["rx_metadata"].([]interface{}); !ok {
		if rxMetadata, ok = uplinkMessage["gateways"].([]interface{}); !ok {
			if rxMetadata, ok = uplinkMessage["gateway_info"].([]interface{}); !ok {
				rxMetadata, ok = uplinkMessage["rxInfo"].([]interface{})
			}
		}
	}

	if !ok {
		return 0
	}

	count := 0
	for _, gw := range rxMetadata {
		gateway, ok := gw.(map[string]interface{})
		if !ok {
			continue
		}

		// Check if gateway has location data
		if location, exists := gateway["location"]; exists && location != nil {
			count++
		} else {
			// Check for direct lat/lon fields
			if _, hasLat := gateway["latitude"]; hasLat {
				if _, hasLon := gateway["longitude"]; hasLon {
					count++
				}
			}
		}
	}

	return count
}

// HasLocationData checks if the payload contains location data
func HasLocationData(payload map[string]interface{}) bool {
	// Try to extract uplink message from different possible locations
	var uplinkMessage map[string]interface{}

	if msg, ok := payload["uplink_message"].(map[string]interface{}); ok {
		uplinkMessage = msg
	} else if payloadData, ok := payload["payload"].(map[string]interface{}); ok {
		if msg, ok := payloadData["uplink_message"].(map[string]interface{}); ok {
			uplinkMessage = msg
		} else {
			uplinkMessage = payloadData
		}
	} else if uplinkEvent, ok := payload["uplinkEvent"].(map[string]interface{}); ok {
		// Check uplinkEvent for rxInfo (custom format)
		uplinkMessage = uplinkEvent
	} else {
		uplinkMessage = payload
	}

	// Check for gateway metadata in multiple possible locations
	var rxMetadata []interface{}
	var ok bool

	if rxMetadata, ok = uplinkMessage["rx_metadata"].([]interface{}); ok {
		return len(rxMetadata) > 0
	}
	if rxMetadata, ok = uplinkMessage["gateways"].([]interface{}); ok {
		return len(rxMetadata) > 0
	}
	if rxMetadata, ok = uplinkMessage["gateway_info"].([]interface{}); ok {
		return len(rxMetadata) > 0
	}
	if rxMetadata, ok = uplinkMessage["rxInfo"].([]interface{}); ok {
		return len(rxMetadata) > 0
	}

	return false
}
