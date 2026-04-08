package rak4630

import (
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

const (
	Model        = "RAK4630"
	Manufacturer = "rakwireless"
)

// Parser implements devicecommon.Parser for the RAK4630.
type Parser struct{}

func NewParser() *Parser { return &Parser{} }

func (p *Parser) SupportsGPS() bool        { return true }
func (p *Parser) GetSupportedPorts() []int { return []int{1, 2, 3, 4, 5} }
func (p *Parser) GetSupportedEntityTypes() []string {
	return []string{"location", "temperature", "humidity", "pressure", "battery_voltage"}
}

var _ interface {
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
	ParsePayload(*common.RawPayload) (*common.ParsedData, error)
	ParseToEntities(string, string, *common.RawPayload, *common.Location) ([]common.Entity, error)
} = (*Parser)(nil)
