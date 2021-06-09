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

package sshutils

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/gravitational/teleport/api/constants"

	"github.com/gravitational/trace"
	"golang.org/x/crypto/ssh"
)

// NewChConn returns a new net.Conn implemented over
// SSH channel
func NewChConn(conn ssh.Conn, ch ssh.Channel) *ChConn {
	return &ChConn{
		Channel: ch,
		conn:    conn,
	}
}

// NewExclusiveChConn returns a new net.Conn implemented over
// SSH channel, whenever this connection closes
func NewExclusiveChConn(conn ssh.Conn, ch ssh.Channel) *ChConn {
	return &ChConn{
		Channel:   ch,
		conn:      conn,
		exclusive: true,
	}
}

// ChConn is a net.Conn like object
// that uses SSH channel
type ChConn struct {
	mu sync.Mutex

	ssh.Channel
	conn ssh.Conn
	// exclusive indicates that whenever this channel connection
	// is getting closed, the underlying connection is closed as well
	exclusive bool
}

// Close closes channel and if the ChConn is exclusive, connection as well
func (c *ChConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	err := c.Channel.Close()
	if !c.exclusive {
		return trace.Wrap(err)
	}
	err2 := c.conn.Close()
	return trace.NewAggregate(err, err2)
}

// LocalAddr returns a local address of a connection
// Uses underlying net.Conn implementation
func (c *ChConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// RemoteAddr returns a remote address of a connection
// Uses underlying net.Conn implementation
func (c *ChConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// SetDeadline sets a connection deadline
// ignored for the channel connection
func (c *ChConn) SetDeadline(t time.Time) error {
	return nil
}

// SetReadDeadline sets a connection read deadline
// ignored for the channel connection
func (c *ChConn) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline sets write deadline on a connection
// ignored for the channel connection
func (c *ChConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// CancelableChConn is a wrapped SSH channel connection that supports deadlines.
type CancelableChConn struct {
	// ChConn is the base wrapped SSH channel connection.
	*ChConn
	// reader is the part of the pipe that clients read from.
	reader net.Conn
	// writer is the part of the pipe that receives data from SSH channel.
	writer net.Conn
}

// NewCancelableChConn returns a new instance of wrapped SSH channel connection
// that supports deadlines.
func NewCancelableChConn(conn ssh.Conn, ch ssh.Channel) *CancelableChConn {
	reader, writer := net.Pipe()
	c := &CancelableChConn{
		ChConn: NewChConn(conn, ch),
		reader: reader,
		writer: writer,
	}
	// Start copying from the SSH channel to the writer part of the pipe. The
	// clients are reading from the reader part of the pipe (see Read below).
	//
	// This goroutine stops when either the SSH channel closes or this
	// connection is closed e.g. by a http.Server (see Close below).
	go io.Copy(writer, ch)
	return c
}

// Read reads from the channel.
func (c *CancelableChConn) Read(data []byte) (int, error) {
	n, err := c.reader.Read(data)
	// A lot of code relies on "use of closed network connection" error to
	// gracefully handle terminated connections so convert the closed pipe
	// error to it.
	if err != nil && err == io.ErrClosedPipe {
		return n, trace.ConnectionProblem(err, constants.UseOfClosedNetworkConnection)
	}
	// Do not wrap the error to avoid masking the underlying error such as
	// timeout error which is returned when read deadline is exceeded.
	return n, err
}

// SetDeadline sets the channel connection read/write deadlines.
func (c *CancelableChConn) SetDeadline(t time.Time) error {
	return c.reader.SetDeadline(t)
}

// SetReadDeadline sets the channel connection read deadline.
func (c *CancelableChConn) SetReadDeadline(t time.Time) error {
	return c.reader.SetReadDeadline(t)
}

// Closes closes all parts of the connection.
func (c *CancelableChConn) Close() error {
	var errors []error
	if err := c.ChConn.Close(); err != nil {
		errors = append(errors, err)
	}
	if err := c.reader.Close(); err != nil {
		errors = append(errors, err)
	}
	if err := c.writer.Close(); err != nil {
		errors = append(errors, err)
	}
	return trace.NewAggregate(errors...)
}

const (
	// ConnectionTypeRequest is a request sent over a SSH channel that returns a
	// boolean which indicates the connection type (direct or tunnel).
	ConnectionTypeRequest = "x-teleport-connection-type"
)
