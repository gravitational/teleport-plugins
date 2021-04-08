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

package mysql

import (
	"crypto/tls"
	"io"
	"net"
	"strings"
	"sync/atomic"

	"github.com/gravitational/teleport/lib/srv/db/common"

	"github.com/siddontang/go-mysql/client"
	"github.com/siddontang/go-mysql/mysql"
	"github.com/siddontang/go-mysql/server"

	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
)

// MakeTestClient returns MySQL client connection according to the provided
// parameters.
func MakeTestClient(config common.TestClientConfig) (*client.Conn, error) {
	tlsConfig, err := common.MakeTestClientTLSConfig(config)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	conn, err := client.Connect(config.Address,
		config.RouteToDatabase.Username,
		"",
		config.RouteToDatabase.Database,
		func(conn *client.Conn) {
			conn.SetTLSConfig(tlsConfig)
		})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return conn, nil
}

// TestServer is a test MySQL server used in functional database
// access tests.
type TestServer struct {
	listener  net.Listener
	port      string
	tlsConfig *tls.Config
	log       logrus.FieldLogger
	handler   *testHandler
}

// NewTestServer returns a new instance of a test MySQL server.
func NewTestServer(config common.TestServerConfig) (*TestServer, error) {
	address := "localhost:0"
	if config.Address != "" {
		address = config.Address
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return nil, trace.Wrap(err)
	}
	tlsConfig, err := common.MakeTestServerTLSConfig(config)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	log := logrus.WithFields(logrus.Fields{
		trace.Component: "mysql",
		"name":          config.Name,
	})
	return &TestServer{
		listener:  listener,
		port:      port,
		tlsConfig: tlsConfig,
		log:       log,
		handler:   &testHandler{log: log},
	}, nil
}

// Serve starts serving client connections.
func (s *TestServer) Serve() error {
	s.log.Debugf("Starting test MySQL server on %v.", s.listener.Addr())
	defer s.log.Debug("Test MySQL server stopped.")
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if err == io.EOF || strings.Contains(err.Error(), "use of closed network connection") {
				return nil
			}
			s.log.WithError(err).Error("Failed to accept connection.")
			continue
		}
		s.log.Debug("Accepted connection.")
		go func() {
			defer s.log.Debug("Connection done.")
			defer conn.Close()
			err = s.handleConnection(conn)
			if err != nil {
				s.log.Errorf("Failed to handle connection: %v.",
					trace.DebugReport(err))
			}
		}()
	}
}

func (s *TestServer) handleConnection(conn net.Conn) error {
	serverConn, err := server.NewCustomizedConn(
		conn,
		server.NewServer(
			serverVersion,
			mysql.DEFAULT_COLLATION_ID,
			mysql.AUTH_NATIVE_PASSWORD,
			nil,
			s.tlsConfig),
		&credentialProvider{},
		s.handler)
	if err != nil {
		return trace.Wrap(err)
	}
	for {
		if serverConn.Closed() {
			return nil
		}
		err = serverConn.HandleCommand()
		if err != nil {
			return trace.Wrap(err)
		}
	}
}

// Port returns the port server is listening on.
func (s *TestServer) Port() string {
	return s.port
}

// QueryCount returns the number of queries the server has received.
func (s *TestServer) QueryCount() uint32 {
	return atomic.LoadUint32(&s.handler.queryCount)
}

// Close closes the server listener.
func (s *TestServer) Close() error {
	return s.listener.Close()
}

type testHandler struct {
	server.EmptyHandler
	log logrus.FieldLogger
	// queryCount keeps track of the number of queries the server has received.
	queryCount uint32
}

func (h *testHandler) HandleQuery(query string) (*mysql.Result, error) {
	h.log.Debugf("Received query %q.", query)
	atomic.AddUint32(&h.queryCount, 1)
	return TestQueryResponse, nil
}

// TestQueryResponse is what test MySQL server returns to every query.
var TestQueryResponse = &mysql.Result{
	InsertId:     1,
	AffectedRows: 0,
}
