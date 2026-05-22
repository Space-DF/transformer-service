package r809ag

import (
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

const (
	Model        = "R809AG"
	Manufacturer = "netvox"
)

// R809AGComponent implements the common.Parser interface for the Netvox R809AG
// LoRaWAN Smart Plug with power monitoring.
//
// Payload reference: Netvox R809A/R809AG User Manual
// http://www.netvox.com.tw/um/r809a/r809ausermanual.pdf
type R809AGComponent struct{}

func NewR809AGComponent() *R809AGComponent { return &R809AGComponent{} }

func (p *R809AGComponent) SupportsGPS() bool        { return false }
func (p *R809AGComponent) GetSupportedPorts() []int { return []int{6, 7} }
func (p *R809AGComponent) GetSupportedEntityTypes() []string {
	return []string{
		"switch", "voltage", "current", "power", "energy",
		"overcurrent_alarm", "dash_current_alarm", "power_off_alarm",
	}
}

var _ interface {
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
	ParsePayload(*common.RawPayload) (*common.ParsedData, error)
	ParseToEntities(string, string, *common.RawPayload, *common.Location) ([]common.Entity, error)
} = (*R809AGComponent)(nil)
