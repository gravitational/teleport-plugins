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
	"strconv"
	"strings"
	"time"
)

type StringMap = map[string]string

type Marshaller interface {
	MarshalPluginData() StringMap
}
type Unmarshaller interface {
	UnmarshalPluginData(StringMap)
}

type MarshallerUnmarshaller interface {
	Marshaller
	Unmarshaller
}

// SplitString is the same as strings.Split but it makes sure that the
// result is nil if the input string is empty.
func SplitString(s, sep string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, sep)
}

func DecodeInt(str string) int {
	if str == "" {
		return 0
	}
	i, _ := strconv.Atoi(str)
	return i
}

func EncodeInt(i int) string {
	if i == 0 {
		return ""
	}
	return strconv.Itoa(i)
}

func DecodeUint64(str string) uint64 {
	i, _ := strconv.ParseUint(str, 10, 0)
	return i
}

func EncodeUint64(i uint64) string {
	if i == 0 {
		return ""
	}
	return strconv.FormatUint(i, 10)
}

func DecodeInt64(str string) int64 {
	if str == "" {
		return 0
	}
	i, _ := strconv.ParseInt(str, 10, 0)
	return i
}

func EncodeInt64(i int64) string {
	if i == 0 {
		return ""
	}
	return strconv.FormatInt(i, 10)
}

func EncodeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return EncodeInt64(t.Unix())
}

func DecodeTime(str string) time.Time {
	if str == "" {
		return time.Time{}
	}
	return time.Unix(DecodeInt64(str), 0)
}
