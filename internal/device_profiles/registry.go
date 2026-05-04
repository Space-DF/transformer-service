package deviceprofile

import (
	"fmt"

	"github.com/Space-DF/transformer-service/internal/device_profiles/abeeway/industrial_tracker"
	"github.com/Space-DF/transformer-service/internal/device_profiles/common"
	"github.com/Space-DF/transformer-service/internal/device_profiles/digital_matter/g62"
	"github.com/Space-DF/transformer-service/internal/device_profiles/digital_matter/yabby_edge"
	"github.com/Space-DF/transformer-service/internal/device_profiles/dragino/lcc01lb"
	"github.com/Space-DF/transformer-service/internal/device_profiles/dragino/lsn50v2_s31"
	"github.com/Space-DF/transformer-service/internal/device_profiles/dut/wlbv1"
	"github.com/Space-DF/transformer-service/internal/device_profiles/lilygo/tbeam"
	"github.com/Space-DF/transformer-service/internal/device_profiles/mclimate/ht"
	"github.com/Space-DF/transformer-service/internal/device_profiles/milesight/am307"
	"github.com/Space-DF/transformer-service/internal/device_profiles/milesight/ct101"
	"github.com/Space-DF/transformer-service/internal/device_profiles/netvox/r718n17"
	"github.com/Space-DF/transformer-service/internal/device_profiles/netvox/r809ag"
	"github.com/Space-DF/transformer-service/internal/device_profiles/quandify/cubicmeter"
	"github.com/Space-DF/transformer-service/internal/device_profiles/rakwireless/rak2270"
	"github.com/Space-DF/transformer-service/internal/device_profiles/rakwireless/rak4630"
	"github.com/Space-DF/transformer-service/internal/device_profiles/rakwireless/rak7200"
	"github.com/Space-DF/transformer-service/internal/device_profiles/seeed/sensecap_t1000"
	"github.com/Space-DF/transformer-service/internal/services"
)

// NewComponentRegistry creates a new Component for dependency injection.
// Call RegisterAll to populate it with all known device parsers.
func NewComponentRegistry() *Component {
	return New()
}

// RegisterAll explicitly registers all known device parsers into r with error handling.
func RegisterAll(r *Component, locationService *services.LocationService) error {
	entries := []struct {
		model        string
		manufacturer string
		parser       common.Parser
	}{
		{industrial_tracker.Model, industrial_tracker.Manufacturer, industrial_tracker.NewAbeewayComponent()},
		{r718n17.Model, r718n17.Manufacturer, r718n17.NewNetvoxR718N17Component()},
		{r809ag.Model, r809ag.Manufacturer, r809ag.NewR809AGComponent()},
		{lcc01lb.Model, lcc01lb.Manufacturer, lcc01lb.NewLCC01LBComponent()},
		{lsn50v2_s31.Model, lsn50v2_s31.Manufacturer, lsn50v2_s31.NewLSN50v2S31Component()},
		{am307.Model, am307.Manufacturer, am307.NewAM307Component()},
		{cubicmeter.Model, cubicmeter.Manufacturer, cubicmeter.NewCubicMeterComponent()},
		{ct101.Model, ct101.Manufacturer, ct101.NewCT101Component()},
		{ht.Model, ht.Manufacturer, ht.NewMclimateHTComponent()},
		{rak2270.Model, rak2270.Manufacturer, rak2270.NewRAK2270Component()},
		{rak4630.Model, rak4630.Manufacturer, rak4630.NewRAK4630Component(locationService)},
		{rak7200.Model, rak7200.Manufacturer, rak7200.NewRAK7200Component()},
		{sensecap_t1000.Model, sensecap_t1000.Manufacturer, sensecap_t1000.NewSenseCapT1000Component()},
		{tbeam.Model, tbeam.Manufacturer, tbeam.NewTBeamComponent()},
		{wlbv1.Model, wlbv1.Manufacturer, wlbv1.NewWLBV1Component()},
		{yabby_edge.Model, yabby_edge.Manufacturer, yabby_edge.NewYabbyEdgeComponent()},
		{g62.Model, g62.Manufacturer, g62.NewG62Component()},
	}

	for _, e := range entries {
		if e.model == "" {
			return fmt.Errorf("device model name must not be empty")
		}
		r.RegisterParser(e.model, e.manufacturer, e.parser)
	}
	return nil
}
