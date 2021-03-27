package tcp

import (
	"errors"
	"net"
	"syscall"
	"unsafe"

	"github.com/mikioh/tcp"
)

// This is only needed because mikioh/tcp doesn't accept wrapped connections.
// See https://github.com/mikioh/tcp/pull/2.

type tcpConn struct {
	net.Conn
	c syscall.RawConn
}

// newTCPConn returns a new end point.
func newTCPConn(c net.Conn) (*tcp.Conn, error) {
	type tcpConnI interface {
		SyscallConn() (syscall.RawConn, error)
		SetLinger(int) error
	}
	var _ tcpConnI = &net.TCPConn{}
	cc := &tcpConn{Conn: c}
	switch c := c.(type) {
	case tcpConnI:
		var err error
		cc.c, err = c.SyscallConn()
		if err != nil {
			return nil, err
		}
		return (*tcp.Conn)(unsafe.Pointer(cc)), nil
	default:
		return nil, errors.New("unknown connection type")
	}
}
