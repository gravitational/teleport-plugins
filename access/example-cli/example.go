package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gravitational/teleport-plugins/access"

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
	tc, err := conf.LoadTLSConfig()
	if err != nil {
		return trace.Wrap(err)
	}
	ctx := context.TODO()
	// Establish new client connection to the Teleport auth server.
	client, err := access.NewClient(ctx, conf.AuthServer, tc)
	if err != nil {
		return trace.Wrap(err)
	}
	// Register a watcher for pending access requests.
	watcher, err := client.WatchRequests(ctx, access.Filter{
		State: access.StatePending,
	})
	if err != nil {
		return trace.Wrap(err)
	}
	stdin := bufio.NewReader(os.Stdin)
	defer watcher.Close()
	for {
		select {
		case req := <-watcher.Requests():
			fmt.Printf("User %s wants to assume role(s): %s\n", req.User, strings.Join(req.Roles, ","))
			var state access.State
		Inner:
			for {
				fmt.Print("approve/deny> ")
				input, err := stdin.ReadString('\n')
				if err != nil {
					return trace.Wrap(err)
				}
				switch in := strings.TrimSpace(input); in {
				case "approve":
					state = access.StateApproved
					break Inner
				case "deny":
					state = access.StateDenied
					break Inner
				default:
					eprintln("ERROR: expected one of 'approve or 'deny', got '%s'", in)
				}
			}
			if err := client.SetRequestState(ctx, req.ID, state); err != nil {
				return trace.Wrap(err)
			}
			eprintln("Updated request %s to state %s\n", req.ID, state)
		case <-watcher.Done():
			return watcher.Error()
		}
	}
	return nil
}
