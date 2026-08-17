package ct101

const (
	Model        = "CT101"
	Manufacturer = "milesight"
)

// CT101Component implements common.Parser for the Milesight CT101.
type CT101Component struct{}

func NewCT101Component() *CT101Component { return &CT101Component{} }

func (p *CT101Component) SupportsGPS() bool        { return false }
func (p *CT101Component) GetSupportedPorts() []int { return []int{2} }
func (p *CT101Component) GetSupportedEntityTypes() []string {
	return []string{
		"current", "total_current", "temperature",
		"current_alarm", "temperature_alarm",
		"current_sensor_status", "temperature_sensor_status",
	}
}
