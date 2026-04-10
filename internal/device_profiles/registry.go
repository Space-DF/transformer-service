package deviceprofile

import (
	"fmt"

	"github.com/Space-DF/transformer-service/internal/device_profiles/abeeway"
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
	"github.com/Space-DF/transformer-service/internal/device_profiles/netvox"
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
	entries := []struct {
		model        string
		manufacturer string
		parser       common.Parser
	}{
		{abeeway.Model, abeeway.Manufacturer, abeeway.NewAbeewayComponent()},
		{netvox.Model, netvox.Manufacturer, netvox.NewNetvoxR718N17Component()},
		{rak2270.Model, rak2270.Manufacturer, rak2270.NewRAK2270Component()},
		{rak4630.Model, rak4630.Manufacturer, rak4630.NewRAK4630Component()},
		{rak7200.Model, rak7200.Manufacturer, rak7200.NewRAK7200Component()},
		{sensecap_t1000.Model, sensecap_t1000.Manufacturer, sensecap_t1000.NewSenseCapT1000Component()},
		{tbeam.Model, tbeam.Manufacturer, tbeam.NewTBeamComponent()},
		{wlbv1.Model, wlbv1.Manufacturer, wlbv1.NewWLBV1Component()},
		{yabby_edge.Model, yabby_edge.Manufacturer, yabby_edge.NewYabbyEdgeComponent()},
	}

	for _, e := range entries {
		if e.model == "" {
			return fmt.Errorf("device model name must not be empty")
		}
		r.RegisterParser(e.model, e.manufacturer, e.parser)
	}
	return nil
}
