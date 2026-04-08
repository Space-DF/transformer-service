package yabby_edge

import (
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

const (
	Model        = "YABBY_EDGE"
	Manufacturer = "digitalmatter"
)

// YabbyEdgeComponent implements devicecommon.YabbyEdgeComponent for the Digital Matter Yabby Edge.
type YabbyEdgeComponent struct{}

func NewYabbyEdgeComponent() *YabbyEdgeComponent { return &YabbyEdgeComponent{} }

func (p *YabbyEdgeComponent) SupportsGPS() bool { return true }
func (p *YabbyEdgeComponent) GetSupportedPorts() []int {
	return []int{
		1, 2, 3, 5, 89, 90, 91, 92,
		101, 102, 103, 104, 105, 106, 107, 108,
		109, 110, 111, 112, 113, 114, 115, 116, 202,
	}
}
func (p *YabbyEdgeComponent) GetSupportedEntityTypes() []string {
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
} = (*YabbyEdgeComponent)(nil)
