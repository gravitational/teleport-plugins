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

// Getter is a function used to get custom value from ResourceData
type Getter func(string, reflect.Value, *SchemaMeta, *schema.Schema, *schema.ResourceData) error

// Setter is a function used to set custom value to ResearchData. Must return value to set.
type Setter func(reflect.Value, *SchemaMeta, *schema.Schema) (interface{}, error)

// SchemaMeta represents schema metadata struct
type SchemaMeta struct {
	// Name field name in target struct
	Name string

	// IsTime is true if field contains time
	IsTime bool

	// IsDuration is true if field contains duration
	IsDuration bool

	// Getter contains reference to getter function if value is a custom type
	Getter Getter

	// Setter contains reference to setter function if value is a custom type
	Setter Setter

	// Nested nested message definition
	Nested map[string]*SchemaMeta
}
