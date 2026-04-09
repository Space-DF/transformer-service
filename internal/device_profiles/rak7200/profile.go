package rak7200

import (
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

const (
	Model        = "RAK7200"
	Manufacturer = "rakwireless"
)

// RAK7200Component implements devicecommon.RAK7200Component for the RAK7200.
type RAK7200Component struct{}

func NewRAK7200Component() *RAK7200Component { return &RAK7200Component{} }

func (p *RAK7200Component) SupportsGPS() bool        { return true }
func (p *RAK7200Component) GetSupportedPorts() []int { return []int{2, 3, 4, 5} }
func (p *RAK7200Component) GetSupportedEntityTypes() []string {
	return []string{"location", "battery", "temperature"}
}

var _ interface {
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
	ParsePayload(*common.RawPayload) (*common.ParsedData, error)
	ParseToEntities(string, string, *common.RawPayload, *common.Location) ([]common.Entity, error)
} = (*RAK7200Component)(nil)
