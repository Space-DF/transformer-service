package models

// DeviceModel represents the API response structure for a device model
type DeviceModel struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	ManufacturerID   string `json:"manufacturer_id"`
	ManufacturerName string `json:"manufacturer_name"`
	DeviceType       string `json:"device_type"`
	KeyFeature       string `json:"key_feature"`
	Logo             string `json:"logo"`
}

// DeviceEntityTemplate represents a bootstrap entity definition derived from a device model.
type DeviceEntityTemplate struct {
	Key          string   `json:"key"`
	UniqueID     string   `json:"unique_id"`
	ModelKey     string   `json:"model_key,omitempty"`
	EntityType   string   `json:"entity_type"`
	Category     string   `json:"category"`
	Name         string   `json:"name"`
	Manufacturer string   `json:"manufacturer,omitempty"`
	UnitOfMeas   string   `json:"unit_of_measurement,omitempty"`
	Icon         string   `json:"icon,omitempty"`
	DisplayType  []string `json:"display_type,omitempty"`
}

// DeviceProfile represents the YAML structure for device profiles
type DeviceProfile struct {
	ID                    string   `yaml:"id"`
	Name                  string   `yaml:"name"`
	ManufacturerID        string   `yaml:"manufacturer_id"`
	DeviceType            string   `yaml:"device_type"`
	KeyFeature            string   `yaml:"key_feature"`
	Logo                  string   `yaml:"logo"`
	Protocol              string   `yaml:"protocol"`
	Capabilities          []string `yaml:"capabilities"`
	GPSCapable            bool     `yaml:"gps_capable"`
	RequiresTrilateration bool     `yaml:"requires_trilateration"`
	SupportedFPorts       []int    `yaml:"supported_fports"`
	CreatedAt             string   `yaml:"created_at"`
	UpdatedAt             string   `yaml:"updated_at"`
}

// Manufacturer represents manufacturer information
type Manufacturer struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	PortalURL   string `yaml:"portal_url"`
}

// DeviceProfileConfig represents the root YAML structure for device profiles
type DeviceProfileConfig struct {
	DeviceProfiles []DeviceProfile `yaml:"device_profiles"`
}

// ManufacturerConfig represents the root YAML structure for manufacturers
type ManufacturerConfig struct {
	Manufacturers []Manufacturer `yaml:"manufacturers"`
}
