package rak4630

import (
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
	"github.com/Space-DF/transformer-service/internal/services"
)

const (
	Model        = "RAK4630"
	Manufacturer = "rakwireless"
)

// RAK4630Component implements devicecommon.RAK4630Component for the RAK4630.
type RAK4630Component struct {
	locationService *services.LocationService
}

func NewRAK4630Component(locationService *services.LocationService) *RAK4630Component {
	return &RAK4630Component{locationService: locationService}
}

func (p *RAK4630Component) SupportsGPS() bool        { return true }
func (p *RAK4630Component) GetSupportedPorts() []int { return []int{1, 2, 3, 4, 5} }
func (p *RAK4630Component) GetSupportedEntityTypes() []string {
	return []string{"location", "temperature", "humidity", "pressure", "battery_voltage"}
}

var _ interface {
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
	ParsePayload(*common.RawPayload) (*common.ParsedData, error)
	ParseToEntities(string, string, *common.RawPayload, *common.Location) ([]common.Entity, error)
} = (*RAK4630Component)(nil)
