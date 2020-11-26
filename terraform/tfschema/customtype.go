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

package tfschema

import (
	"fmt"
	"reflect"
	time "time"

	"github.com/gravitational/protoc-gen-terraform/accessors"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/api/types/wrappers"
	"github.com/gravitational/teleport/api/utils"
	"github.com/gravitational/trace"
	schema "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// TruncateMs truncates nanoseconds from Metadata.Expires to prevent state change.
// Errors are silenced because this function can not report error to Terraform
func TruncateMs(v interface{}) string {
	value, ok := v.(string)
	if !ok {
		return ""
	}

	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return value
	}

	return t.Truncate(time.Second).Format(time.RFC3339Nano)
}

// SchemaBoolOption represents schema of custom bool value with true default
func SchemaBoolOption() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeBool,
		Optional: true,
		Default:  true,
	}
}

// SchemaTraits represents traits schema
func SchemaTraits() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeSet,
		Optional: true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"key": {
					Type:     schema.TypeString,
					Required: true,
				},
				"value": {
					Type:     schema.TypeList,
					Required: true,
					Elem: &schema.Schema{
						Type: schema.TypeString,
					},
				},
			},
		},
	}
}

// SchemaLabels represents traits schema map[string]utils.Strings
func SchemaLabels() *schema.Schema {
	return SchemaTraits()
}

// GetBoolOption unmarshals bool value
func GetBoolOption(path string, target reflect.Value, meta *accessors.SchemaMeta, sch *schema.Schema, data *schema.ResourceData) error {
	v := data.Get(path)

	b, ok := v.(bool)
	if !ok {
		return trace.Errorf("can not convert %T to bool", v)
	}

	o := &types.BoolOption{Value: b}
	target.Set(reflect.ValueOf(o))

	return nil
}

// getArrayMap gets map of arrays from data to object
func getArrayMap(
	obj interface{},
	sliceT reflect.Type,
	path string,
	target reflect.Value,
	meta *accessors.SchemaMeta,
	sch *schema.Schema,
	data *schema.ResourceData,
) error {
	l, err := accessors.GetLen(path, data)
	if err != nil {
		return trace.Wrap(err)
	}

	if l == 0 {
		target.Set(reflect.ValueOf(obj))
		return nil
	}

	raw := data.Get(path)

	s, ok := raw.(*schema.Set)
	if !ok {
		return trace.Errorf("can not convert %T to *schema.Set", raw)
	}

	for _, i := range s.List() {
		h := s.F(i)

		k := data.Get(fmt.Sprintf("%v.%v.key", path, h))
		v := data.Get(fmt.Sprintf("%v.%v.value", path, h))

		vi, ok := v.([]interface{})
		if !ok {
			return trace.Errorf("can not convert %T to []interface{}", v)
		}

		t := reflect.MakeSlice(sliceT, len(vi), len(vi))
		for i, s := range vi {
			t.Index(i).Set(reflect.ValueOf(s))
		}

		reflect.ValueOf(obj).SetMapIndex(reflect.ValueOf(k), t)
	}

	if s.Len() > 0 {
		target.Set(reflect.ValueOf(obj))
	}

	return nil
}

// GetLabels unmarshals wrapper value map[string]utils.Strings
func GetLabels(path string, target reflect.Value, meta *accessors.SchemaMeta, sch *schema.Schema, data *schema.ResourceData) error {
	t := reflect.TypeOf((*utils.Strings)(nil)).Elem()

	return getArrayMap(
		make(types.Labels),
		t,
		path,
		target,
		meta,
		sch,
		data,
	)
}

// GetTraits unmarshals traits value map[string][]string
func GetTraits(path string, target reflect.Value, meta *accessors.SchemaMeta, sch *schema.Schema, data *schema.ResourceData) error {
	t := reflect.TypeOf((*[]string)(nil)).Elem()

	return getArrayMap(
		make(wrappers.Traits),
		t,
		path,
		target,
		meta,
		sch,
		data,
	)
}

func SetBoolOption(source reflect.Value, meta *accessors.SchemaMeta, sch *schema.Schema) (interface{}, error) {
	if !source.IsValid() {
		return nil, nil
	}

	if source.IsNil() {
		return nil, nil
	}

	i := reflect.Indirect(source).Interface()
	v, ok := i.(types.BoolOption)
	if !ok {
		return nil, trace.Errorf("can not convert %T to types.BoolOption", v)
	}

	return v.Value, nil
}

// setArrayMap sets map of labels/traits to data source
func setArrayMap(source reflect.Value, sliceT reflect.Type, meta *accessors.SchemaMeta, sch *schema.Schema) (interface{}, error) {
	if source.Len() == 0 {
		return nil, nil
	}

	s, ok := sch.ZeroValue().(*schema.Set)
	if !ok {
		return nil, trace.Errorf("zero value for schema set element is not *schema.Set")
	}

	for _, k := range source.MapKeys() {
		i := source.MapIndex(k)

		if !i.Type().ConvertibleTo(sliceT) {
			return nil, trace.Errorf("can not convert to %v", sliceT)
		}

		a := i.Convert(sliceT)
		v := make([]interface{}, a.Len())
		for i := 0; i < a.Len(); i++ {
			v[i] = a.Index(i).Interface()
		}

		t := map[string]interface{}{
			"key":   k.Interface(),
			"value": v}

		s.Add(t)
	}

	if s.Len() > 0 {
		return s, nil
	}

	return nil, nil
}

// SetLables sets labels from data to object
func SetLabels(source reflect.Value, meta *accessors.SchemaMeta, sch *schema.Schema) (interface{}, error) {
	return setArrayMap(source, reflect.TypeOf((utils.Strings)(nil)), meta, sch)
}

// SetLables sets traits from data to object
func SetTraits(source reflect.Value, meta *accessors.SchemaMeta, sch *schema.Schema) (interface{}, error) {
	return setArrayMap(source, reflect.TypeOf(([]string)(nil)), meta, sch)
}
