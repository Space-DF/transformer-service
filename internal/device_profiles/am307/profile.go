package am307

import (
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

const (
	Model        = "AM307"
	Manufacturer = "milesight"
)

// AM307Component implements device profile for the Milesight AM307.
// AM307 is a LoRaWAN® indoor air quality sensor that detects temperature,
// humidity, CO2, TVOC, barometric pressure, light, and PIR motion.
type AM307Component struct{}

func NewAM307Component() *AM307Component { return &AM307Component{} }

func (p *AM307Component) SupportsGPS() bool        { return false }
func (p *AM307Component) GetSupportedPorts() []int { return []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10} }
func (p *AM307Component) GetSupportedEntityTypes() []string {
	return []string{
		"location",
		"temperature",
		"humidity",
		"battery",
		"occupancy",
		"pir_sensor_value",
		"pir_sensor_status",
		"light_level",
		"co2",
		"tvoc",
		"pressure",
	}
}

var _ interface {
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
	ParsePayload(*common.RawPayload) (*common.ParsedData, error)
	ParseToEntities(string, string, *common.RawPayload, *common.Location) ([]common.Entity, error)
} = (*AM307Component)(nil)
