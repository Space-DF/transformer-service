package seeed

import (
	"log"

	"github.com/Space-DF/transformer-service/internal/components/registry"
)

func init() {
	component := NewSeeedComponent()
	if err := registry.RegisterComponent("seeed", component); err != nil {
		log.Printf("Failed to register Seeed component: %v", err)
	} else {
		log.Println("Seeed component registered successfully")
	}
}
