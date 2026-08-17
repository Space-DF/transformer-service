package cubicmeter

const (
	Model        = "QUANDIFY_CUBICMETER"
	Manufacturer = "quandify"
)

// CubicMeterComponent implements the Parser interface for the Quandify Cubicmeter 1.1.
// It is a water metering device with no built-in GPS.
type CubicMeterComponent struct{}

func NewCubicMeterComponent() *CubicMeterComponent { return &CubicMeterComponent{} }

func (p *CubicMeterComponent) SupportsGPS() bool        { return false }
func (p *CubicMeterComponent) GetSupportedPorts() []int { return []int{1, 6} }
func (p *CubicMeterComponent) GetSupportedEntityTypes() []string {
	return []string{
		"total_volume",
		"ambient_temperature",
		"water_temperature_min",
		"water_temperature_max",
		"battery_active",
		"battery_recovered",
		"error_code",
		"leak_state",
		"is_sensing",
	}
}
