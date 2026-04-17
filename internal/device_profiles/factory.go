package deviceprofile

import "github.com/Space-DF/transformer-service/internal/device_profiles/common"

// New returns a Component with no parsers registered.
func New() *Component {
	return &Component{
		parsers:       make(map[common.DeviceType]common.Parser),
		manufacturers: make(map[string]string),
	}
}
