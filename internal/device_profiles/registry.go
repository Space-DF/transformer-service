package deviceprofile

import "github.com/Space-DF/transformer-service/internal/services"

//go:generate go run ./genregistrars

// NewComponentRegistry creates a new Component for dependency injection.
// Call RegisterAll to populate it with all known device parsers.
func NewComponentRegistry() *Component {
	return New()
}

// RegisterAll explicitly registers all known device parsers into r with error handling.
func RegisterAll(r *Component, locationService *services.LocationService) error {
	for _, register := range generatedRegistrars {
		if err := register(r); err != nil {
			return err
		}
	}
	return nil
}
