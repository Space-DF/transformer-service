package sen0313

const (
	Model        = "SEN0313"
	Manufacturer = "dfrobot"
)

// SEN0313Component implements a parser for the DFRobot A01NYUB / SEN0313 UART sensor.
type SEN0313Component struct{}

func NewSEN0313Component() *SEN0313Component { return &SEN0313Component{} }

func (p *SEN0313Component) SupportsGPS() bool        { return false }
func (p *SEN0313Component) GetSupportedPorts() []int { return nil }
func (p *SEN0313Component) GetSupportedEntityTypes() []string {
	return []string{"distance"}
}
