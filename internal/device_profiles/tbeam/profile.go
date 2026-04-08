package tbeam

import (
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
)

const (
	Model        = "TBEAM"
	Manufacturer = "lilygo"
)

// TBeamComponent implements devicecommon.TBeamComponent for the LilyGO T-Beam (Cayenne LPP).
type TBeamComponent struct{}

func NewTBeamComponent() *TBeamComponent { return &TBeamComponent{} }

func (p *TBeamComponent) SupportsGPS() bool        { return true }
func (p *TBeamComponent) GetSupportedPorts() []int { return []int{1, 2, 5} }
func (p *TBeamComponent) GetSupportedEntityTypes() []string {
	return []string{"location", "temperature", "humidity", "pressure", "illuminance"}
}

var _ interface {
	SupportsGPS() bool
	GetSupportedPorts() []int
	GetSupportedEntityTypes() []string
	ParsePayload(*common.RawPayload) (*common.ParsedData, error)
	ParseToEntities(string, string, *common.RawPayload, *common.Location) ([]common.Entity, error)
} = (*TBeamComponent)(nil)
