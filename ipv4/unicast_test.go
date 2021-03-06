// Copyright 2012 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv4_test

import (
	"net"
	"os"
	"runtime"
	"testing"
	"time"

	"golang.org/x/net/internal/iana"
	"golang.org/x/net/internal/icmp"
	"golang.org/x/net/internal/nettest"
	"golang.org/x/net/ipv4"
)

func TestPacketConnReadWriteUnicastUDP(t *testing.T) {
	switch runtime.GOOS {
	case "nacl", "plan9", "solaris", "windows":
		t.Skipf("not supported on %q", runtime.GOOS)
	}
	ifi := nettest.RoutedInterface("ip4", net.FlagUp|net.FlagLoopback)
	if ifi == nil {
		t.Skipf("not available on %q", runtime.GOOS)
	}

	c, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.ListenPacket failed: %v", err)
	}
	defer c.Close()

	dst, err := net.ResolveUDPAddr("udp4", c.LocalAddr().String())
	if err != nil {
		t.Fatalf("net.ResolveUDPAddr failed: %v", err)
	}
	p := ipv4.NewPacketConn(c)
	defer p.Close()
	cf := ipv4.FlagTTL | ipv4.FlagDst | ipv4.FlagInterface

	for i, toggle := range []bool{true, false, true} {
		if err := p.SetControlMessage(cf, toggle); err != nil {
			if nettest.ProtocolNotSupported(err) {
				t.Skipf("not supported on %q", runtime.GOOS)
			}
			t.Fatalf("ipv4.PacketConn.SetControlMessage failed: %v", err)
		}
		p.SetTTL(i + 1)
		if err := p.SetWriteDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
			t.Fatalf("ipv4.PacketConn.SetWriteDeadline failed: %v", err)
		}
		if _, err := p.WriteTo([]byte("HELLO-R-U-THERE"), nil, dst); err != nil {
			t.Fatalf("ipv4.PacketConn.WriteTo failed: %v", err)
		}
		rb := make([]byte, 128)
		if err := p.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
			t.Fatalf("ipv4.PacketConn.SetReadDeadline failed: %v", err)
		}
		if _, cm, _, err := p.ReadFrom(rb); err != nil {
			t.Fatalf("ipv4.PacketConn.ReadFrom failed: %v", err)
		} else {
			t.Logf("rcvd cmsg: %v", cm)
		}
	}
}

func TestPacketConnReadWriteUnicastICMP(t *testing.T) {
	switch runtime.GOOS {
	case "nacl", "plan9", "solaris", "windows":
		t.Skipf("not supported on %q", runtime.GOOS)
	}
	if os.Getuid() != 0 {
		t.Skip("must be root")
	}
	ifi := nettest.RoutedInterface("ip4", net.FlagUp|net.FlagLoopback)
	if ifi == nil {
		t.Skipf("not available on %q", runtime.GOOS)
	}

	c, err := net.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		t.Fatalf("net.ListenPacket failed: %v", err)
	}
	defer c.Close()

	dst, err := net.ResolveIPAddr("ip4", "127.0.0.1")
	if err != nil {
		t.Fatalf("ResolveIPAddr failed: %v", err)
	}
	p := ipv4.NewPacketConn(c)
	defer p.Close()
	cf := ipv4.FlagTTL | ipv4.FlagDst | ipv4.FlagInterface

	for i, toggle := range []bool{true, false, true} {
		wb, err := (&icmp.Message{
			Type: ipv4.ICMPTypeEcho, Code: 0,
			Body: &icmp.Echo{
				ID: os.Getpid() & 0xffff, Seq: i + 1,
				Data: []byte("HELLO-R-U-THERE"),
			},
		}).Marshal(nil)
		if err != nil {
			t.Fatalf("icmp.Message.Marshal failed: %v", err)
		}
		if err := p.SetControlMessage(cf, toggle); err != nil {
			if nettest.ProtocolNotSupported(err) {
				t.Skipf("not supported on %q", runtime.GOOS)
			}
			t.Fatalf("ipv4.PacketConn.SetControlMessage failed: %v", err)
		}
		p.SetTTL(i + 1)
		if err := p.SetWriteDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
			t.Fatalf("ipv4.PacketConn.SetWriteDeadline failed: %v", err)
		}
		if _, err := p.WriteTo(wb, nil, dst); err != nil {
			t.Fatalf("ipv4.PacketConn.WriteTo failed: %v", err)
		}
		b := make([]byte, 128)
	loop:
		if err := p.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
			t.Fatalf("ipv4.PacketConn.SetReadDeadline failed: %v", err)
		}
		if n, cm, _, err := p.ReadFrom(b); err != nil {
			t.Fatalf("ipv4.PacketConn.ReadFrom failed: %v", err)
		} else {
			t.Logf("rcvd cmsg: %v", cm)
			m, err := icmp.ParseMessage(iana.ProtocolICMP, b[:n])
			if err != nil {
				t.Fatalf("icmp.ParseMessage failed: %v", err)
			}
			if runtime.GOOS == "linux" && m.Type == ipv4.ICMPTypeEcho {
				// On Linux we must handle own sent packets.
				goto loop
			}
			if m.Type != ipv4.ICMPTypeEchoReply || m.Code != 0 {
				t.Fatalf("got type=%v, code=%v; expected type=%v, code=%v", m.Type, m.Code, ipv4.ICMPTypeEchoReply, 0)
			}
		}
	}
}

func TestRawConnReadWriteUnicastICMP(t *testing.T) {
	switch runtime.GOOS {
	case "nacl", "plan9", "solaris", "windows":
		t.Skipf("not supported on %q", runtime.GOOS)
	}
	if os.Getuid() != 0 {
		t.Skip("must be root")
	}
	ifi := nettest.RoutedInterface("ip4", net.FlagUp|net.FlagLoopback)
	if ifi == nil {
		t.Skipf("not available on %q", runtime.GOOS)
	}

	c, err := net.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		t.Fatalf("net.ListenPacket failed: %v", err)
	}
	defer c.Close()

	dst, err := net.ResolveIPAddr("ip4", "127.0.0.1")
	if err != nil {
		t.Fatalf("ResolveIPAddr failed: %v", err)
	}
	r, err := ipv4.NewRawConn(c)
	if err != nil {
		t.Fatalf("ipv4.NewRawConn failed: %v", err)
	}
	defer r.Close()
	cf := ipv4.FlagTTL | ipv4.FlagDst | ipv4.FlagInterface

	for i, toggle := range []bool{true, false, true} {
		wb, err := (&icmp.Message{
			Type: ipv4.ICMPTypeEcho, Code: 0,
			Body: &icmp.Echo{
				ID: os.Getpid() & 0xffff, Seq: i + 1,
				Data: []byte("HELLO-R-U-THERE"),
			},
		}).Marshal(nil)
		if err != nil {
			t.Fatalf("icmp.Message.Marshal failed: %v", err)
		}
		wh := &ipv4.Header{
			Version:  ipv4.Version,
			Len:      ipv4.HeaderLen,
			TOS:      i + 1,
			TotalLen: ipv4.HeaderLen + len(wb),
			TTL:      i + 1,
			Protocol: 1,
			Dst:      dst.IP,
		}
		if err := r.SetControlMessage(cf, toggle); err != nil {
			if nettest.ProtocolNotSupported(err) {
				t.Skipf("not supported on %q", runtime.GOOS)
			}
			t.Fatalf("ipv4.RawConn.SetControlMessage failed: %v", err)
		}
		if err := r.SetWriteDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
			t.Fatalf("ipv4.RawConn.SetWriteDeadline failed: %v", err)
		}
		if err := r.WriteTo(wh, wb, nil); err != nil {
			t.Fatalf("ipv4.RawConn.WriteTo failed: %v", err)
		}
		rb := make([]byte, ipv4.HeaderLen+128)
	loop:
		if err := r.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
			t.Fatalf("ipv4.RawConn.SetReadDeadline failed: %v", err)
		}
		if _, b, cm, err := r.ReadFrom(rb); err != nil {
			t.Fatalf("ipv4.RawConn.ReadFrom failed: %v", err)
		} else {
			t.Logf("rcvd cmsg: %v", cm)
			m, err := icmp.ParseMessage(iana.ProtocolICMP, b)
			if err != nil {
				t.Fatalf("icmp.ParseMessage failed: %v", err)
			}
			if runtime.GOOS == "linux" && m.Type == ipv4.ICMPTypeEcho {
				// On Linux we must handle own sent packets.
				goto loop
			}
			if m.Type != ipv4.ICMPTypeEchoReply || m.Code != 0 {
				t.Fatalf("got type=%v, code=%v; expected type=%v, code=%v", m.Type, m.Code, ipv4.ICMPTypeEchoReply, 0)
			}
		}
	}
}
