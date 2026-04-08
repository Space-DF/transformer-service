package rak2270

import (
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

const (
	Model        = "RAK2270"
	Manufacturer = "rakwireless"
)

// RAK2270Component implements devicecommon.RAK2270Component for the RAK2270.
type RAK2270Component struct{}

func NewRAK2270Component() *RAK2270Component { return &RAK2270Component{} }

func (p *RAK2270Component) SupportsGPS() bool        { return false }
func (p *RAK2270Component) GetSupportedPorts() []int { return []int{1, 2, 3} }
func (p *RAK2270Component) GetSupportedEntityTypes() []string {
	return []string{"location", "temperature", "battery_voltage"}
}

var _ interface {
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
	ParsePayload(*common.RawPayload) (*common.ParsedData, error)
	ParseToEntities(string, string, *common.RawPayload, *common.Location) ([]common.Entity, error)
} = (*RAK2270Component)(nil)
