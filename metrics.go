package tcp

import (
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-tcp-transport/metrics"
	"github.com/marten-seemann/tcp"
	"github.com/mikioh/tcpinfo"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	newConns      *prometheus.CounterVec
	closedConns   *prometheus.CounterVec
	segsSentDesc  *prometheus.Desc
	segsRcvdDesc  *prometheus.Desc
	bytesSentDesc *prometheus.Desc
	bytesRcvdDesc *prometheus.Desc
)

var collector *aggregatingCollector

func init() {
	segsSentDesc = prometheus.NewDesc("tcp_sent_segments_total", "TCP segments sent", nil, nil)
	segsRcvdDesc = prometheus.NewDesc("tcp_rcvd_segments_total", "TCP segments received", nil, nil)
	bytesSentDesc = prometheus.NewDesc("tcp_sent_bytes", "TCP bytes sent", nil, nil)
	bytesRcvdDesc = prometheus.NewDesc("tcp_rcvd_bytes", "TCP bytes received", nil, nil)

	collector = newAggregatingCollector()
	prometheus.MustRegister(collector)

	const direction = "direction"

	newConns = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tcp_connections_new_total",
			Help: "TCP new connections",
		},
		[]string{direction},
	)
	prometheus.MustRegister(newConns)
	closedConns = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tcp_connections_closed_total",
			Help: "TCP connections closed",
		},
		[]string{direction},
	)
	prometheus.MustRegister(closedConns)
}

type aggregatingCollector struct {
	mutex sync.Mutex

	highestID     uint64
	conns         map[uint64] /* id */ *tracingConn
	rtts          prometheus.Histogram
	connDurations prometheus.Histogram
}

var _ prometheus.Collector = &aggregatingCollector{}

func newAggregatingCollector() *aggregatingCollector {
	return &aggregatingCollector{
		conns: make(map[uint64]*tracingConn),
		rtts: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "tcp_rtt",
			Help:    "TCP round trip time",
			Buckets: prometheus.ExponentialBuckets(0.001, 1.25, 40), // 1ms to ~6000ms
		}),
		connDurations: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "tcp_connection_duration",
			Help:    "TCP Connection Duration",
			Buckets: prometheus.ExponentialBuckets(1, 1.5, 40), // 1s to ~12 weeks
		}),
	}
}

func (c *aggregatingCollector) AddConn(t *tracingConn) uint64 {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.highestID++
	c.conns[c.highestID] = t
	return c.highestID
}

func (c *aggregatingCollector) removeConn(id uint64) {
	delete(c.conns, id)
}

func (c *aggregatingCollector) Describe(descs chan<- *prometheus.Desc) {
	descs <- c.rtts.Desc()
	descs <- c.connDurations.Desc()
	if hasSegmentCounter {
		descs <- segsSentDesc
		descs <- segsRcvdDesc
	}
	if hasByteCounter {
		descs <- bytesSentDesc
		descs <- bytesRcvdDesc
	}
}

func (c *aggregatingCollector) Collect(metrics chan<- prometheus.Metric) {
	now := time.Now()
	c.mutex.Lock()
	var segsSent, segsRcvd uint64
	var bytesSent, bytesRcvd uint64
	for _, conn := range c.conns {
		info, err := conn.getTCPInfo()
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				c.closedConn(conn)
				continue
			}
			log.Errorf("Failed to get TCP info: %s", err)
			continue
		}
		if hasSegmentCounter {
			segsSent += getSegmentsSent(info)
			segsRcvd += getSegmentsRcvd(info)
		}
		if hasByteCounter {
			bytesSent += getBytesSent(info)
			bytesRcvd += getBytesRcvd(info)
		}
		c.rtts.Observe(info.RTT.Seconds())
		c.connDurations.Observe(now.Sub(conn.startTime).Seconds())
		if info.State == tcpinfo.Closed {
			c.closedConn(conn)
		}
	}
	c.mutex.Unlock()
	metrics <- c.rtts
	metrics <- c.connDurations
	if hasSegmentCounter {
		segsSentMetric, err := prometheus.NewConstMetric(segsSentDesc, prometheus.CounterValue, float64(segsSent))
		if err != nil {
			log.Errorf("creating tcp_sent_segments_total metric failed: %v", err)
			return
		}
		segsRcvdMetric, err := prometheus.NewConstMetric(segsRcvdDesc, prometheus.CounterValue, float64(segsRcvd))
		if err != nil {
			log.Errorf("creating tcp_rcvd_segments_total metric failed: %v", err)
			return
		}
		metrics <- segsSentMetric
		metrics <- segsRcvdMetric
	}
	if hasByteCounter {
		bytesSentMetric, err := prometheus.NewConstMetric(bytesSentDesc, prometheus.CounterValue, float64(bytesSent))
		if err != nil {
			log.Errorf("creating tcp_sent_bytes metric failed: %v", err)
			return
		}
		bytesRcvdMetric, err := prometheus.NewConstMetric(bytesRcvdDesc, prometheus.CounterValue, float64(bytesRcvd))
		if err != nil {
			log.Errorf("creating tcp_rcvd_bytes metric failed: %v", err)
			return
		}
		metrics <- bytesSentMetric
		metrics <- bytesRcvdMetric
	}
}

func (c *aggregatingCollector) closedConn(conn *tracingConn) {
	collector.removeConn(conn.id)
	closedConns.WithLabelValues(conn.getDirection()).Inc()
}

type tracingConn struct {
	id uint64

	startTime time.Time
	isClient  bool

	manet.Conn
	tcpConn  *tcp.Conn
	lastInfo *tcpinfo.Info
}

func newTracingConn(c manet.Conn, isClient bool) (*tracingConn, error) {
	conn, err := tcp.NewConn(c)
	if err != nil {
		return nil, err
	}
	tc := &tracingConn{
		startTime: time.Now(),
		isClient:  isClient,
		Conn:      c,
		tcpConn:   conn,
	}
	tc.id = collector.AddConn(tc)
	newConns.WithLabelValues(tc.getDirection()).Inc()
	return tc, nil
}

func (c *tracingConn) getDirection() string {
	if c.isClient {
		return "outgoing"
	}
	return "incoming"
}

func (c *tracingConn) Close() error {
	if err := c.saveTCPInfo(); err != nil {
		log.Errorf("failed to save TCP info: %v", err)
	}
	return c.Conn.Close()
}

func (c *tracingConn) saveTCPInfo() error {
	if c.lastInfo == nil {
		return nil
	}
	return (&metrics.ConnectionStats{
		IsClient:   c.isClient,
		StartTime:  c.startTime,
		EndTime:    time.Now(),
		LocalAddr:  c.LocalAddr(),
		RemoteAddr: c.RemoteAddr(),
		LastRTT: metrics.RTTMeasurement{
			SmoothedRTT: c.lastInfo.RTT,
			RTTVar:      c.lastInfo.RTTVar,
		},
	}).Save()
}

func (c *tracingConn) getTCPInfo() (*tcpinfo.Info, error) {
	var o tcpinfo.Info
	var b [256]byte
	i, err := c.tcpConn.Option(o.Level(), o.Name(), b[:])
	if err != nil {
		return nil, err
	}
	info := i.(*tcpinfo.Info)
	c.lastInfo = info
	return info, nil
}

type tracingListener struct {
	manet.Listener
}

func (l *tracingListener) Accept() (manet.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return newTracingConn(conn, false)
}
