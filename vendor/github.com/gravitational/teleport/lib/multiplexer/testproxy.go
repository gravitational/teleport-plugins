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

package multiplexer

import (
	"io"
	"net"

	"github.com/gravitational/teleport/lib/utils"

	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
)

// TestProxy is tcp passthrough proxy that sends a proxy-line when connecting
// to the target server.
type TestProxy struct {
	listener net.Listener
	target   string
	closeCh  chan (struct{})
	log      logrus.FieldLogger
}

// NewTestProxy creates a new test proxy that sends a proxy-line when
// proxying connections to the provided target address.
func NewTestProxy(target string) (*TestProxy, error) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &TestProxy{
		listener: listener,
		target:   target,
		closeCh:  make(chan struct{}),
		log:      logrus.WithField(trace.Component, "test:proxy"),
	}, nil
}

// Address returns the proxy listen address.
func (p *TestProxy) Address() string {
	return p.listener.Addr().String()
}

// Serve starts accepting client connections and proxying them to the target.
func (p *TestProxy) Serve() error {
	for {
		clientConn, err := p.listener.Accept()
		if err != nil {
			return trace.Wrap(err)
		}
		go func() {
			if err := p.handleConnection(clientConn); err != nil {
				p.log.WithError(err).Error("Failed to handle connection.")
			}
		}()
	}
}

// handleConnection dials the target address, sends a proxy line to it and
// then starts proxying all traffic b/w client and target.
func (p *TestProxy) handleConnection(clientConn net.Conn) error {
	serverConn, err := net.Dial("tcp", p.target)
	if err != nil {
		clientConn.Close()
		return trace.Wrap(err)
	}
	defer serverConn.Close()
	errCh := make(chan error, 2)
	go func() { // Client -> server.
		defer clientConn.Close()
		defer serverConn.Close()
		// Write proxy-line first and then start proxying from client.
		err := p.sendProxyLine(clientConn, serverConn)
		if err == nil {
			_, err = io.Copy(serverConn, clientConn)
		}
		errCh <- trace.Wrap(err)
	}()
	go func() { // Server -> client.
		defer clientConn.Close()
		defer serverConn.Close()
		_, err := io.Copy(clientConn, serverConn)
		errCh <- trace.Wrap(err)
	}()
	var errs []error
	for i := 0; i < 2; i++ {
		select {
		case err := <-errCh:
			if err != nil && !utils.IsOKNetworkError(err) {
				errs = append(errs, err)
			}
		case <-p.closeCh:
			p.log.Debug("Closing.")
			return trace.NewAggregate(errs...)
		}
	}
	return trace.NewAggregate(errs...)
}

// sendProxyLine sends proxy-line to the server.
func (p *TestProxy) sendProxyLine(clientConn, serverConn net.Conn) error {
	clientAddr, err := utils.ParseAddr(clientConn.RemoteAddr().String())
	if err != nil {
		return trace.Wrap(err)
	}
	serverAddr, err := utils.ParseAddr(serverConn.RemoteAddr().String())
	if err != nil {
		return trace.Wrap(err)
	}
	proxyLine := &ProxyLine{
		Protocol:    TCP4,
		Source:      net.TCPAddr{IP: net.ParseIP(clientAddr.Host()), Port: clientAddr.Port(0)},
		Destination: net.TCPAddr{IP: net.ParseIP(serverAddr.Host()), Port: serverAddr.Port(0)},
	}
	_, err = serverConn.Write([]byte(proxyLine.String()))
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// Close closes the proxy listener.
func (p *TestProxy) Close() error {
	close(p.closeCh)
	return p.listener.Close()
}
