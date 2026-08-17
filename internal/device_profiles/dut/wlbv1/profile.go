package wlbv1

const (
	Model        = "WLBV1"
	Manufacturer = "dut"
)

// WLBV1Component implements devicecommon.WLBV1Component for the WLBV1 water-level sensor.
type WLBV1Component struct{}

func NewWLBV1Component() *WLBV1Component { return &WLBV1Component{} }

func (p *WLBV1Component) SupportsGPS() bool        { return true }
func (p *WLBV1Component) GetSupportedPorts() []int { return []int{1, 2, 3, 4, 5} }
func (p *WLBV1Component) GetSupportedEntityTypes() []string {
	return []string{"location", "battery", "water_depth"}
}
