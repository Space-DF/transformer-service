package lcc01lb

import "github.com/Space-DF/transformer-service/internal/device_profiles/common"

const (
	Model        = "LCC01LB"
	Manufacturer = "dragino"
)

// LCC01LBComponent implements the Parser interface for the Dragino LCC01-LB load cell sensor.
// fPort 2: Uplink data — battery, weight, weight state, scale factor.
// fPort 5: Config info — firmware version, frequency band, battery.
// This device has no GPS; location is always nil.
type LCC01LBComponent struct{}

func NewLCC01LBComponent() *LCC01LBComponent { return &LCC01LBComponent{} }

func (p *LCC01LBComponent) SupportsGPS() bool        { return false }
func (p *LCC01LBComponent) GetSupportedPorts() []int { return []int{2, 5} }
func (p *LCC01LBComponent) GetSupportedEntityTypes() []string {
	return []string{
		"battery_voltage",
		"actual_weight_g",
		"weight_reading",
		"weight_state",
		"scale_factor",
		"weight_flag",
	}
}

var _ interface {
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
	ParsePayload(*common.RawPayload) (*common.ParsedData, error)
	ParseToEntities(string, string, *common.RawPayload, *common.Location) ([]common.Entity, error)
} = (*LCC01LBComponent)(nil)
