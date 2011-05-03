// Copyright 2010 GoDCCP Authors. All rights reserved.
// Use of this source code is governed by a 
// license that can be found in the LICENSE file.

package dccp

import (
	"fmt"
	"net"
	"testing"
)

// TODO: 
//   Test over-sized writes
//   Test that small writes are not combined in single packets

type endToEnd struct {
	t     *testing.T
	alink Link
	dlink Link
	addr  net.Addr
	nc    int
	done  chan int
}

func newEndToEnd(t *testing.T, alink,dlink Link, addr net.Addr, nc int) *endToEnd { 
	return &endToEnd{ t, alink, dlink, addr, nc, make(chan int) } 
}

func (ee *endToEnd) acceptLoop(link Link) {

	m := newMux(link, link.FragmentLen())

	// Accept connections
	gg := make(chan int)
	for i := 0; i < ee.nc; i++ {
		c, err := m.Accept()
		if err != nil {
			ee.t.Fatalf("accept %s", c, err)
		}
		go func(c net.Conn) {
			i := int(readUint32(ee.t, c))
			fmt.Printf("ACCEPTING %d\n", i)

			// Expect to read the number i i-times
			for j := 0; j < i; j++ {
				i0 := int(readUint32(ee.t, c))
				if i0 != i {
					ee.t.Fatalf("expecting %d, got %d\n", i, i0)
				}
				fmt.Printf("%d/%d --> %d\n", j+1, i, i0)
			}
			if err = c.Close(); err != nil {
				ee.t.Fatalf("close %s", err)
			}
			gg <- i
			ee.done <- i
		}(c)
	}
	// Wait until all flows finish
	for i := 0; i < ee.nc; i++ {
		<-gg
	}
	fmt.Printf("CLOSING accept loop\n")
	if err := m.Close(); err != nil {
		ee.t.Errorf("a-close: %s", err)
	}
}

func (ee *endToEnd) dialLoop(link Link) {

	m := newMux(link, link.FragmentLen())

	// Dial connections
	gg := make(chan int)
	for i := 0; i < ee.nc; i++ {
		go func(i int) {
			c, err := m.Dial(ee.addr)
			if err != nil {
				ee.t.Fatalf("dial #%d: %s", i, err)
			}
			writeUint32(ee.t, c, uint32(i))

			// Write the number i i-times
			for j := 0; j < i; j++ {
				writeUint32(ee.t, c, uint32(i))
			}
			if err = c.Close(); err != nil {
				ee.t.Fatalf("close: %s", err)
			}
			gg <- i
		}(i)
	}
	// Wait until all flows finish
	for i := 0; i < ee.nc; i++ {
		<-gg
	}
	fmt.Printf("CLOSING dial loop\n")
	if err := m.Close(); err != nil {
		ee.t.Errorf("d-close: %s", err)
	}
}

func readUint32(t *testing.T, c net.Conn) uint32 {
	p := make([]byte, 400)
	n, err := c.Read(p)
	if err != nil {
		t.Fatalf("read: %s", err)
	}
	if n != 4 {
		t.Fatalf("read size: %d != 4", n)
	}
	// fmt.Printf("  %s ···> %v\n", c.(*flow).String(), p[:4])
	return decode4ByteUint(p[:4])
}

func writeUint32(t *testing.T, c net.Conn, u uint32) {
	p := make([]byte, 4)
	encode4ByteUint(u, p)
	n, err := c.Write(p)
	if err != nil {
		t.Fatalf("write: %s", err)
	}
	if n != 4 {
		t.Fatalf("write·size: %d != 4", n)
	}
	// fmt.Printf("  %s <··· %v\n", c.(*flow).String(), p)
}

func (ee *endToEnd) Run() {
	go ee.acceptLoop(ee.alink)
	go ee.dialLoop(ee.dlink)
	for i := 0; i < ee.nc; i++ {
		k, ok := <-ee.done
		if !ok {
			ee.t.Fatalf("premature close")
		}
		fmt.Printf("finished with conn #%d\n", k)
	}
}

func TestMuxOverChan(t *testing.T) {
	alink, dlink := NewChanPipe()
	ee := newEndToEnd(t, alink, dlink, nil, 10)
	ee.Run()
}

func _TestMuxOverUDP(t *testing.T) {
	// Bind acceptor link
	aaddr, err := net.ResolveUDPAddr("udp", "0.0.0.0:44000")
	if err != nil {
		t.Fatalf("resolve a-addr: %s", err)
	}
	alink, err := BindUDPLink("udp", aaddr)
	if err != nil {
		t.Fatalf("bind udp a-link: %s", err)
	}

	// Bind dialer link
	daddr, err := net.ResolveUDPAddr("udp", "0.0.0.0:44001")
	if err != nil {
		t.Fatalf("resolve d-addr: %s", err)
	}
	dlink, err := BindUDPLink("udp", daddr)
	if err != nil {
		t.Fatalf("bind udp d-link: %s", err)
	}

	// Resolve dialer address
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:44000")
	if err != nil {
		t.Fatalf("resolve addr: %s", err)
	}

	ee := newEndToEnd(t, alink, dlink, addr, 10)
	ee.Run()
}
