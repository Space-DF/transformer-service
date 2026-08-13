package sensecap_t1000e

import "github.com/Space-DF/transformer-service/internal/device_profiles/common"

const (
	Model        = "SENSECAP_T1000E"
	Manufacturer = "seeed"
)

// SenseCapT1000EComponent implements the parser for Seeed SenseCAP T1000E.
type SenseCapT1000EComponent struct{}

func NewSenseCapT1000EComponent() *SenseCapT1000EComponent { return &SenseCapT1000EComponent{} }

func (p *SenseCapT1000EComponent) SupportsGPS() bool        { return true }
func (p *SenseCapT1000EComponent) GetSupportedPorts() []int { return []int{1, 5} }
func (p *SenseCapT1000EComponent) GetSupportedEntityTypes() []string {
	return []string{
		"location", "battery_level", "temperature", "light",
	}
}

var _ interface {
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
	ParsePayload(*common.RawPayload) (*common.ParsedData, error)
	ParseToEntities(string, string, *common.RawPayload, *common.Location) ([]common.Entity, error)
} = (*SenseCapT1000EComponent)(nil)
