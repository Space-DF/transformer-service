package industrial_tracker

import "github.com/Space-DF/transformer-service/internal/device_profiles/common"

const (
	Model        = "ABEEWAY_INDUSTRIAL_TRACKER"
	Manufacturer = "abeeway"
)

// AbeewayComponent implements devicecommon.AbeewayComponent for the Abeeway Industrial Tracker.
type AbeewayComponent struct{}

func NewAbeewayComponent() *AbeewayComponent { return &AbeewayComponent{} }

func Register(r common.ParserRegistry) error {
	return common.RegisterParser(r, Model, Manufacturer, NewAbeewayComponent())
}

func (p *AbeewayComponent) SupportsGPS() bool        { return true }
func (p *AbeewayComponent) GetSupportedPorts() []int { return []int{1, 2, 5, 17, 100} }
func (p *AbeewayComponent) GetSupportedEntityTypes() []string {
	return []string{
		"location", "battery_voltage", "battery_percent",
		"temperature", "speed", "heading", "sos_alert", "motion",
	}
}
