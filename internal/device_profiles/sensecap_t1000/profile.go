package sensecap_t1000

import (
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

const (
	Model        = "SENSECAP_T1000"
	Manufacturer = "seeed"
)

// SenseCapT1000Component implements devicecommon.SenseCapT1000Component for the SenseCAP T1000.
type SenseCapT1000Component struct{}

func NewSenseCapT1000Component() *SenseCapT1000Component { return &SenseCapT1000Component{} }

func (p *SenseCapT1000Component) SupportsGPS() bool        { return true }
func (p *SenseCapT1000Component) GetSupportedPorts() []int { return []int{1, 5} }
func (p *SenseCapT1000Component) GetSupportedEntityTypes() []string {
	return []string{
		"location", "battery_level", "temperature", "light",
		"motion", "shock_event", "sos_alert",
		"temperature_event", "light_event", "press_once_event",
	}
}

var _ interface {
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
	ParsePayload(*common.RawPayload) (*common.ParsedData, error)
	ParseToEntities(string, string, *common.RawPayload, *common.Location) ([]common.Entity, error)
} = (*SenseCapT1000Component)(nil)
