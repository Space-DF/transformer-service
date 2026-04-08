/*
Copyright 2026 Digital Fortress.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"github.com/Space-DF/transformer-service/cmd/transformer/cmd"

	_ "github.com/Space-DF/transformer-service/internal/device_profiles/abeeway"
	_ "github.com/Space-DF/transformer-service/internal/device_profiles/rak2270"
	_ "github.com/Space-DF/transformer-service/internal/device_profiles/rak4630"
	_ "github.com/Space-DF/transformer-service/internal/device_profiles/rak7200"
	_ "github.com/Space-DF/transformer-service/internal/device_profiles/sensecap_t1000"
	_ "github.com/Space-DF/transformer-service/internal/device_profiles/tbeam"
	_ "github.com/Space-DF/transformer-service/internal/device_profiles/wlbv1"
	_ "github.com/Space-DF/transformer-service/internal/device_profiles/yabby_edge"
	_ "github.com/Space-DF/transformer-service/internal/lns"
)

func main() {
	cmd.Execute()
}
