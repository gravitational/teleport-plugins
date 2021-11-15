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
package accessors

import (
	"reflect"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// FromTerraformFn is a function used to read custom value from ResourceData
type FromTerraformFn func(string, reflect.Value, *SchemaMeta, *schema.Schema, *schema.ResourceData) error

// ToTerraformFn is a function used to write custom value to ResearchData. Must return value to set or nil.
type ToTerraformFn func(string, reflect.Value, *SchemaMeta, *schema.Schema, *schema.ResourceData) (interface{}, error)

// SchemaMeta represents schema metadata struct
type SchemaMeta struct {
	// Name field name in target struct
	Name string

	// IsTime is true if field contains time
	IsTime bool

	// IsDuration is true if field contains duration
	IsDuration bool

	// FromTerraform represents a reference to FromTerraform* function if value is a custom type
	FromTerraform FromTerraformFn

	// ToTerraform represents a reference to ToTerraform* function if value is a custom type
	ToTerraform ToTerraformFn

	// Nested nested message definition
	Nested map[string]*SchemaMeta
}
