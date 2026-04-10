package netvox

import (
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

const (
	Model        = "R718N17"
	Manufacturer = "netvox"
)

// NetvoxR718N17Component implements device profile for the Netvox R718N17.
// R718N17 is a LoRaWAN® 1-phase current sensor that measures current on a single phase.
type NetvoxR718N17Component struct{}

func NewNetvoxR718N17Component() *NetvoxR718N17Component { return &NetvoxR718N17Component{} }

func (p *NetvoxR718N17Component) SupportsGPS() bool        { return false }
func (p *NetvoxR718N17Component) GetSupportedPorts() []int { return []int{6, 7} }
func (p *NetvoxR718N17Component) GetSupportedEntityTypes() []string {
	return []string{
		"location",
		"battery_voltage",
		"current_ma",
		"current_raw_ma",
		"multiplier",
		"report_type",
		"report_mode",
		"config_status",
		"min_time_s",
		"max_time_s",
		"current_change_ma",
	}
}

var _ interface {
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
	ParsePayload(*common.RawPayload) (*common.ParsedData, error)
	ParseToEntities(string, string, *common.RawPayload, *common.Location) ([]common.Entity, error)
} = (*NetvoxR718N17Component)(nil)
