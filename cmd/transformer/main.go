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
	"log"

	"github.com/Space-DF/transformer-service/cmd/transformer/cmd"
	deviceprofile "github.com/Space-DF/transformer-service/internal/device_profiles"
	_ "github.com/Space-DF/transformer-service/internal/lns"
)

func main() {
	// Create a new component registry and register all device parsers explicitly.
	registry := deviceprofile.NewComponentRegistry()
	if err := deviceprofile.RegisterAll(registry); err != nil {
		log.Fatalf("failed to register device profiles: %v", err)
	}

	// Set the global registry so existing services can access it via Global().
	deviceprofile.SetGlobal(registry)

	cmd.Execute()
}
