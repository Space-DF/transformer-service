package abeeway

import (
	"log"

	"github.com/Space-DF/transformer-service/internal/components/registry"
)

// init automatically registers the Abeeway component
func init() {
	component := NewAbeewayComponent()
	if err := registry.RegisterComponent("abeeway", component); err != nil {
		log.Printf("Failed to register Abeeway component: %v", err)
	} else {
		log.Println("Abeeway component registered successfully")
	}
}
