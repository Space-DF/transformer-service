package ht

import (
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

const (
	Model        = "MCLIMATE_HT"
	Manufacturer = "mclimate"
)

// MclimateHTComponent implements device profile for the Mclimate HT (with PIR Lite).
type MclimateHTComponent struct{}

func NewMclimateHTComponent() *MclimateHTComponent { return &MclimateHTComponent{} }

func (p *MclimateHTComponent) SupportsGPS() bool        { return false }
func (p *MclimateHTComponent) GetSupportedPorts() []int { return []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10} }
func (p *MclimateHTComponent) GetSupportedEntityTypes() []string {
	return []string{
		"location",
		"temperature",
		"humidity",
		"battery_voltage",
		"occupancy",
		"pir_trigger_count",
	}
}

var _ interface {
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
	ParsePayload(*common.RawPayload) (*common.ParsedData, error)
	ParseToEntities(string, string, *common.RawPayload, *common.Location) ([]common.Entity, error)
} = (*MclimateHTComponent)(nil)
