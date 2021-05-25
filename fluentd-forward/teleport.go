package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/lib/events"
	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
)

// TODO: TeleportEventsIterator
// TeleportClient represents wrapper around Teleport client to work with events
type TeleportClient struct {
	// client is an instance of GRPC Teleport client
	client *client.Client

	// current cursor value
	cursor string
}

// NewTeleportClient builds Teleport client instance
func NewTeleportClient(c *Config) (*TeleportClient, error) {
	if c.TeleportIdentityFile != "" {
		t, err := newUsingIdentityFile(c)
		if err != nil {
			return nil, err
		}
		return t, nil
	}

	return newUsingKeys(c)
}

// newUsingIdentityFile tries to build API client using identity file
func newUsingIdentityFile(c *Config) (*TeleportClient, error) {
	identity := client.LoadIdentityFile(c.TeleportIdentityFile)

	config := client.Config{
		Addrs:       []string{c.TeleportAddr},
		Credentials: []client.Credentials{identity},
	}

	client, err := client.New(context.Background(), config)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &TeleportClient{client: client}, nil
}

// newUsingKeys tries to build API client using keys
func newUsingKeys(c *Config) (*TeleportClient, error) {
	config := client.Config{
		Addrs: []string{c.TeleportAddr},
		Credentials: []client.Credentials{
			client.LoadKeyPair(c.TeleportCert, c.TeleportKey, c.TeleportCA),
		},
	}

	client, err := client.New(context.Background(), config)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &TeleportClient{client: client}, nil
}

// Close closes connection to Teleport
func (t *TeleportClient) Close() {
	t.client.Close()
}

// Fetches next batch of events starting from a last cursor position
func (t *TeleportClient) Fetch() error {

}

// Next returns next event
func (t *TeleportClient) Next() (*events.AuditEvent, error) {

}

func (t *TeleportClient) Test() {
	e, cursor, err := t.client.SearchEvents(context.Background(), time.Now().AddDate(-5, 0, 0), time.Now().UTC(), "default", []string{}, 5, "")
	if err != nil {
		log.Fatalf("%v", err)
	}

	for _, v := range e {
		s, _ := json.Marshal(v)
		logrus.Printf("%+v", string(s))
	}

	log.Printf("----")

	e, cursor, err = t.client.SearchEvents(context.Background(), time.Now().AddDate(-5, 0, 0), time.Now().UTC(), "default", []string{}, 5, cursor)
	if err != nil {
		log.Fatalf("%v", err)
	}

	for _, v := range e {
		s, _ := json.Marshal(v)
		logrus.Printf("%+v", string(s))
	}
}
