package tcp

import (
	"testing"

	csms "github.com/libp2p/go-conn-security-multistream"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/sec"
	"github.com/libp2p/go-libp2p-core/sec/insecure"
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

		ta, err := NewTCPTransport(&tptu.Upgrader{
			Secure: ia,
			Muxer:  new(mplex.Transport),
		})
		require.NoError(t, err)
		tb, err := NewTCPTransport(&tptu.Upgrader{
			Secure: ib,
			Muxer:  new(mplex.Transport),
		})
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

		_, sm := makeInsecureMuxer(t)
		tpt, err := NewTCPTransport(&tptu.Upgrader{
			Secure: sm,
			Muxer:  new(mplex.Transport),
		})
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

		_, sm := makeInsecureMuxer(t)
		tpt, err := NewTCPTransport(&tptu.Upgrader{
			Secure: sm,
			Muxer:  new(mplex.Transport),
		})
		require.NoError(t, err)

		_, err = tpt.Listen(utpa)
		require.Error(t, err, "shouldnt be able to listen on utp addr with tcp transport")

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
