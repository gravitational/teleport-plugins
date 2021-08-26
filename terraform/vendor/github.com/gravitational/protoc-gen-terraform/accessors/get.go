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

// Package accessors contains Get and Set methods for ResourceData
package accessors

import (
	"fmt"
	"reflect"
	"time"

	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Get reads object data from schema.ResourceData to object
//
// Example:
//   user := UserV2{}
//   Get(&user, data, SchemaUserV2, MetaUserV2)
func Get(
	obj interface{},
	data *schema.ResourceData,
	sch map[string]*schema.Schema,
	meta map[string]*SchemaMeta,
) error {
	if obj == nil {
		return trace.Errorf("obj must not be nil")
	}

	value := reflect.ValueOf(obj)
	if value.Kind() != reflect.Ptr {
		return trace.Errorf("obj must be a pointer")
	}

	value = reflect.Indirect(value)

	return getFragment("", value, meta, sch, data)
}

// GetLen returns TypeSet or TypeList value length
func GetLen(path string, data *schema.ResourceData) (int, error) {
	num, ok := data.GetOk(path + ".#") // Terraform stores collection length in "collection_name.#" key
	if !ok || num == nil {
		return 0, nil
	}

	len, ok := num.(int)
	if !ok {
		return 0, trace.Errorf("failed to convert list count to number %s", path)
	}

	return len, nil
}

// getFragment iterates over a schema fragment and calls appropriate getters for a fields of passed target.
// Target must point to a struct.
func getFragment(
	path string,
	target reflect.Value,
	meta map[string]*SchemaMeta,
	sch map[string]*schema.Schema,
	data *schema.ResourceData,
) error {
	for key, fieldMeta := range meta {
		fieldSchema, ok := sch[key]
		if !ok {
			return trace.Errorf("field %v%v not found in corresponding schema", path, key)
		}

		fieldValue := target.FieldByName(fieldMeta.Name)
		if !fieldValue.IsValid() {
			return trace.Errorf("field %v%v not found in target struct", path, key)
		}

		fieldPath := path + key

		if fieldMeta.Getter != nil {
			err := fieldMeta.Getter(fieldPath, fieldValue, fieldMeta, fieldSchema, data)
			if err != nil {
				return trace.Wrap(err)
			}

			continue
		}

		switch fieldSchema.Type {
		case schema.TypeInt, schema.TypeFloat, schema.TypeBool, schema.TypeString:
			err := getElementary(fieldPath, fieldValue, fieldMeta, fieldSchema, data)
			if err != nil {
				return trace.Wrap(err)
			}
		case schema.TypeList:
			err := getList(fieldPath, fieldValue, fieldMeta, fieldSchema, data)
			if err != nil {
				return trace.Wrap(err)
			}

		case schema.TypeMap:
			err := getMap(fieldPath, fieldValue, fieldMeta, fieldSchema, data)
			if err != nil {
				return trace.Wrap(err)
			}

		case schema.TypeSet:
			err := getSet(fieldPath, fieldValue, fieldMeta, fieldSchema, data)
			if err != nil {
				return trace.Wrap(err)
			}

		default:
			return trace.Errorf("unknown type %v for %s", fieldSchema.Type.String(), fieldPath)
		}
	}

	return nil
}

// getEnumerableElement gets singular slice element from a resource data. If enumerable element is empty, it assigns
// an empty value to the target.
func getEnumerableElement(
	path string,
	target reflect.Value,
	sch *schema.Schema,
	meta *SchemaMeta,
	data *schema.ResourceData,
) error {
	switch s := sch.Elem.(type) {
	case *schema.Schema:
		return getElementary(path, target, meta, s, data)
	case *schema.Resource:
		v := newEmptyValue(target.Type())

		_, ok := data.GetOk(path)
		if ok {
			err := getFragment(path+".", v, meta.Nested, s.Schema, data)
			if err != nil {
				return trace.Wrap(err)
			}
		}

		return assign(v, target)
	default:
		return trace.Errorf("unknown Elem type: %T", sch.Elem)
	}
}

// getElementary gets elementary value (scalar, string, time, duration)
func getElementary(path string, target reflect.Value, meta *SchemaMeta, sch *schema.Schema, data *schema.ResourceData) error {
	value, ok := data.GetOk(path)

	if !ok {
		AssignZeroValue(target)
		return nil
	}

	switch {
	case meta.IsTime:
		err := assignTime(value, target)
		if err != nil {
			return trace.Wrap(err)
		}
	case meta.IsDuration:
		err := assignDuration(value, target)
		if err != nil {
			return trace.Wrap(err)
		}
	default:
		v := reflect.ValueOf(value)
		err := assign(v, target)
		if err != nil {
			return trace.Wrap(err)
		}
	}

	return nil
}

// assignTime parses time value from a string and assigns it to target
func assignTime(source interface{}, target reflect.Value) error {
	value, ok := source.(string)
	if !ok {
		return trace.Errorf("can not convert %T to string", source)
	}

	parsedTime, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return trace.Errorf("can not parse time: %w", err)
	}

	v := reflect.ValueOf(parsedTime)
	return assign(v, target)
}

// assignTime parses duration value from a string and assigns it to target
func assignDuration(source interface{}, target reflect.Value) error {
	value, ok := source.(string)
	if !ok {
		return trace.Errorf("can not convert %T to string", source)
	}

	parsedDuration, err := time.ParseDuration(value)
	if err != nil {
		return trace.Errorf("can not parse duration: %w", err)
	}

	v := reflect.ValueOf(parsedDuration)
	return assign(v, target)
}

// setList gets list from ResourceData
func getList(path string, target reflect.Value, meta *SchemaMeta, sch *schema.Schema, data *schema.ResourceData) error {
	len, err := GetLen(path, data)
	if err != nil {
		return trace.Wrap(err)
	}

	// Empty list: do nothing, but set target field to empty value
	if len == 0 {
		AssignZeroValue(target)
		return nil
	}

	// Target is a slice of elementary values or objects
	if target.Type().Kind() == reflect.Slice {
		slice := reflect.MakeSlice(target.Type(), len, len)

		for i := 0; i < len; i++ {
			value := slice.Index(i)
			elementPath := fmt.Sprintf("%v.%v", path, i)

			err := getEnumerableElement(elementPath, value, sch, meta, data)
			if err != nil {
				return trace.Wrap(err)
			}
		}

		return assign(slice, target)
	}

	// Target is an object represented by a single element list
	return getEnumerableElement(path+".0", target, sch, meta, data)
}

// setMap sets map of elementary values (scalar, string, time, duration)
func getMap(path string, target reflect.Value, meta *SchemaMeta, sch *schema.Schema, data *schema.ResourceData) error {
	raw, ok := data.GetOk(path)
	if !ok {
		return nil
	}

	sourceMap, ok := raw.(map[string]interface{})
	if !ok {
		return trace.Errorf("failed to convert %T to map[string]interface{}", raw)
	}

	// If map is empty, set target empty map
	if len(sourceMap) == 0 {
		AssignZeroValue(target)
		return nil
	}

	if target.Type().Kind() != reflect.Map {
		return trace.Errorf("target time is not a map")
	}

	targetMap := reflect.MakeMap(target.Type())

	// Iterate over map keys
	for key := range sourceMap {
		value := newEmptyValue(target.Type().Elem())

		err := getEnumerableElement(path+"."+key, value, sch, meta, data)
		if err != nil {
			return trace.Wrap(err)
		}

		assignMapIndex(targetMap, reflect.ValueOf(key), value)
	}

	return assign(targetMap, target)
}

// setSet reads set from resource data
func getSet(path string, target reflect.Value, meta *SchemaMeta, sch *schema.Schema, data *schema.ResourceData) error {
	len, err := GetLen(path, data)
	if err != nil {
		return trace.Wrap(err)
	}

	if len == 0 {
		AssignZeroValue(target)
		return nil
	}

	raw, ok := data.GetOk(path)
	if !ok {
		return trace.Errorf("can not read key " + path)
	}

	set, ok := raw.(*schema.Set)
	if !ok {
		return trace.Errorf("can not convert %T to *schema.Set", raw)
	}

	switch target.Kind() {
	case reflect.Slice:
		// We do not have sets mapped to slices for now. It might be needed for unordered collections which
		// change its order on every API request. Set is unordered collection.
		//
		// It will require adding explicit configuration flag "represent_collection_as_set".
		return trace.NotImplemented("set acting as list on target is not implemented yet")
	case reflect.Map:
		// This set must be read into a map, so, it contains artificial key and value arguments
		targetMap := reflect.MakeMap(target.Type())

		for _, i := range set.List() {
			itemMap, ok := i.(map[string]interface{})
			if !ok {
				return trace.Errorf("can not convert %T to map[string]interface{}", itemMap)
			}

			resource, ok := sch.Elem.(*schema.Resource)
			if !ok {
				return fmt.Errorf("can not convert %T to *schema.Resource", sch.Elem)
			}

			itemPath := fmt.Sprintf("%v.%v.value.0", path, set.F(i))
			itemKey, ok := itemMap["key"]
			if !ok {
				return fmt.Errorf("one of the element keys is empty in %s", path)
			}

			value := newEmptyValue(target.Type().Elem())

			err := getEnumerableElement(itemPath, value, resource.Schema["value"], meta, data)
			if err != nil {
				return trace.Wrap(err)
			}

			assignMapIndex(targetMap, reflect.ValueOf(itemKey), value)
		}

		target.Set(targetMap)

		return nil
	default:
		return trace.Errorf("unknown set target type %v", target.Kind())
	}
}

// newEmptyValue constructs new empty value for a given type. Type might be a pointer.
func newEmptyValue(source reflect.Type) reflect.Value {
	t := source

	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	return reflect.Indirect(reflect.New(t))
}
