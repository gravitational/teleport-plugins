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
	"net"
	"strings"
	"time"

	"github.com/gravitational/trace"
	"github.com/peterbourgon/diskv"
	log "github.com/sirupsen/logrus"
)

const (
	// cacheSizeMaxBytes max memory cache
	cacheSizeMaxBytes = 1024
)

// State is the state repository
type State struct {
	// dv is a diskv instance
	dv *diskv.Diskv

	// prefix is the state variable prefix
	prefix string

	// startTimeName is the start time variable name
	startTimeName string

	// cursorName is the cursor variable name
	cursorName string

	// idName is the id variable name
	idName string
}

// NewCursor creates new cursor instance
func NewState(c *StartCmd) (*State, error) {
	// Simplest transform function: put all the data files into the base dir.
	flatTransform := func(s string) []string { return []string{} }

	dv := diskv.New(diskv.Options{
		BasePath:     c.StorageDir,
		Transform:    flatTransform,
		CacheSizeMax: cacheSizeMaxBytes,
	})

	host, port, err := net.SplitHostPort(c.TeleportAddr)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	prefix := strings.TrimSpace(host + "_" + port)
	if prefix == "_" {
		return nil, trace.Errorf("Can not generate cursor name from Teleport host %s", c.TeleportAddr)
	}

	log.WithField("prefix", prefix).Info("Using state prefix")

	s := State{dv, prefix, prefix + "_start_time", prefix + "_cursor", prefix + "_id"}

	err = s.resetOnStartTimeChanged(c)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	t, err := s.GetStartTime()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	log.WithField("value", t).Info("Using start time")

	return &s, nil
}

// resetOnStartTimeChanged resets state if start time explicitly changed from the previous run
func (s *State) resetOnStartTimeChanged(c *StartCmd) error {
	prevStartTime, err := s.GetStartTime()
	if err != nil {
		return trace.Wrap(err)
	}

	if prevStartTime == nil {
		log.WithField("value", c.StartTime).Debug("Setting start time")

		err := s.dv.EraseAll()
		if err != nil {
			return trace.Wrap(err)
		}

		if c.StartTime == nil {
			t := time.Now().UTC().Truncate(time.Second)
			return s.SetStartTime(&t)
		}

		return s.SetStartTime(c.StartTime)
	}

	// If there is a time saved in the state and this time does not equal to the time passed from CLI and a
	// time was explicitly passed from CLI
	if prevStartTime != nil && c.StartTime != nil && *prevStartTime != *c.StartTime {
		return trace.Errorf("You can not change start time in the middle of ingestion. To restart the ingestion, rm -rf %v", c.StorageDir)
	}

	return nil
}

// GetStartTime gets current start time
func (s *State) GetStartTime() (*time.Time, error) {
	if !s.dv.Has(s.startTimeName) {
		return nil, nil
	}

	b, err := s.dv.Read(s.startTimeName)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// No previous start time exist
	if string(b) == "" {
		return nil, nil
	}

	t, err := time.Parse(time.RFC3339, string(b))
	if err != nil {
		return nil, trace.Wrap(err)
	}

	t = t.Truncate(time.Second)

	return &t, nil
}

// SetStartTime sets current start time
func (s *State) SetStartTime(t *time.Time) error {
	if t == nil {
		return s.dv.Write(s.startTimeName, []byte(""))
	}

	v := t.Truncate(time.Second).Format(time.RFC3339)
	return s.dv.Write(s.startTimeName, []byte(v))
}

// GetCursor sets current cursor value
func (s *State) GetCursor() (string, error) {
	return s.getStringValue(s.cursorName)
}

// SetCursor sets cursor value
func (s *State) SetCursor(v string) error {
	return s.setStringValue(s.cursorName, v)
}

// GetID sets current ID value
func (s *State) GetID() (string, error) {
	return s.getStringValue(s.idName)
}

// SetID sets cursor value
func (s *State) SetID(v string) error {
	return s.setStringValue(s.idName, v)
}

// getStringValue gets a string value
func (s *State) getStringValue(name string) (string, error) {
	if !s.dv.Has(name) {
		return "", nil
	}

	b, err := s.dv.Read(name)
	if err != nil {
		return "", trace.Wrap(err)
	}

	return string(b), err
}

// setStringValue sets string value
func (s *State) setStringValue(name string, value string) error {
	err := s.dv.Write(name, []byte(value))
	return trace.Wrap(err)
}
