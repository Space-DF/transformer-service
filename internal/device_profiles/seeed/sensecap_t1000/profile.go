package sensecap_t1000

const (
	Model        = "SENSECAP_T1000"
	Manufacturer = "seeed"
)

// SenseCapT1000Component implements devicecommon.SenseCapT1000Component for the SenseCAP T1000.
type SenseCapT1000Component struct{}

func NewSenseCapT1000Component() *SenseCapT1000Component { return &SenseCapT1000Component{} }

func (p *SenseCapT1000Component) SupportsGPS() bool        { return true }
func (p *SenseCapT1000Component) GetSupportedPorts() []int { return []int{1, 5} }
func (p *SenseCapT1000Component) GetSupportedEntityTypes() []string {
	return []string{
		"location", "battery_level", "temperature", "light",
		"motion", "shock_event", "sos_alert",
		"temperature_event", "light_event", "press_once_event",
	}
}
