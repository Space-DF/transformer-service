package digitalmatter

import (
	"log"

	"github.com/Space-DF/transformer-service/internal/components/registry"
)

func init() {
	component := NewDigitalMatterComponent()
	if err := registry.RegisterComponent("digitalmatter", component); err != nil {
		log.Printf("Failed to register Digital Matter component: %v", err)
	} else {
		log.Println("Digital Matter component registered successfully")
	}
}
