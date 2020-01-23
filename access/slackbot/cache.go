/*
Copyright 2019 Gravitational, Inc.

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
	"context"
	"sync"
	"time"

	"github.com/gravitational/teleport-plugins/access"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

type Entry struct {
	Request   access.Request
	ChannelID string
	Timestamp string
	exp       uint
}

// RequestCache holds pending request data.
type RequestCache struct {
	sync.Mutex
	entries map[string]Entry
	index   uint
	next    uint
	err     error
}

func NewRequestCache(ctx context.Context) *RequestCache {
	cache := &RequestCache{
		entries: make(map[string]Entry),
	}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				cache.tick()
			case <-ctx.Done():
				cache.taint(ctx.Err())
				return
			}
		}
	}()
	return cache
}

func (c *RequestCache) Put(entry Entry) error {
	const TTL = 60 * 60
	c.Lock()
	defer c.Unlock()
	if c.err != nil {
		return trace.Wrap(c.err)
	}
	entry.exp = c.index + TTL
	if c.next == 0 || c.next > entry.exp {
		c.next = entry.exp
	}
	c.entries[entry.Request.ID] = entry
	return nil
}

func (c *RequestCache) Pop(reqID string) (Entry, error) {
	c.Lock()
	defer c.Unlock()
	if c.err != nil {
		return Entry{}, trace.Wrap(c.err)
	}
	if e, ok := c.entries[reqID]; ok {
		delete(c.entries, reqID)
		return e, nil
	} else {
		return Entry{}, trace.NotFound("no request matching %q", reqID)
	}
}

func (c *RequestCache) Drop(reqID string) error {
	c.Lock()
	defer c.Unlock()

	if c.err != nil {
		return trace.Wrap(c.err)
	}

	delete(c.entries, reqID)

	return nil
}

func (c *RequestCache) tick() int {
	c.Lock()
	defer c.Unlock()
	c.index++
	if c.index < c.next {
		return len(c.entries)
	}
	for reqID, entry := range c.entries {
		if entry.exp < c.index {
			log.Debugf("removing expired cache entry %s...", reqID)
			delete(c.entries, reqID)
			continue
		}
		if entry.exp < c.next {
			c.next = entry.exp
		}
	}
	return len(c.entries)
}

func (c *RequestCache) taint(err error) {
	c.Lock()
	defer c.Unlock()
	c.entries = nil
	c.err = err
}
