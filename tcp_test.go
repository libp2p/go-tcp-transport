package tcp

import (
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"testing"

	"github.com/libp2p/go-libp2p-core/sec/insecure"
	mplex "github.com/libp2p/go-libp2p-mplex"
	tptu "github.com/libp2p/go-libp2p-transport-upgrader"

	ttransport "github.com/libp2p/go-libp2p-testing/suites/transport"

	ma "github.com/multiformats/go-multiaddr"
)

func TestTcpTransport(t *testing.T) {
	for i := 0; i < 2; i++ {
		ia := makeInsecureTransport(t)
		ib := makeInsecureTransport(t)

		ta := NewTCPTransport(&tptu.Upgrader{
			Secure: ia,
			Muxer:  new(mplex.Transport),
		})
		tb := NewTCPTransport(&tptu.Upgrader{
			Secure: ib,
			Muxer:  new(mplex.Transport),
		})

		zero := "/ip4/127.0.0.1/tcp/0"
		ttransport.SubtestTransport(t, ta, tb, zero, ia.LocalPeer())

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

		tpt := NewTCPTransport(&tptu.Upgrader{
			Secure: makeInsecureTransport(t),
			Muxer:  new(mplex.Transport),
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

		tpt := NewTCPTransport(&tptu.Upgrader{
			Secure: makeInsecureTransport(t),
			Muxer:  new(mplex.Transport),
		})

		_, err = tpt.Listen(utpa)
		if err == nil {
			t.Fatal("shouldnt be able to listen on utp addr with tcp transport")
		}

		envReuseportVal = false
	}
	envReuseportVal = true
}

func makeInsecureTransport(t *testing.T) *insecure.Transport {
	priv, pub, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatal(err)
	}
	id, err := peer.IDFromPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return insecure.NewWithIdentity(id, priv)
}
