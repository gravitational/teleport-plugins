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
	"reflect"
	"time"

	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var (
	// reflect.Type of float64
	floatType = reflect.TypeOf((*float64)(nil)).Elem()

	// reflect.Type of int
	intType = reflect.TypeOf((*int)(nil)).Elem()

	// reflect.Type of bool
	boolType = reflect.TypeOf((*bool)(nil)).Elem()

	// reflect.Type of string
	stringType = reflect.TypeOf((*string)(nil)).Elem()
)

// Set assigns object data from object to schema.ResourceData
//
// Example:
//   user := UserV2{Name: "example"}
//   Set(&user, data, SchemaUserV2, MetaUserV2)
func Set(
	obj interface{},
	data *schema.ResourceData,
	sch map[string]*schema.Schema,
	meta map[string]*SchemaMeta,
) error {
	if obj == nil {
		return trace.Errorf("obj must not be nil")
	}

	root, err := setFragment(reflect.Indirect(reflect.ValueOf(obj)), meta, sch)
	if err != nil {
		return trace.Wrap(err)
	}

	for k, v := range root {
		if v != nil {
			err := data.Set(k, v)
			if err != nil {
				return trace.Wrap(err)
			}
		}
	}

	return nil
}

// setFragment returns map[string]interface{} of a block
func setFragment(
	source reflect.Value,
	meta map[string]*SchemaMeta,
	sch map[string]*schema.Schema,
) (map[string]interface{}, error) {
	target := make(map[string]interface{})

	if !source.IsValid() {
		return nil, nil
	}

	for key, fieldMeta := range meta {
		fieldSchema, ok := sch[key]
		if !ok {
			return nil, trace.Errorf("field %v not found in corresponding schema", key)
		}

		fieldValue := source.FieldByName(fieldMeta.Name)

		if !fieldValue.IsValid() {
			return nil, trace.Errorf("field %v not found in source struct", key)
		}

		if fieldMeta.Setter != nil {
			r, err := fieldMeta.Setter(fieldValue, fieldMeta, fieldSchema)
			if err != nil {
				return nil, trace.Wrap(err)
			}

			if r != nil {
				target[key] = r
			}

			continue
		}

		switch fieldSchema.Type {
		case schema.TypeInt, schema.TypeFloat, schema.TypeBool, schema.TypeString:
			result, err := setElementary(fieldValue, fieldMeta, fieldSchema)
			if err != nil {
				return nil, trace.Wrap(err)
			}

			if result != nil {
				err = setConvertedKey(target, key, result, fieldSchema)
				if err != nil {
					return nil, trace.Wrap(err)
				}
			}

		case schema.TypeList:
			result, err := setList(fieldValue, fieldMeta, fieldSchema)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			if result != nil {
				target[key] = result
			}

		case schema.TypeMap:
			result, err := setMap(fieldValue, fieldMeta, fieldSchema)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			target[key] = result

		case schema.TypeSet:
			result, err := setSet(fieldValue, fieldMeta, fieldSchema)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			target[key] = result

		default:
			return nil, trace.Errorf("unknown type %v", fieldSchema.Type.String())
		}
	}

	return target, nil
}

// setElementary gets elementary value (scalar, string, time, duration)
func setElementary(source reflect.Value, meta *SchemaMeta, sch *schema.Schema) (interface{}, error) {
	if source.Kind() == reflect.Ptr && source.IsNil() {
		return nil, nil
	}

	switch {
	case meta.IsTime:
		t, err := setTime(source)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		return t, nil
	case meta.IsDuration:
		d, err := setDuration(source)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		return d, nil
	default:
		if source.IsZero() {
			return nil, nil
		}

		return source.Interface(), nil
	}
}

// setList converts source value to list
func setList(source reflect.Value, meta *SchemaMeta, sch *schema.Schema) (interface{}, error) {
	if source.Type().Kind() == reflect.Slice {
		if source.Len() == 0 {
			return nil, nil
		}

		slice := make([]interface{}, source.Len())

		for i := 0; i < source.Len(); i++ {
			value := source.Index(i)

			el, err := setEnumerableElement(value, meta, sch)
			if err != nil {
				return nil, err
			}

			slice[i] = el
		}

		return slice, nil
	}

	slice := make([]interface{}, 1)

	item, err := setEnumerableElement(reflect.Indirect(source), meta, sch)
	if err != nil {
		return nil, err
	}

	if item != nil {
		slice[0] = item
		return slice, nil
	}

	return nil, nil
}

// setMap converts source value to map
func setMap(source reflect.Value, meta *SchemaMeta, sch *schema.Schema) (interface{}, error) {
	if source.Len() == 0 {
		return nil, nil
	}

	targetMap := make(map[string]interface{})

	for _, key := range source.MapKeys() {
		i := source.MapIndex(key)

		value, err := setEnumerableElement(i, meta, sch)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		reflect.ValueOf(targetMap).SetMapIndex(key, reflect.ValueOf(value))
	}

	return targetMap, nil
}

// setSet converts source value to set
func setSet(source reflect.Value, meta *SchemaMeta, sch *schema.Schema) (interface{}, error) {
	if source.Len() == 0 {
		return nil, nil
	}

	set, ok := sch.ZeroValue().(*schema.Set)
	if !ok {
		return nil, trace.Errorf("zero value for schema set element is not *schema.Set")
	}

	switch source.Kind() {
	case reflect.Slice:
		// We do not have sets mapped to slices for now. It might be needed for unordered collections which
		// change its order on every API request. Set is unordered collection.
		//
		// It will require adding explicit configuration flag "represent_collection_as_set".
		return nil, trace.NotImplemented("set acting as list on target is not implemented yet")
	case reflect.Map:
		for _, key := range source.MapKeys() {
			i := source.MapIndex(key)

			valueSchema := sch.Elem.(*schema.Resource).Schema["value"]

			value, err := setEnumerableElement(i, meta, valueSchema)
			if err != nil {
				return nil, trace.Wrap(err)
			}

			t := map[string]interface{}{
				"key":   key.Interface(),
				"value": []interface{}{value},
			}

			set.Add(t)
		}

		return set, nil
	default:
		return nil, trace.Errorf("unknown set source type")
	}
}

// setTime returns value as time
func setTime(source reflect.Value) (interface{}, error) {
	t := reflect.Indirect(source).Interface()
	v, ok := t.(time.Time)
	if !ok {
		return nil, trace.Errorf("can not convert %T to time.Time", t)
	}

	return v.Format(time.RFC3339Nano), nil
}

// setDuration returns value as duration
func setDuration(source reflect.Value) (interface{}, error) {
	sourceValue := reflect.Indirect(source)
	durationType := reflect.TypeOf((*time.Duration)(nil)).Elem()

	if !sourceValue.Type().ConvertibleTo(durationType) {
		return nil, trace.Errorf("can not convert %T to time.Duration", sourceValue)
	}

	duration, ok := sourceValue.Convert(durationType).Interface().(time.Duration)
	if !ok {
		return nil, trace.Errorf("can not convert %T to time.Duration", sourceValue)
	}

	if duration != 0 {
		return duration.String(), nil
	}

	return nil, nil
}

// convert converts source value to schema type given in meta
func convert(source reflect.Value, sch *schema.Schema) (interface{}, error) {
	sourceValue := reflect.Indirect(source)
	schemaType, err := schemaValueType(sch)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if !sourceValue.Type().ConvertibleTo(schemaType) {
		return nil, trace.Errorf("can not convert %T to %T", sourceValue.Type(), schemaType)
	}

	return sourceValue.Convert(schemaType).Interface(), nil
}

// setConvertedKey converts value to target schema type and sets it to resulting map if not nil
func setConvertedKey(target map[string]interface{}, key string, source interface{}, sch *schema.Schema) error {
	if source != nil {
		value, err := convert(reflect.ValueOf(source), sch)
		if err != nil {
			return trace.Wrap(err)
		}
		target[key] = value
	}

	return nil
}

// setEnumerableElement gets singular slice element from a resource data and sets it to target. If enumerable element
// is empty, it assigns an empty value to the target.
func setEnumerableElement(
	source reflect.Value,
	meta *SchemaMeta,
	sch *schema.Schema,
) (interface{}, error) {
	switch s := sch.Elem.(type) {
	case *schema.Schema:
		value, err := setElementary(source, meta, s)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		target, err := convert(reflect.ValueOf(value), s)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		return target, nil

	case *schema.Resource:
		value, err := setFragment(reflect.Indirect(source), meta.Nested, s.Schema)
		if err != nil {
			return nil, err
		}

		if len(value) == 0 {
			return nil, nil
		}

		return value, nil
	default:
		return nil, trace.Errorf("unknown Elem type")
	}
}

// schemaValueType returns type to convert value to
func schemaValueType(sch *schema.Schema) (reflect.Type, error) {
	switch sch.Type {
	case schema.TypeFloat:
		return floatType, nil
	case schema.TypeInt:
		return intType, nil
	case schema.TypeBool:
		return boolType, nil
	case schema.TypeString:
		return stringType, nil
	default:
		return nil, trace.Errorf("unknown schema type: %v", sch.Type.String())
	}

}
