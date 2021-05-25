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

package main

import (
	"github.com/gravitational/trace"
	"github.com/peterbourgon/diskv"
)

const (
	// cacheSizeMax max memory cache
	cacheSizeMax = 1024 * 1024
)

type Cursor struct {
	// dv is a diskv instance
	dv *diskv.Diskv

	// name is the cursor name
	name string
}

// NewCursor creates new cursor instance
func NewCursor(c *Config) (*Cursor, error) {
	// Simplest transform function: put all the data files into the base dir.
	flatTransform := func(s string) []string { return []string{} }

	// Initialize a new diskv store, rooted at "my-data-dir", with a 1MB cache.
	dv := diskv.New(diskv.Options{
		BasePath:     c.StorageDir,
		Transform:    flatTransform,
		CacheSizeMax: cacheSizeMax,
	})

	return &Cursor{dv: dv, name: c.TeleportAddr}, nil
}

// Get gets current cursor value
func (c *Cursor) Get() (string, error) {
	value, err := c.dv.Read(c.name)
	if err != nil {
		return "", trace.Wrap(err)
	}

	return string(value), nil
}

// Set sets current cursor value
func (c *Cursor) Set(v string) error {
	return c.dv.Write(c.name, []byte(v))
}
