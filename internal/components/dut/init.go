package dut

import (
	"log"

	"github.com/Space-DF/transformer-service/internal/components/registry"
)

// init automatically registers the DUT component
func init() {
	component := NewDUTComponent()
	if err := registry.RegisterComponent("dut", component); err != nil {
		log.Printf("Failed to register DUT component: %v", err)
	} else {
		log.Println("DUT component registered successfully")
	}
}