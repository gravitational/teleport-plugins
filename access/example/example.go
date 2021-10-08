/*
Copyright 2019-2021 Gravitational, Inc.

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

// Example whitelist based access plugin.
//
// This plugin approves/denies access requests based on a simple whitelist
// of usernames.  Requests from whitelisted users are approved, all others
// are denied.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"

	"github.com/gravitational/trace"
)

// eprintln prints an optionally formatted string to stderr.
func eprintln(msg string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, msg, a...)
	fmt.Fprintf(os.Stderr, "\n")
}

func main() {
	pgrm := os.Args[0]
	args := os.Args[1:]
	if len(args) < 1 {
		eprintln("USAGE: %s (configure | <config-path>)", pgrm)
		os.Exit(1)
	}
	if args[0] == "configure" {
		fmt.Print(exampleConfig)
		return
	}
	if err := run(args[0]); err != nil {
		eprintln("ERROR: %s", err)
		os.Exit(1)
	}
}

func run(configPath string) error {
	conf, err := LoadConfig(configPath)
	if err != nil {
		return trace.Wrap(err)
	}
	ctx := context.TODO()
	// Establish new client connection to the Teleport auth server.
	client, err := client.New(ctx, client.Config{
		Addrs:       conf.GetAddrs(),
		Credentials: conf.Credentials(),
	})
	if err != nil {
		return trace.Wrap(err)
	}
	// Register a watcher for pending access requests.
	filter := types.AccessRequestFilter{State: types.RequestState_PENDING}
	watcher, err := client.NewWatcher(ctx, types.Watch{
		Kinds: []types.WatchKind{
			{
				Kind:   types.KindAccessRequest,
				Filter: filter.IntoMap(),
			},
		},
	})
	if err != nil {
		return trace.Wrap(err)
	}
	select {
	case event := <-watcher.Events():
		if event.Type != types.OpInit {
			return trace.Errorf("expected to get OpInit, but got %s", event.Type)
		}
	case <-ctx.Done():
		return trace.Wrap(ctx.Err())
	}
	eprintln("watcher initialized...")
	defer watcher.Close()
	for {
		select {
		case event := <-watcher.Events():
			op := event.Type
			switch op {
			case types.OpPut:
				request := event.Resource.(types.AccessRequest)
				// OpPut indicates that a request has been created or updated.  Since we specified
				// StatePending in our filter, only pending requests should appear here.
				eprintln("Handling request: %+v", request)
				whitelisted := false
			CheckWhitelist:
				for _, user := range conf.Whitelist {
					if request.GetUser() == user {
						whitelisted = true
						break CheckWhitelist
					}
				}
				params := types.AccessRequestUpdate{
					Annotations: map[string][]string{
						"strategy": []string{"whitelist"},
					},
				}
				if whitelisted {
					eprintln("User %q in whitelist, approving request...", request.GetUser())
					params.State = types.RequestState_APPROVED
					params.Reason = "user in whitelist"
				} else {
					eprintln("User %q not in whitelist, denying request...", request.GetUser())
					params.State = types.RequestState_DENIED
					params.Reason = "user not in whitelist"
				}
				if err := client.SetAccessRequestState(ctx, params); err != nil {
					return trace.Wrap(err)
				}
				eprintln("ok.")
			case types.OpDelete:
				// request has been removed (expired).
				// Only the ID is non-zero in this case.
				// Due to some limitations in Teleport's event system, filters
				// don't really work with OpDelete events.  As such, we may get
				// OpDelete events for requests that would not typically match
				// the filter argument we supplied above.
				eprintln("Request %s has automatically expired.", event.Resource.GetName())
			default:
				return trace.BadParameter("unexpected event operation %s", op)
			}
		case <-watcher.Done():
			return watcher.Error()
		}
	}
}
