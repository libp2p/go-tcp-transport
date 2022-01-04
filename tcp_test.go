package tcp

import (
	"testing"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/sec"
	"github.com/libp2p/go-libp2p-core/sec/insecure"
	"github.com/libp2p/go-libp2p-core/transport"

	csms "github.com/libp2p/go-conn-security-multistream"
	mplex "github.com/libp2p/go-libp2p-mplex"
	ttransport "github.com/libp2p/go-libp2p-testing/suites/transport"
	tptu "github.com/libp2p/go-libp2p-transport-upgrader"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/stretchr/testify/require"
)

func TestTcpTransport(t *testing.T) {
	for i := 0; i < 2; i++ {
		peerA, ia := makeInsecureMuxer(t)
		_, ib := makeInsecureMuxer(t)

		ua, err := tptu.New(ia, new(mplex.Transport))
		require.NoError(t, err)
		ta, err := NewTCPTransport(ua)
		require.NoError(t, err)
		ub, err := tptu.New(ib, new(mplex.Transport))
		require.NoError(t, err)
		tb, err := NewTCPTransport(ub)
		require.NoError(t, err)

		zero := "/ip4/127.0.0.1/tcp/0"
		ttransport.SubtestTransport(t, ta, tb, zero, peerA)

		envReuseportVal = false
	}
	envReuseportVal = true
}

func TestTcpTransportCantDialDNS(t *testing.T) {
	for i := 0; i < 2; i++ {
		dnsa, err := ma.NewMultiaddr("/dns4/example.com/tcp/1234")
		require.NoError(t, err)

		var u transport.Upgrader
		tpt, err := NewTCPTransport(u)
		require.NoError(t, err)

		if tpt.CanDial(dnsa) {
			t.Fatal("shouldn't be able to dial dns")
		}

		envReuseportVal = false
	}
	envReuseportVal = true
}

func TestTcpTransportCantListenUtp(t *testing.T) {
	for i := 0; i < 2; i++ {
		utpa, err := ma.NewMultiaddr("/ip4/127.0.0.1/udp/0/utp")
		require.NoError(t, err)

		var u transport.Upgrader
		tpt, err := NewTCPTransport(u)
		require.NoError(t, err)

		_, err = tpt.Listen(utpa)
		require.Error(t, err, "shouldn't be able to listen on utp addr with tcp transport")

		envReuseportVal = false
	}
	envReuseportVal = true
}

func makeInsecureMuxer(t *testing.T) (peer.ID, sec.SecureMuxer) {
	t.Helper()
	priv, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	require.NoError(t, err)
	id, err := peer.IDFromPrivateKey(priv)
	require.NoError(t, err)
	var secMuxer csms.SSMuxer
	secMuxer.AddTransport(insecure.ID, insecure.NewWithIdentity(id, priv))
	return id, &secMuxer
}
