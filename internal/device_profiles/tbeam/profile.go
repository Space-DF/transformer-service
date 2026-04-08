package tbeam

import (
	deviceprofile "github.com/Space-DF/transformer-service/internal/device_profiles"
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

func init() {
	deviceprofile.Register(Model, Manufacturer, NewParser())
}

const (
	Model        = "TBEAM"
	Manufacturer = "lilygo"
)

// Parser implements devicecommon.Parser for the LilyGO T-Beam (Cayenne LPP).
type Parser struct{}

func NewParser() *Parser { return &Parser{} }

func (p *Parser) SupportsGPS() bool        { return true }
func (p *Parser) GetSupportedPorts() []int { return []int{1, 2, 5} }
func (p *Parser) GetSupportedEntityTypes() []string {
	return []string{"location", "temperature", "humidity", "pressure", "illuminance"}
}

var _ interface {
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
	ParsePayload(*common.RawPayload) (*common.ParsedData, error)
	ParseToEntities(string, string, *common.RawPayload, *common.Location) ([]common.Entity, error)
} = (*Parser)(nil)
