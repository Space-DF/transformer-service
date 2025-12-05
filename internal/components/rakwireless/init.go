package rakwireless

import (
	"log"

	"github.com/Space-DF/transformer-service/internal/components/registry"
)

// init automatically registers the RAKwireless component
func init() {
	component := NewRAKwirelessComponent()
	if err := registry.RegisterComponent("rakwireless", component); err != nil {
		log.Printf("Failed to register RAKwireless component: %v", err)
	} else {
		log.Println("RAKwireless component registered successfully")
	}
}
