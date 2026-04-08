package rak7200

import (
	deviceprofile "github.com/Space-DF/transformer-service/internal/device_profiles"
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

func init() {
	deviceprofile.Register(Model, Manufacturer, NewParser())
}

const (
	Model        = "RAK7200"
	Manufacturer = "rakwireless"
)

// Parser implements devicecommon.Parser for the RAK7200.
type Parser struct{}

func NewParser() *Parser { return &Parser{} }

func (p *Parser) SupportsGPS() bool        { return true }
func (p *Parser) GetSupportedPorts() []int { return []int{2, 3, 4, 5} }
func (p *Parser) GetSupportedEntityTypes() []string {
	return []string{"location", "battery", "temperature"}
}

var _ interface {
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
	ParsePayload(*common.RawPayload) (*common.ParsedData, error)
	ParseToEntities(string, string, *common.RawPayload, *common.Location) ([]common.Entity, error)
} = (*Parser)(nil)
