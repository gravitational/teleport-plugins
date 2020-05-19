// Package nettest provides a modified version of the GetFreeTCPPorts function
// from Teleport's utils package.
package nettest

import (
	"net"
	"strconv"

	utils "github.com/gravitational/teleport/lib/utils"
	"github.com/gravitational/trace"
)

// GetFreeTCPPortsForTests returns n free ports (which are suggested by the kernel)
func GetFreeTCPPortsForTests(n int) (utils.PortList, error) {
	list := make(utils.PortList, 0, n)
	for i := 0; i < n; i++ {
		addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, trace.Wrap(err)
		}

		listen, err := net.ListenTCP("tcp", addr)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		defer listen.Close()

		port := strconv.Itoa(listen.Addr().(*net.TCPAddr).Port)
		list = append(list, port)
	}
	return list, nil
}
