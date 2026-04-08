package rak2270

import (
	deviceprofile "github.com/Space-DF/transformer-service/internal/device_profiles"
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

func init() {
	deviceprofile.Register(Model, Manufacturer, NewParser())
}

const (
	Model        = "RAK2270"
	Manufacturer = "rakwireless"
)

// Parser implements devicecommon.Parser for the RAK2270.
type Parser struct{}

func NewParser() *Parser { return &Parser{} }

func (p *Parser) SupportsGPS() bool        { return false }
func (p *Parser) GetSupportedPorts() []int { return []int{1, 2, 3} }
func (p *Parser) GetSupportedEntityTypes() []string {
	return []string{"location", "temperature", "battery_voltage"}
}

var _ interface {
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
	ParsePayload(*common.RawPayload) (*common.ParsedData, error)
	ParseToEntities(string, string, *common.RawPayload, *common.Location) ([]common.Entity, error)
} = (*Parser)(nil)
