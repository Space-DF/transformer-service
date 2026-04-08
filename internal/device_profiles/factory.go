package deviceprofile

import "github.com/Space-DF/transformer-service/internal/device_profiles/common"

// New returns a Component with no parsers registered.
func New() *Component {
	return &Component{
		parsers:       make(map[common.DeviceType]common.Parser),
		manufacturers: make(map[string]string),
	}
}

// Register adds a parser for the given model/manufacturer into the global Component.
// Each device package calls this from its own init() function.
func Register(model, manufacturer string, parser common.Parser) {
	_global.RegisterParser(model, manufacturer, parser)
}
