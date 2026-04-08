package abeeway

import (
	deviceprofile "github.com/Space-DF/transformer-service/internal/device_profiles"
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

func init() {
	deviceprofile.Register(Model, Manufacturer, NewParser())
}

const (
	Model        = "ABEEWAY_INDUSTRIAL_TRACKER"
	Manufacturer = "abeeway"
)

// Parser implements devicecommon.Parser for the Abeeway Industrial Tracker.
type Parser struct{}

func NewParser() *Parser { return &Parser{} }

func (p *Parser) SupportsGPS() bool        { return true }
func (p *Parser) GetSupportedPorts() []int { return []int{1, 2, 5, 17, 100} }
func (p *Parser) GetSupportedEntityTypes() []string {
	return []string{
		"location", "battery_voltage", "battery_percent",
		"temperature", "speed", "heading", "sos_alert", "motion",
	}
}

// Ensure Parser satisfies the common.DeviceComponent-compatible interface at compile time.
var _ interface {
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
	ParsePayload(*common.RawPayload) (*common.ParsedData, error)
	ParseToEntities(string, string, *common.RawPayload, *common.Location) ([]common.Entity, error)
} = (*Parser)(nil)
