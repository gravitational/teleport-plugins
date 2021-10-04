/*
Copyright 2021 Gravitational, Inc.

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

package plugindata

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gravitational/teleport-plugins/lib/backoff"
	apiclient "github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
)

// Client is a wrapper for API Client working in a context of a Plugin.
type Client struct {
	APIClient  *apiclient.Client
	PluginName string
}

// Get loads a plugin data for a given resource.
func (client Client) Get(ctx context.Context, kind, name string, data Unmarshaller) error {
	dataMaps, err := client.APIClient.GetPluginData(ctx, types.PluginDataFilter{
		Kind:     kind,
		Resource: name,
		Plugin:   client.PluginName,
	})
	if err != nil {
		return trace.Wrap(err)
	}
	if len(dataMaps) == 0 {
		return trace.NotFound("plugin data not found")
	}
	entry := dataMaps[0].Entries()[client.PluginName]
	if entry == nil {
		return trace.NotFound("plugin data entry not found")
	}
	data.UnmarshalPluginData(entry.Data)
	return nil
}

// Update changes an existing plugin data or sets a new one if it didn't exist.
func (client Client) Update(ctx context.Context, kind, name string, data, expectData Marshaller) error {
	if err := client.APIClient.UpdatePluginData(ctx, types.PluginDataUpdateParams{
		Kind:     kind,
		Resource: name,
		Plugin:   client.PluginName,
		Set:      data.MarshalPluginData(),
		Expect:   expectData.MarshalPluginData(),
	}); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// Modify performs a compare-and-swap update of resources's plugin data.
//
// The value parameter indicates a plugin data type. It must be a pointer to some other type that supports
// both marshaling and unmarshaling. Typically, you can just pass (*YourPluginData)(nil).
//
// The callback function parameter is nil if plugin data hasn't been created yet.
// Otherwise, the callback function parameter is a pointer to current plugin data contents.
//
// The callback function return value is an updated plugin data contents plus the boolean flag
// indicating whether it should be written or not.
// Note that callback function fn might be called more than once due to the retry mechanism being used,
// so make sure that the function is "pure", i.e. it doesn't interact with the outside world.
// It shouldn't perform any I/O, and even things like Go channels must be avoided.
// Indeed, this limitation is not that ultimate, at least if you know what you're doing.
//
// If the callback function returned true, but the server responded with a CompareFailed error,
// the operation is being retried with a backoff mechanism.
func (client Client) Modify(ctx context.Context, backoff backoff.Backoff, kind, name string, value MarshallerUnmarshaller, fn func(interface{}) (Marshaller, bool)) (bool, error) {
	valueType := reflect.TypeOf(value)
	if valueType.Kind() != reflect.Ptr {
		panic(fmt.Sprintf("value type %s must be a pointer", valueType))
	}
	for {
		dataVal := reflect.New(valueType.Elem())
		data := dataVal.Interface().(Unmarshaller)
		err := client.Get(ctx, kind, name, data)
		notFound := trace.IsNotFound(err)
		if err != nil && !notFound {
			return false, trace.Wrap(err)
		}

		var oldDataVal reflect.Value
		if notFound {
			oldDataVal = reflect.Zero(valueType) // nil pointer
		} else {
			oldDataVal = dataVal
		}
		oldData := oldDataVal.Interface().(Marshaller)
		newData, ok := fn(oldData)
		if !ok {
			return false, nil
		}

		err = client.Update(ctx, kind, name, newData, oldData)
		if err == nil {
			return true, nil
		}
		if !trace.IsCompareFailed(err) {
			return false, trace.Wrap(err)
		}
		if err := backoff.Do(ctx); err != nil {
			return false, trace.Wrap(err)
		}
	}
}
