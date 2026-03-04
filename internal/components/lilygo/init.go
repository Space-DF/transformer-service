package lilygo

import (
	"log"

	"github.com/Space-DF/transformer-service/internal/components/registry"
)

// init automatically registers the TBeam component
func init() {
	component := NewTBeamComponent()
	if err := registry.RegisterComponent("tbeam", component); err != nil {
		log.Printf("Failed to register TBeam component: %v", err)
	} else {
		log.Println("TBeam component registered successfully")
	}
}
