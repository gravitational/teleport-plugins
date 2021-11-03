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
	"strconv"

	"github.com/gravitational/trace"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// GetListLen returns the length of a list
func GetListLen(path string, data *schema.ResourceData) (int, error) {
	return getLen(path, data, "#")
}

// GetMapLen returns the length of a map
func GetMapLen(path string, data *schema.ResourceData) (int, error) {
	return getLen(path, data, "%")
}

// getLen returns List, Set or Map value length
func getLen(path string, data *schema.ResourceData, suffix string) (int, error) {
	num, ok := data.GetOk(path + "." + suffix) // Terraform stores collection length in "collection_name.#" key
	if !ok || num == nil {
		return 0, nil
	}

	var len int
	var err error

	if reflect.TypeOf(num) == stringType {
		s, ok := num.(string)
		if !ok {
			return 0, trace.Errorf("failed to convert list count to string %s", path)
		}

		len, err = strconv.Atoi(s)
		if err != nil {
			return 0, trace.Errorf("failed to parse len %s", path)
		}
	} else {
		len, ok = num.(int)
		if !ok {
			return 0, trace.Errorf("failed to convert list count to number %s", path)
		}
	}

	return len, nil
}
