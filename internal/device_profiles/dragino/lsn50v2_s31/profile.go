package lsn50v2_s31

import (
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

const (
	Model        = "LSN50V2_S31"
	Manufacturer = "dragino"
)

// LSN50v2S31Component implements the common.Parser interface for the Dragino LSN50v2-S31/S31B.
// LoRaWAN sensor node with multiple work modes (IIC, Distance, 3ADC, 3DS18B20, Weight, Count).
type LSN50v2S31Component struct{}

func NewLSN50v2S31Component() *LSN50v2S31Component { return &LSN50v2S31Component{} }

func (p *LSN50v2S31Component) SupportsGPS() bool        { return false }
func (p *LSN50v2S31Component) GetSupportedPorts() []int { return []int{2, 3, 5} }
func (p *LSN50v2S31Component) GetSupportedEntityTypes() []string {
	return []string{
		"battery_voltage",
		"exti_trigger",
		"door_status",
		"work_mode",
		"temperature_sht31",
		"humidity_sht31",
		"data_time",
		"sht_temp_min",
		"sht_temp_max",
		"sht_hum_min",
		"sht_hum_max",
		"firmware_version",
		"freq_band",
		"tdc_sec",
	}
}

var _ interface {
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
	ParsePayload(*common.RawPayload) (*common.ParsedData, error)
	ParseToEntities(string, string, *common.RawPayload, *common.Location) ([]common.Entity, error)
} = (*LSN50v2S31Component)(nil)
