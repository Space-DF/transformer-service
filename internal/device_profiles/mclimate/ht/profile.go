package ht

const (
	Model        = "MCLIMATE_HT"
	Manufacturer = "mclimate"
)

// MclimateHTComponent implements device profile for the Mclimate HT (with PIR Lite).
type MclimateHTComponent struct{}

func NewMclimateHTComponent() *MclimateHTComponent { return &MclimateHTComponent{} }

func (p *MclimateHTComponent) SupportsGPS() bool        { return false }
func (p *MclimateHTComponent) GetSupportedPorts() []int { return []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10} }
func (p *MclimateHTComponent) GetSupportedEntityTypes() []string {
	return []string{
		"location",
		"temperature",
		"humidity",
		"battery_voltage",
		"occupancy",
		"pir_trigger_count",
	}
}
