package yabby_edge

import (
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

const (
	Model        = "YABBY_EDGE"
	Manufacturer = "digitalmatter"
)

// Parser implements devicecommon.Parser for the Digital Matter Yabby Edge.
type Parser struct{}

func NewParser() *Parser { return &Parser{} }

func (p *Parser) SupportsGPS() bool { return true }
func (p *Parser) GetSupportedPorts() []int {
	return []int{
		1, 2, 3, 5, 89, 90, 91, 92,
		101, 102, 103, 104, 105, 106, 107, 108,
		109, 110, 111, 112, 113, 114, 115, 116, 202,
	}
}
func (p *Parser) GetSupportedEntityTypes() []string {
	return []string{
		"location", "battery_voltage", "battery_level",
		"trip_status", "inactivity", "trip_count",
		"firmware", "battery_stats", "downlink_ack", "connect",
	}
}

var _ interface {
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
	ParsePayload(*common.RawPayload) (*common.ParsedData, error)
	ParseToEntities(string, string, *common.RawPayload, *common.Location) ([]common.Entity, error)
} = (*Parser)(nil)
