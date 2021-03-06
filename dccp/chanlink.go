// Copyright 2011 GoDCCP Authors. All rights reserved.
// Use of this source code is governed by a 
// license that can be found in the LICENSE file.

package dccp

import (
	"net"
	"time"
)

// ChanLink treats one side of a channel as an incoming packet link
type ChanLink struct {
	Mutex
	in, out chan []byte
}

func NewChanPipe() (p, q *ChanLink) {
	c0 := make(chan []byte)
	c1 := make(chan []byte)
	return &ChanLink{in: c0, out: c1}, &ChanLink{in: c1, out: c0}
}

func (l *ChanLink) GetMTU() int {
	return 1500
}

func (l *ChanLink) SetReadDeadline(t time.Time) error {
	// SetReadDeadline does not apply because ChanLink returns
	// an EIO error if no input is available
	panic("SetReadDeadline does not apply")
	return nil
}

func (l *ChanLink) ReadFrom(buf []byte) (n int, addr net.Addr, err error) {
	l.Lock()
	in := l.in
	l.Unlock()
	if in == nil {
		return 0, nil, ErrBad
	}

	p, ok := <-in
	if !ok {
		return 0, nil, ErrIO
	}
	n = copy(buf, p)
	if n != len(p) {
		panic("insufficient buf len")
	}
	return len(p), nil, nil
}

func (l *ChanLink) WriteTo(buf []byte, addr net.Addr) (n int, err error) {
	l.Lock()
	out := l.out
	l.Unlock()
	if out == nil {
		return 0, ErrBad
	}

	p := make([]byte, len(buf))
	copy(p, buf)
	out <- p
	return len(buf), nil
}

func (l *ChanLink) Close() error {
	l.Lock()
	defer l.Unlock()

	if l.out != nil {
		close(l.out)
		l.out = nil
	}
	return nil
}
