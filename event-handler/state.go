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
	"encoding/binary"
	"time"

	"github.com/gravitational/trace"
	"github.com/peterbourgon/diskv"
	log "github.com/sirupsen/logrus"
)

const (
	// cacheSizeMaxBytes max memory cache
	cacheSizeMaxBytes = 1024

	// startTimeName is the start time variable name
	startTimeName = "start_time"

	// cursorName is the cursor variable name
	cursorName = "cursor"

	// idName is the id variable name
	idName = "id"

	// sessionPrefix is the session key prefix
	sessionPrefix = "session"
)

// State is the state repository
type State struct {
	// dv is a diskv instance
	dv *diskv.Diskv
}

// NewCursor creates new cursor instance
func NewState(c *StorageConfig, ic *IngestConfig) (*State, error) {
	// Simplest transform function: put all the data files into the base dir.
	flatTransform := func(s string) []string { return []string{} }

	dv := diskv.New(diskv.Options{
		BasePath:     c.StorageDir,
		Transform:    flatTransform,
		CacheSizeMax: cacheSizeMaxBytes,
	})

	s := State{dv}

	err := s.resetOnStartTimeChanged(c, ic)
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
func (s *State) resetOnStartTimeChanged(c *StorageConfig, ic *IngestConfig) error {
	prevStartTime, err := s.GetStartTime()
	if err != nil {
		return trace.Wrap(err)
	}

	if prevStartTime == nil {
		log.WithField("value", ic.StartTime).Debug("Setting start time")

		err := s.dv.EraseAll()
		if err != nil {
			return trace.Wrap(err)
		}

		if ic.StartTime == nil {
			t := time.Now().UTC().Truncate(time.Second)
			return s.SetStartTime(&t)
		}

		return s.SetStartTime(ic.StartTime)
	}

	// If there is a time saved in the state and this time does not equal to the time passed from CLI and a
	// time was explicitly passed from CLI
	if prevStartTime != nil && ic.StartTime != nil && *prevStartTime != *ic.StartTime {
		return trace.Errorf("You can not change start time in the middle of ingestion. To restart the ingestion, rm -rf %v", c.StorageDir)
	}

	return nil
}

// GetStartTime gets current start time
func (s *State) GetStartTime() (*time.Time, error) {
	if !s.dv.Has(startTimeName) {
		return nil, nil
	}

	b, err := s.dv.Read(startTimeName)
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
		return s.dv.Write(startTimeName, []byte(""))
	}

	v := t.Truncate(time.Second).Format(time.RFC3339)
	return s.dv.Write(startTimeName, []byte(v))
}

// GetCursor sets current cursor value
func (s *State) GetCursor() (string, error) {
	return s.getStringValue(cursorName)
}

// SetCursor sets cursor value
func (s *State) SetCursor(v string) error {
	return s.setStringValue(cursorName, v)
}

// GetID sets current ID value
func (s *State) GetID() (string, error) {
	return s.getStringValue(idName)
}

// SetID sets cursor value
func (s *State) SetID(v string) error {
	return s.setStringValue(idName, v)
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

// GetSessions get active sessions (map[id]index)
func (s *State) GetSessions() (map[string]int64, error) {
	r := make(map[string]int64)

	for key := range s.dv.KeysPrefix(sessionPrefix, nil) {
		b, err := s.dv.Read(key)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		id := key[len(sessionPrefix):]
		r[id] = int64(binary.BigEndian.Uint64(b))
	}

	return r, nil
}

// SetSessionIndex writes current session index into state
func (s *State) SetSessionIndex(id string, index int64) error {
	var b []byte = make([]byte, 8)

	binary.BigEndian.PutUint64(b, uint64(index))

	return s.dv.Write(sessionPrefix+id, b)
}

// RemoveSession removes session from the state
func (s *State) RemoveSession(id string) error {
	return s.dv.Erase(sessionPrefix + id)
}
