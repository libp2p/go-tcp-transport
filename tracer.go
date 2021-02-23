package tcp

import (
	"errors"
	"net"
	"syscall"
	"time"
	"unsafe"

	"github.com/libp2p/go-tcp-transport/metrics"
	"github.com/mikioh/tcp"
	"github.com/mikioh/tcpinfo"
	manet "github.com/multiformats/go-multiaddr/net"
)

type tcpConn struct {
	net.Conn
	c syscall.RawConn
}

// newTCPConn returns a new end point.
func newTCPConn(c net.Conn) (*tcpConn, error) {
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
		return cc, nil
	default:
		return nil, errors.New("unknown connection type")
	}
}

type tracingConn struct {
	closed    bool
	startTime time.Time
	isClient  bool

	manet.Conn
}

func newTracingConn(c manet.Conn, isClient bool) *tracingConn {
	return &tracingConn{
		startTime: time.Now(),
		isClient:  isClient,
		Conn:      c,
	}
}

func (c *tracingConn) Close() error {
	if !c.closed {
		if err := c.close(); err != nil {
			log.Errorf("Saving logs failed: %v", err)
		} else {
			log.Infof("Saving logs succeeded.")
		}
		c.closed = true
	}
	return c.Conn.Close()
}

func (c *tracingConn) close() error {
	conn, err := newTCPConn(c.Conn)
	if err != nil {
		return err
	}
	tconn := (*tcp.Conn)(unsafe.Pointer(conn))
	return c.saveTCPInfo(tconn)
}

func (c *tracingConn) saveTCPInfo(tc *tcp.Conn) error {
	endTime := time.Now()
	var o tcpinfo.Info
	var b [256]byte
	i, err := tc.Option(o.Level(), o.Name(), b[:])
	if err != nil {
		return err
	}
	info := i.(*tcpinfo.Info)
	return (&metrics.ConnectionStats{
		IsClient:   c.isClient,
		StartTime:  c.startTime,
		EndTime:    endTime,
		LocalAddr:  c.LocalAddr(),
		RemoteAddr: c.RemoteAddr(),
		LastRTT: metrics.RTTMeasurement{
			SmoothedRTT: info.RTT,
			RTTVar:      info.RTTVar,
		},
	}).Save()
}

type tracingListener struct {
	manet.Listener
}

func (l *tracingListener) Accept() (manet.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return newTracingConn(conn, false), nil
}
