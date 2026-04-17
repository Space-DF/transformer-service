package wlbv1

import (
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

const (
	Model        = "WLBV1"
	Manufacturer = "dut"
)

// WLBV1Component implements devicecommon.WLBV1Component for the WLBV1 water-level sensor.
type WLBV1Component struct{}

func NewWLBV1Component() *WLBV1Component { return &WLBV1Component{} }

func (p *WLBV1Component) SupportsGPS() bool        { return true }
func (p *WLBV1Component) GetSupportedPorts() []int { return []int{1, 2, 3, 4, 5} }
func (p *WLBV1Component) GetSupportedEntityTypes() []string {
	return []string{"location", "battery", "water_depth"}
}

var _ interface {
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
	ParsePayload(*common.RawPayload) (*common.ParsedData, error)
	ParseToEntities(string, string, *common.RawPayload, *common.Location) ([]common.Entity, error)
} = (*WLBV1Component)(nil)
