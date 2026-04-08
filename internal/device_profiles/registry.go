package deviceprofile

import (
	"fmt"

	"github.com/Space-DF/transformer-service/internal/device_profiles/abeeway"
	"github.com/Space-DF/transformer-service/internal/device_profiles/rak2270"
	"github.com/Space-DF/transformer-service/internal/device_profiles/rak4630"
	"github.com/Space-DF/transformer-service/internal/device_profiles/rak7200"
	"github.com/Space-DF/transformer-service/internal/device_profiles/sensecap_t1000"
	"github.com/Space-DF/transformer-service/internal/device_profiles/tbeam"
	"github.com/Space-DF/transformer-service/internal/device_profiles/wlbv1"
	"github.com/Space-DF/transformer-service/internal/device_profiles/yabby_edge"
)

// NewComponentRegistry creates a new Component for dependency injection.
// Call RegisterAll to populate it with all known device parsers.
func NewComponentRegistry() *Component {
	return New()
}

// RegisterAll explicitly registers all known device parsers into r with error handling.
func RegisterAll(r *Component) error {
	parsers := []struct {
		model        string
		manufacturer string
		register     func()
	}{
		{abeeway.Model, abeeway.Manufacturer, func() {
			r.RegisterParser(abeeway.Model, abeeway.Manufacturer, abeeway.NewParser())
		}},
		{rak2270.Model, rak2270.Manufacturer, func() {
			r.RegisterParser(rak2270.Model, rak2270.Manufacturer, rak2270.NewParser())
		}},
		{rak4630.Model, rak4630.Manufacturer, func() {
			r.RegisterParser(rak4630.Model, rak4630.Manufacturer, rak4630.NewParser())
		}},
		{rak7200.Model, rak7200.Manufacturer, func() {
			r.RegisterParser(rak7200.Model, rak7200.Manufacturer, rak7200.NewParser())
		}},
		{sensecap_t1000.Model, sensecap_t1000.Manufacturer, func() {
			r.RegisterParser(sensecap_t1000.Model, sensecap_t1000.Manufacturer, sensecap_t1000.NewParser())
		}},
		{tbeam.Model, tbeam.Manufacturer, func() {
			r.RegisterParser(tbeam.Model, tbeam.Manufacturer, tbeam.NewParser())
		}},
		{wlbv1.Model, wlbv1.Manufacturer, func() {
			r.RegisterParser(wlbv1.Model, wlbv1.Manufacturer, wlbv1.NewParser())
		}},
		{yabby_edge.Model, yabby_edge.Manufacturer, func() {
			r.RegisterParser(yabby_edge.Model, yabby_edge.Manufacturer, yabby_edge.NewParser())
		}},
	}

	for _, p := range parsers {
		if p.model == "" {
			return fmt.Errorf("device model name must not be empty")
		}
		p.register()
	}
	return nil
}
