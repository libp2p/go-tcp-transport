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
)

func TestTcpTransportWithSecureMuxer(t *testing.T) {
	for i := 0; i < 2; i++ {
		peerA, ia := makeInsecureMuxer(t)
		_, ib := makeInsecureMuxer(t)

		ta := NewTCPTransport(&tptu.Upgrader{
			SecureMuxer: ia,
			Muxer:       new(mplex.Transport),
		})
		tb := NewTCPTransport(&tptu.Upgrader{
			SecureMuxer: ib,
			Muxer:       new(mplex.Transport),
		})

		zero := "/ip4/127.0.0.1/tcp/0"
		ttransport.SubtestTransport(t, ta, tb, zero, peerA)

		envReuseportVal = false
	}
	envReuseportVal = true
}

func TestTcpTransportWithSecureTransport(t *testing.T) {
	for i := 0; i < 2; i++ {
		peerA, ia := makeInsecureTransport(t)
		_, ib := makeInsecureTransport(t)

		ta := NewTCPTransport(&tptu.Upgrader{
			SecureTransport: ia,
			Muxer:           new(mplex.Transport),
		})
		tb := NewTCPTransport(&tptu.Upgrader{
			SecureTransport: ib,
			Muxer:           new(mplex.Transport),
		})

		zero := "/ip4/127.0.0.1/tcp/0/plaintextv2"
		ttransport.SubtestTransport(t, ta, tb, zero, peerA)

		envReuseportVal = false
	}
	envReuseportVal = true
}

func TestTcpTransportCantDialDNS(t *testing.T) {
	for i := 0; i < 2; i++ {
		dnsa, err := ma.NewMultiaddr("/dns4/example.com/tcp/1234")
		if err != nil {
			t.Fatal(err)
		}

		_, sm := makeInsecureMuxer(t)
		tpt := NewTCPTransport(&tptu.Upgrader{
			SecureMuxer: sm,
			Muxer:       new(mplex.Transport),
		})

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
		if err != nil {
			t.Fatal(err)
		}

		_, sm := makeInsecureMuxer(t)
		tpt := NewTCPTransport(&tptu.Upgrader{
			SecureMuxer: sm,
			Muxer:       new(mplex.Transport),
		})
		if _, err := tpt.Listen(utpa); err == nil {
			t.Fatal("shouldnt be able to listen on utp addr with tcp transport")
		}

		envReuseportVal = false
	}
	envReuseportVal = true
}

func makeInsecureMuxer(t *testing.T) (peer.ID, sec.SecureMuxer) {
	t.Helper()
	id, tr := makeInsecureTransport(t)
	var secMuxer csms.SSMuxer
	secMuxer.AddTransport(insecure.ID, tr)
	return id, &secMuxer
}

func makeInsecureTransport(t *testing.T) (peer.ID, sec.SecureTransport) {
	t.Helper()
	priv, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatal(err)
	}
	id, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return id, insecure.NewWithIdentity(id, priv)
}
