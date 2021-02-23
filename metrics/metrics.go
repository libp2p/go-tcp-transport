package metrics

import (
	"context"
	"log"
	"net"
	"sync"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/lucas-clemente/quic-go/logging"
)

const (
	bigQueryDataset = "connections"
	bigQueryTable   = "tcp"
)

const timeout = 5 * time.Second

var nodeBootTime time.Time

var (
	bigqueryInitOnce sync.Once
	bigqueryClient   *bigquery.Client
)

func init() {
	nodeBootTime = time.Now()
	// Check validity of the bigquery schema.
	if _, err := bigquery.InferSchema(&connectionStats{}); err != nil {
		log.Fatal(err)
	}
}

type rttMeasurement struct {
	SmoothedRTT float64 `bigquery:"smoothed_rtt"`
	RTTVar      float64 `bigquery:"rtt_var"`
}

type RTTMeasurement struct {
	SmoothedRTT, RTTVar time.Duration
}

func toMilliSecond(d time.Duration) float64 { return float64(d.Nanoseconds()) / 1e6 }

func (m *RTTMeasurement) toBigQuery() rttMeasurement {
	return rttMeasurement{
		SmoothedRTT: toMilliSecond(m.SmoothedRTT),
		RTTVar:      toMilliSecond(m.RTTVar),
	}
}

type connectionStats struct {
	NodeID       string         `bigquery:"node"`
	NodeBootTime time.Time      `bigquery:"node_boot_time"`
	IsClient     bool           `bigquery:"is_client"`
	StartTime    time.Time      `bigquery:"start_time"`
	EndTime      time.Time      `bigquery:"end_time"`
	LocalAddr    string         `bigquery:"local_addr"`
	RemoteAddr   string         `bigquery:"remote_addr"`
	LastRTT      rttMeasurement `bigquery:"last_rtt"`
}

type ConnectionStats struct {
	Node                  peer.ID
	IsClient              bool
	Perspective           logging.Perspective
	StartTime, EndTime    time.Time
	LocalAddr, RemoteAddr net.Addr
	LastRTT               RTTMeasurement
}

func (s *ConnectionStats) toBigQuery() *connectionStats {
	return &connectionStats{
		NodeID:       s.Node.Pretty(),
		NodeBootTime: nodeBootTime,
		IsClient:     s.IsClient,
		StartTime:    s.StartTime,
		EndTime:      s.EndTime,
		LocalAddr:    s.LocalAddr.String(),
		RemoteAddr:   s.RemoteAddr.String(),
		LastRTT:      s.LastRTT.toBigQuery(),
	}
}

func (s *ConnectionStats) Save() error {
	var initErr error
	bigqueryInitOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		bigqueryClient, initErr = bigquery.NewClient(ctx, "transport-performance")
	})
	if initErr != nil || bigqueryClient == nil {
		return initErr
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ins := bigqueryClient.Dataset(bigQueryDataset).Table(bigQueryTable).Inserter()
	return ins.Put(ctx, s.toBigQuery())
}
