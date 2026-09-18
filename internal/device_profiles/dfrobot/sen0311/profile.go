package sen0311

const (
	Model        = "SEN0311"
	Manufacturer = "dfrobot"
)

// SEN0311Component implements a parser for the DFRobot A02YYUW / SEN0311 UART sensor.
type SEN0311Component struct{}

func NewSEN0311Component() *SEN0311Component { return &SEN0311Component{} }

func (p *SEN0311Component) SupportsGPS() bool        { return false }
func (p *SEN0311Component) GetSupportedPorts() []int { return nil }
func (p *SEN0311Component) GetSupportedEntityTypes() []string {
	return []string{"water_depth"}
}
