package tcp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	lgbl "github.com/ipfs/go-libp2p-loggables"
	logging "github.com/ipfs/go-log"
	reuseport "github.com/jbenet/go-reuseport"
	tpt "github.com/libp2p/go-libp2p-transport"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
	mafmt "github.com/whyrusleeping/mafmt"
)

var log = logging.Logger("tcp-tpt")

type TcpTransport struct {
	dlock   sync.Mutex
	dialers map[string]tpt.Dialer

	llock     sync.Mutex
	listeners map[string]tpt.Listener
}

// NewTCPTransport creates a tcp transport object that tracks dialers and listeners
// created. It represents an entire tcp stack (though it might not necessarily be)
func NewTCPTransport() *TcpTransport {
	return &TcpTransport{
		dialers:   make(map[string]tpt.Dialer),
		listeners: make(map[string]tpt.Listener),
	}
}

func (t *TcpTransport) Dialer(laddr ma.Multiaddr, opts ...tpt.DialOpt) (tpt.Dialer, error) {
	t.dlock.Lock()
	defer t.dlock.Unlock()
	s := laddr.String()
	d, found := t.dialers[s]
	if found {
		return d, nil
	}
	var base manet.Dialer

	var doReuse bool
	for _, o := range opts {
		switch o := o.(type) {
		case tpt.TimeoutOpt:
			base.Timeout = time.Duration(o)
		case tpt.ReuseportOpt:
			doReuse = bool(o)
		default:
			return nil, fmt.Errorf("unrecognized option: %#v", o)
		}
	}

	tcpd, err := t.newTcpDialer(base, laddr, doReuse)
	if err != nil {
		return nil, err
	}

	t.dialers[s] = tcpd
	return tcpd, nil
}

func (t *TcpTransport) Listen(laddr ma.Multiaddr) (tpt.Listener, error) {
	if !t.Matches(laddr) {
		return nil, fmt.Errorf("tcp transport cannot listen on %q", laddr)
	}

	t.llock.Lock()
	defer t.llock.Unlock()
	s := laddr.String()
	l, found := t.listeners[s]
	if found {
		return l, nil
	}

	list, err := manetListen(laddr)
	if err != nil {
		return nil, err
	}

	tlist := &tcpListener{
		list:      list,
		transport: t,
	}

	t.listeners[s] = tlist
	return tlist, nil
}

func manetListen(addr ma.Multiaddr) (manet.Listener, error) {
	network, naddr, err := manet.DialArgs(addr)
	if err != nil {
		return nil, err
	}

	if ReuseportIsAvailable() {
		nl, err := reuseport.Listen(network, naddr)
		if err == nil {
			// hey, it worked!
			return manet.WrapNetListener(nl)
		}
		// reuseport is available, but we failed to listen. log debug, and retry normally.
		log.Debugf("reuseport available, but failed to listen: %s %s, %s", network, naddr, err)
	}

	// either reuseport not available, or it failed. try normally.
	return manet.Listen(addr)
}

func (t *TcpTransport) Matches(a ma.Multiaddr) bool {
	return mafmt.TCP.Matches(a)
}

type tcpDialer struct {
	laddr ma.Multiaddr

	doReuse bool

	rd       reuseport.Dialer
	madialer manet.Dialer

	transport tpt.Transport
}

func (t *TcpTransport) newTcpDialer(base manet.Dialer, laddr ma.Multiaddr, doReuse bool) (*tcpDialer, error) {
	// get the local net.Addr manually
	la, err := manet.ToNetAddr(laddr)
	if err != nil {
		return nil, err // something wrong with laddr.
	}

	if doReuse && ReuseportIsAvailable() {
		rd := reuseport.Dialer{
			D: net.Dialer{
				LocalAddr: la,
				Timeout:   base.Timeout,
			},
		}

		return &tcpDialer{
			doReuse:   true,
			laddr:     laddr,
			rd:        rd,
			madialer:  base,
			transport: t,
		}, nil
	}

	return &tcpDialer{
		doReuse:   false,
		laddr:     laddr,
		madialer:  base,
		transport: t,
	}, nil
}

func (d *tcpDialer) Dial(raddr ma.Multiaddr) (tpt.Conn, error) {
	return d.DialContext(context.Background(), raddr)
}

func (d *tcpDialer) DialContext(ctx context.Context, raddr ma.Multiaddr) (tpt.Conn, error) {
	var c manet.Conn
	var err error
	if d.doReuse {
		c, err = d.reuseDial(ctx, raddr)
	} else {
		c, err = d.madialer.DialContext(ctx, raddr)
	}

	if err != nil {
		return nil, err
	}

	return &tpt.ConnWrap{
		Conn: c,
		Tpt:  d.transport,
	}, nil
}

func (d *tcpDialer) reuseDial(ctx context.Context, raddr ma.Multiaddr) (manet.Conn, error) {
	logdial := lgbl.Dial("conn", "", "", d.laddr, raddr)
	rpev := log.EventBegin(ctx, "tptDialReusePort", logdial)

	network, netraddr, err := manet.DialArgs(raddr)
	if err != nil {
		return nil, err
	}

	_ = ctx // TODO: implement DialContext in reuseport
	con, err := d.rd.Dial(network, netraddr)
	if err == nil {
		logdial["reuseport"] = "success"
		rpev.Done()
		return manet.WrapNetConn(con)
	}

	if !ReuseErrShouldRetry(err) {
		logdial["reuseport"] = "failure"
		logdial["error"] = err
		rpev.Done()
		return nil, err
	}

	logdial["reuseport"] = "retry"
	logdial["error"] = err
	rpev.Done()

	return d.madialer.DialContext(ctx, raddr)
}

func (d *tcpDialer) Matches(a ma.Multiaddr) bool {
	return mafmt.TCP.Matches(a)
}

type tcpListener struct {
	list      manet.Listener
	transport tpt.Transport
}

func (d *tcpListener) Accept() (tpt.Conn, error) {
	c, err := d.list.Accept()
	if err != nil {
		return nil, err
	}

	return &tpt.ConnWrap{
		Conn: c,
		Tpt:  d.transport,
	}, nil
}

func (d *tcpListener) Addr() net.Addr {
	return d.list.Addr()
}

func (t *tcpListener) Multiaddr() ma.Multiaddr {
	return t.list.Multiaddr()
}

func (t *tcpListener) NetListener() net.Listener {
	return t.list.NetListener()
}

func (d *tcpListener) Close() error {
	return d.list.Close()
}
