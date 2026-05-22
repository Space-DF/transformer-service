package g62

import (
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

const (
	Model        = "G62"
	Manufacturer = "digitalmatter"
)

type G62Component struct{}

func NewG62Component() *G62Component { return &G62Component{} }

func (p *G62Component) SupportsGPS() bool { return true }

func (p *G62Component) GetSupportedPorts() []int {
	return []int{1, 2, 3, 4, 5}
}

func (p *G62Component) GetSupportedEntityTypes() []string {
	return []string{
		"location", "heading", "speed", "battery_voltage",
		"external_voltage", "analog_input", "temperature",
		"gps_accuracy", "trip_type", "ignition",
		"dig_in_1", "dig_in_2", "dig_out",
		"odometer", "runtime", "firmware", "downlink_ack",
	}
}

var _ interface {
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
	ParsePayload(*common.RawPayload) (*common.ParsedData, error)
	ParseToEntities(string, string, *common.RawPayload, *common.Location) ([]common.Entity, error)
} = (*G62Component)(nil)
