package rak4630

const (
	Model        = "RAK4630"
	Manufacturer = "rakwireless"
)

// RAK4630Component implements devicecommon.RAK4630Component for the RAK4630.
type RAK4630Component struct{}

func NewRAK4630Component() *RAK4630Component { return &RAK4630Component{} }

func (p *RAK4630Component) SupportsGPS() bool        { return true }
func (p *RAK4630Component) GetSupportedPorts() []int { return []int{1, 2, 3, 4, 5} }
func (p *RAK4630Component) GetSupportedEntityTypes() []string {
	return []string{"location", "temperature", "humidity", "pressure", "battery_voltage"}
}
