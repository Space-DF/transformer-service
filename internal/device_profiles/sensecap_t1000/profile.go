package sensecap_t1000

import (
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

const (
	Model        = "SENSECAP_T1000"
	Manufacturer = "seeed"
)

// Parser implements devicecommon.Parser for the SenseCAP T1000.
type Parser struct{}

func NewParser() *Parser { return &Parser{} }

func (p *Parser) SupportsGPS() bool        { return true }
func (p *Parser) GetSupportedPorts() []int { return []int{1, 5} }
func (p *Parser) GetSupportedEntityTypes() []string {
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
} = (*Parser)(nil)
