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

package lib

import (
	"github.com/gravitational/trace"
	jsoniter "github.com/json-iterator/go"
)

// configFastestRelaxed represents jsoniter config which encodes all message fields, even those which are marked as jsontag:"-"
var configFastestRelaxed = jsoniter.Config{
	EscapeHTML:                    false,
	MarshalFloatWith6Digits:       true,
	ObjectFieldMustBeSimpleString: true,
	TagKey:                        "-", // This forces jsoniter to serialize all event fields, use with caution
}.Froze()

// FastMarshal serializes given interface to json
func FastMarshal(v interface{}, relaxed bool) ([]byte, error) {
	c := jsoniter.ConfigFastest
	if relaxed {
		c = configFastestRelaxed
	}

	data, err := c.Marshal(v)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return data, nil
}
