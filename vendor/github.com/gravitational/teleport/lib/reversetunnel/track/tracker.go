/*
Copyright 2020 Gravitational, Inc.

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

package track

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/gravitational/teleport/lib/utils"
	"github.com/gravitational/teleport/lib/utils/workpool"
	"github.com/gravitational/trace"
)

type Lease = workpool.Lease

// Config configures basic Tracker parameters.
type Config struct {
	// ProxyExpiry is the duration an entry will be held sice the last
	// successful connection to, or message about, a given proxy.
	ProxyExpiry time.Duration
	// TickRate is the rate at which expired entries are cleared from
	// the cache of known proxies.
	TickRate time.Duration
	// ClusterName is the name of the tracked cluster.
	ClusterName string
}

// SetDefaults set default values for Config.
func (c *Config) CheckAndSetDefaults() error {
	if c.ProxyExpiry < 1 {
		c.ProxyExpiry = 3 * time.Minute
	}
	if c.TickRate < 1 {
		c.TickRate = 30 * time.Second
	}
	if c.ClusterName == "" {
		return trace.BadParameter("missing ClusterName in track.Config")
	}
	return nil
}

// Tracker is a helper for tracking proxies located behind reverse tunnels
// and triggering agent spawning as needed.  Tracker wraps a workpool.Pool
// instance and manages a cache of proxies which *may* exist.  As proxies are
// discovered, or old proxies expire, the target counts are automatically updated
// for the associated address in the workpool.  Agents can attempt to "claim"
// exclusivity for a given proxy, ensuring that multiple agents are not run
// against the same proxy.
type Tracker struct {
	Config
	mu     sync.Mutex
	wp     *workpool.Pool
	sets   map[utils.NetAddr]*proxySet
	cancel context.CancelFunc
}

// New configures a new tracker instance.
func New(ctx context.Context, c Config) (*Tracker, error) {
	if err := c.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	ctx, cancel := context.WithCancel(ctx)
	t := &Tracker{
		Config: c,
		wp:     workpool.NewPool(ctx),
		sets:   make(map[utils.NetAddr]*proxySet),
		cancel: cancel,
	}
	go t.run(ctx)
	return t, nil
}

func (t *Tracker) run(ctx context.Context) {
	ticker := time.NewTicker(t.TickRate)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			t.tick()
		case <-ctx.Done():
			return
		}
	}
}

// Acquire grants access to the Acquire channel of the
// embedded work group.
func (t *Tracker) Acquire() <-chan Lease {
	return t.wp.Acquire()
}

// TrackExpected starts/refreshes tracking for expected proxies.  Called by
// agents when gossip messages are received.
func (t *Tracker) TrackExpected(lease Lease, proxies ...string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	addr := lease.Key().(utils.NetAddr)
	set, ok := t.sets[addr]
	if !ok {
		return
	}
	now := time.Now()
	for _, name := range proxies {
		set.markSeen(now, name)
	}
	count := len(set.proxies)
	if count < 1 {
		count = 1
	}
	t.wp.Set(addr, uint64(count))
}

// Start starts tracking for specified proxy address.
func (t *Tracker) Start(addr utils.NetAddr) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.getOrCreate(addr)
}

// Stop stops tracking for specified proxy address.
func (t *Tracker) Stop(addr utils.NetAddr) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.sets[addr]; !ok {
		return
	}
	delete(t.sets, addr)
	t.wp.Set(addr, 0)
}

// StopAll permanently deactivates this tracker and cleans
// up all background goroutines.
func (t *Tracker) StopAll() {
	t.cancel()
}

func (t *Tracker) tick() {
	t.mu.Lock()
	defer t.mu.Unlock()
	cutoff := time.Now().Add(-1 * t.ProxyExpiry)
	for addr, set := range t.sets {
		if set.expire(cutoff) > 0 {
			count := len(set.proxies)
			if count < 1 {
				count = 1
			}
			t.wp.Set(addr, uint64(count))
		}
	}
}

func (t *Tracker) getOrCreate(addr utils.NetAddr) *proxySet {
	if s, ok := t.sets[addr]; ok {
		return s
	}
	set := newProxySet(addr, t.ClusterName)
	t.sets[addr] = set
	t.wp.Set(addr, 1)
	return set
}

// WithProxy runs the supplied closure if and only if
// no other work is currently being done with the proxy
// identified by principals.
func (t *Tracker) WithProxy(work func(), lease Lease, principals ...string) (didWork bool) {
	addr := lease.Key().(utils.NetAddr)
	if ok := t.claim(addr, principals...); !ok {
		return false
	}
	defer t.unclaim(addr, principals...)
	work()
	return true
}

func (t *Tracker) claim(addr utils.NetAddr, principals ...string) (ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	set, ok := t.sets[addr]
	if !ok {
		return false
	}
	return set.claim(principals...)
}

func (t *Tracker) unclaim(addr utils.NetAddr, principals ...string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	set, ok := t.sets[addr]
	if !ok {
		return
	}
	set.unclaim(principals...)
}

type entry struct {
	lastSeen time.Time
	claimed  bool
}

func newProxySet(addr utils.NetAddr, clusterName string) *proxySet {
	return &proxySet{
		addr:        addr,
		clusterName: clusterName,
		proxies:     make(map[string]entry),
	}
}

type proxySet struct {
	addr        utils.NetAddr
	clusterName string
	proxies     map[string]entry
}

func (p *proxySet) claim(principals ...string) (ok bool) {
	proxy := p.resolveName(principals)
	e, ok := p.proxies[proxy]
	if !ok {
		p.proxies[proxy] = entry{
			claimed: true,
		}
		return true
	}
	if e.claimed {
		return false
	}
	e.claimed = true
	p.proxies[proxy] = e
	return true
}

func (p *proxySet) unclaim(principals ...string) {
	proxy := p.resolveName(principals)
	p.proxies[proxy] = entry{
		lastSeen: time.Now(),
	}
}

func (p *proxySet) markSeen(t time.Time, proxy string) {
	e, ok := p.proxies[proxy]
	if !ok {
		p.proxies[proxy] = entry{
			lastSeen: t,
		}
		return
	}
	if e.lastSeen.After(t) {
		return
	}
	e.lastSeen = t
	p.proxies[proxy] = e
}

func (p *proxySet) expire(cutoff time.Time) (removed int) {
	for name, entry := range p.proxies {
		if entry.claimed {
			continue
		}
		if entry.lastSeen.Before(cutoff) {
			delete(p.proxies, name)
			removed++
		}
	}
	return
}

// resolveName tries to extract the UUID of the proxy as that's the
// only unique identifier in the list of principals.
func (p *proxySet) resolveName(principals []string) string {
	// check if we're already using one of these principals.
	for _, name := range principals {
		if _, ok := p.proxies[name]; ok {
			return name
		}
	}
	// default to using the first principal
	name := principals[0]
	// if we have a `.<cluster-name>` suffix, remove it.
	if strings.HasSuffix(name, p.clusterName) {
		t := strings.TrimSuffix(name, p.clusterName)
		if strings.HasSuffix(t, ".") {
			name = strings.TrimSuffix(t, ".")
		}
	}
	return name
}
