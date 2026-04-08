package milesight

import (
	"log"

	"github.com/Space-DF/transformer-service/internal/components/registry"
)

// init automatically registers the Milesight component
func init() {
	component := NewMilesightComponent()
	if err := registry.RegisterComponent("milesight", component); err != nil {
		log.Printf("Failed to register Milesight component: %v", err)
	} else {
		log.Println("Milesight component registered successfully")
	}
}
