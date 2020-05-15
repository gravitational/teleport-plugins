package utils

import (
	"net"
	"strconv"

	"github.com/gravitational/trace"
)

// PortList is a list of TCP ports
type PortList []string

// Pop returns a value from the list, it panics if the value is not there
func (p *PortList) Pop() string {
	if len(*p) == 0 {
		panic("list is empty")
	}
	val := (*p)[len(*p)-1]
	*p = (*p)[:len(*p)-1]
	return val
}

// PopInt returns a value from the list, it panics if not enough values
// were allocated
func (p *PortList) PopInt() int {
	i, err := strconv.Atoi(p.Pop())
	if err != nil {
		panic(err)
	}
	return i
}

// PopIntSlice returns a slice of values from the list, it panics if not enough
// ports were allocated
func (p *PortList) PopIntSlice(num int) []int {
	ports := make([]int, num)
	for i := range ports {
		ports[i] = p.PopInt()
	}
	return ports
}

// GetFreeTCPPortsForTests returns n free ports (which are suggested by the kernel)
func GetFreeTCPPortsForTests(n int) (PortList, error) {
	list := make(PortList, 0, n)
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
