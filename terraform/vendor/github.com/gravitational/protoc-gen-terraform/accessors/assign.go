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

	"github.com/gravitational/trace"
)

// assign assigns source value to target with type and pointer conversions
func assign(source, target reflect.Value) error {
	t := target.Type()
	v := source

	// If target type is at the pointer reference use underlying type
	if target.Type().Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Convert value to target type
	if source.Type() != t {
		if !v.Type().ConvertibleTo(target.Type()) {
			return trace.Errorf("can not convert %v to %v", source.Type().Name(), t.Name())
		}

		// v := source.(string)
		v = v.Convert(t)
	}

	if !v.Type().AssignableTo(t) {
		return trace.Errorf("can not assign %s to %s", v.Type().Name(), t.Name())
	}

	// If target type is a reference, create new pointer to this reference and assign
	if target.Type().Kind() == reflect.Ptr {
		if v.CanAddr() {
			// target := &source
			target.Set(v.Addr())
			return nil
		}

		// a := "5"
		// target := &a
		ptr := reflect.New(v.Type())
		ptr.Elem().Set(v)
		target.Set(ptr)
		return nil
	}

	target.Set(v)
	return nil
}

// AssignZeroValue sets target to zero value. Target must not be pointer.
func AssignZeroValue(target reflect.Value) {
	target.Set(reflect.Zero(target.Type()))
}

// assignMapIndex assigns map element by value or reference
func assignMapIndex(m, key, value reflect.Value) {
	if m.Type().Elem().Kind() == reflect.Ptr {
		m.SetMapIndex(key, value.Addr())
	} else {
		m.SetMapIndex(key, value)
	}
}
