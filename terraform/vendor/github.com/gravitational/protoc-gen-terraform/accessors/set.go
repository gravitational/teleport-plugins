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

	for k, m := range meta {
		s, ok := sch[k]
		if !ok {
			return nil, trace.Errorf("field %v not found in corresponding schema", k)
		}

		v := source.FieldByName(m.Name)

		if m.Setter != nil {
			r, err := m.Setter(v, m, s)
			if err != nil {
				return nil, trace.Wrap(err)
			}

			if r != nil {
				target[k] = r
			}

			continue
		}

		switch s.Type {
		case schema.TypeInt, schema.TypeFloat, schema.TypeBool, schema.TypeString:
			r, err := setElementary(v, m, s)
			if err != nil {
				return nil, trace.Wrap(err)
			}

			if r != nil {
				err = setConvertedKey(target, k, r, s)
				if err != nil {
					return nil, trace.Wrap(err)
				}
			}

		case schema.TypeList:
			r, err := setList(v, m, s)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			if r != nil {
				target[k] = r
			}

		case schema.TypeMap:
			r, err := setMap(v, m, s)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			target[k] = r

		case schema.TypeSet:
			r, err := setSet(v, m, s)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			target[k] = r

		default:
			return nil, trace.Errorf("unknown type %v", s.Type.String())
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

		t := make([]interface{}, source.Len())

		for i := 0; i < source.Len(); i++ {
			v := source.Index(i)

			el, err := setEnumerableElement(v, meta, sch)
			if err != nil {
				return nil, err
			}

			t[i] = el
		}

		return t, nil
	}

	t := make([]interface{}, 1)

	item, err := setEnumerableElement(reflect.Indirect(source), meta, sch)
	if err != nil {
		return nil, err
	}

	if item != nil {
		t[0] = item
		return t, nil
	}

	return nil, nil
}

// setMap converts source value to map
func setMap(source reflect.Value, meta *SchemaMeta, sch *schema.Schema) (interface{}, error) {
	if source.Len() == 0 {
		return nil, nil
	}

	m := make(map[string]interface{})

	for _, k := range source.MapKeys() {
		i := source.MapIndex(k)

		v, err := setEnumerableElement(i, meta, sch)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		reflect.ValueOf(m).SetMapIndex(k, reflect.ValueOf(v))
	}

	return m, nil
}

// setSet converts source value to set
func setSet(source reflect.Value, meta *SchemaMeta, sch *schema.Schema) (interface{}, error) {
	if source.Len() == 0 {
		return nil, nil
	}

	s, ok := sch.ZeroValue().(*schema.Set)
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
		for _, k := range source.MapKeys() {
			i := source.MapIndex(k)

			vsch := sch.Elem.(*schema.Resource).Schema["value"]

			v, err := setEnumerableElement(i, meta, vsch)
			if err != nil {
				return nil, trace.Wrap(err)
			}

			t := map[string]interface{}{
				"key":   k.Interface(),
				"value": []interface{}{v},
			}

			s.Add(t)
		}

		return s, nil
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
	t := reflect.Indirect(source)
	d := reflect.TypeOf((*time.Duration)(nil)).Elem()

	if !t.Type().ConvertibleTo(d) {
		return nil, trace.Errorf("can not convert %T to time.Duration", t)
	}

	s, ok := t.Convert(d).Interface().(time.Duration)
	if !ok {
		return nil, trace.Errorf("can not convert %T to time.Duration", t)
	}

	if s != 0 {
		return s.String(), nil
	}

	return nil, nil
}

// convert converts source value to schema type given in meta
func convert(source reflect.Value, sch *schema.Schema) (interface{}, error) {
	t := reflect.Indirect(source)
	s, err := schemaValueType(sch)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if !t.Type().ConvertibleTo(s) {
		return nil, trace.Errorf("can not convert %T to %T", t.Type(), s)
	}

	return t.Convert(s).Interface(), nil
}

// setConvertedKey converts value to target schema type and sets it to resulting map if not nil
func setConvertedKey(target map[string]interface{}, key string, source interface{}, sch *schema.Schema) error {
	if source != nil {
		f, err := convert(reflect.ValueOf(source), sch)
		if err != nil {
			return trace.Wrap(err)
		}
		target[key] = f
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
		a, err := setElementary(source, meta, s)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		n, err := convert(reflect.ValueOf(a), s)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		return n, nil

	case *schema.Resource:
		m, err := setFragment(reflect.Indirect(source), meta.Nested, s.Schema)
		if err != nil {
			return nil, err
		}

		if len(m) == 0 {
			return nil, nil
		}

		return m, nil
	default:
		return nil, trace.Errorf("unknown Elem type")
	}
}

// schemaValueType returns type to convert value to
func schemaValueType(sch *schema.Schema) (reflect.Type, error) {
	switch sch.Type {
	case schema.TypeFloat:
		return reflect.TypeOf((*float64)(nil)).Elem(), nil
	case schema.TypeInt:
		return reflect.TypeOf((*int)(nil)).Elem(), nil
	case schema.TypeBool:
		return reflect.TypeOf((*bool)(nil)).Elem(), nil
	case schema.TypeString:
		return reflect.TypeOf((*string)(nil)).Elem(), nil
	default:
		return nil, trace.Errorf("unknown schema type: %v", sch.Type.String())
	}

}
