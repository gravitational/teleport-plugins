/*
Copyright 2015-2021 Gravitational, Inc.

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
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/gravitational/trace"
)

func init() {
	// initConfig()
}

var cli CLI

func main() {
	ctx := kong.Parse(
		&cli,
		kong.UsageOnError(),
		kong.Configuration(TOML),
		kong.Name("Teleport event handler"),
		kong.Description("Forwards Teleport AuditLog to external sources"),
	)

	// See respective commands Run() methods
	err := ctx.Run()
	if cli.Debug {
		fmt.Printf("%v\n", trace.DebugReport(err))
	}
	ctx.FatalIfErrorf(err)
}
