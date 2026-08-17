package rak7200

const (
	Model        = "RAK7200"
	Manufacturer = "rakwireless"
)

// RAK7200Component implements devicecommon.RAK7200Component for the RAK7200.
type RAK7200Component struct{}

func NewRAK7200Component() *RAK7200Component { return &RAK7200Component{} }

func (p *RAK7200Component) SupportsGPS() bool        { return true }
func (p *RAK7200Component) GetSupportedPorts() []int { return []int{2, 3, 4, 5} }
func (p *RAK7200Component) GetSupportedEntityTypes() []string {
	return []string{"location", "battery", "temperature"}
}
