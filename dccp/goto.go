// Copyright 2011 GoDCCP Authors. All rights reserved.
// Use of this source code is governed by a 
// license that can be found in the LICENSE file.

package dccp

import "fmt"

const (
	REQUEST_BACKOFF_FIRST      = 1e9      // Initial re-send period for client Request resends is 1 sec, in ns
	REQUEST_BACKOFF_FREQ       = 10e9     // Back-off Request resend every 10 secs, in ns
	REQUEST_BACKOFF_TIMEOUT    = 30e9     // Request re-sends quit after 30 sec, in ns (shorter than RFC recommendation)

	RESPOND_TIMEOUT            = 30e9     // Timeout in RESPOND state, 30 sec in ns

	LISTEN_TIMEOUT             = REQUEST_BACKOFF_TIMEOUT    // Timeout in LISTEN state

	CLOSING_BACKOFF_FREQ       = 64e9     // Backoff frequency of CLOSING timer, 64 seconds, Section 8.3
	CLOSING_BACKOFF_TIMEOUT    = MSL/4    // Maximum time in CLOSING (RFC recommends MSL, but seems too long)

	TIMEWAIT_TIMEOUT           = MSL/2    // Time to stay in TIMEWAIT, Section 8.3 recommends MSL*2

	PARTOPEN_BACKOFF_FIRST     = 200e6    // 200 miliseconds in ns, Section 8.1.5
	PARTOPEN_BACKOFF_FREQ      = 200e6    // 200 miliseconds in ns
	PARTOPEN_BACKOFF_TIMEOUT   = 30e9     // 30 sec (Section 8.1.5 recommends 8 min)

	EXPIRE_INTERVAL	           = 1e9      // Interval for checking expiration conditions
)

func (c *Conn) gotoLISTEN() {
	c.AssertLocked()
	c.socket.SetServer(true)
	c.socket.SetState(LISTEN)
	c.emitSetState()
	c.env.Expire(
		func()bool {
			c.Lock()
			state := c.socket.GetState()
			c.Unlock()
			// If we've transitioned away from LISTEN, we are in good shape
			return state != LISTEN
		}, 
		func() {
			// Otherwise abort the connection
			c.abortQuietly()
		}, 
		LISTEN_TIMEOUT, EXPIRE_INTERVAL, "gotoLISTEN")
}

func (c *Conn) gotoRESPOND(hServiceCode uint32, hSeqNo int64) {
	c.AssertLocked()
	c.socket.SetState(RESPOND)
	c.emitSetState()
	iss := c.socket.ChooseISS()
	c.socket.SetGAR(iss)
	c.socket.SetISR(hSeqNo)
	c.socket.SetGSR(hSeqNo)
	// TODO: To be more prudent, set service code only if it is currently 0,
	// otherwise check that h.ServiceCode matches socket service code
	c.socket.SetServiceCode(hServiceCode)

	c.env.Expire(
		func()bool {
			c.Lock()
			state := c.socket.GetState()
			c.Unlock()
			return state != RESPOND
		}, 
		func() {
			c.abortQuietly()
		}, 
		RESPOND_TIMEOUT, EXPIRE_INTERVAL, "gotoRESPOND")
}

func (c *Conn) gotoREQUEST(serviceCode uint32) {
	c.AssertLocked()
	c.socket.SetServer(false)
	c.socket.SetState(REQUEST)
	c.emitSetState()
	c.socket.SetServiceCode(serviceCode)
	iss := c.socket.ChooseISS()
	c.socket.SetGAR(iss)
	c.inject(c.generateRequest(serviceCode))

	// Resend Request using exponential backoff, if no response
	c.env.Go(func() {
		b := newBackOff(c.env, REQUEST_BACKOFF_FIRST, REQUEST_BACKOFF_TIMEOUT, REQUEST_BACKOFF_FREQ)
		for {
			err, _ := b.Sleep()
			c.Lock()
			state := c.socket.GetState()
			c.Unlock()
			if state != REQUEST {
				break
			}
			// If the back-off timer has reached maximum wait, quit trying
			if err != nil {
				c.abort()
				break
			}
			c.Lock()
			c.amb.E(EventTurn, "Request resend")
			c.inject(c.generateRequest(serviceCode))
			c.Unlock()
		}
	}, "gotoREQUEST")
}

func (c *Conn) openCCID() {
	c.AssertLocked()
	if c.ccidOpen {
		return
	}
	c.scc.Open()
	c.rcc.Open()
	c.ccidOpen = true
	c.amb.E(EventMatch, "CCID open")
}

func (c *Conn) closeCCID() {
	c.AssertLocked()
	if !c.ccidOpen {
		return
	}
	c.scc.Close()
	c.rcc.Close()
	c.ccidOpen = false
	c.amb.E(EventMatch, "CCID close")
}

func (c *Conn) gotoPARTOPEN() {
	c.AssertLocked()
	c.socket.SetState(PARTOPEN)
	c.emitSetState()
	c.openCCID()
	c.inject(nil) // Unblocks the writeLoop select, so it can see the state change

	// Start PARTOPEN timer, according to Section 8.1.5
	c.env.Go(func() {
		b := newBackOff(c.env, PARTOPEN_BACKOFF_FIRST, PARTOPEN_BACKOFF_TIMEOUT, PARTOPEN_BACKOFF_FREQ)
		c.amb.E(EventInfo, "PARTOPEN backoff start")
		for {
			err, btm := b.Sleep()
			c.Lock()
			state := c.socket.GetState()
			c.Unlock()
			if state != PARTOPEN {
				c.amb.E(EventInfo, "PARTOPEN backoff EXIT via state change")
				break
			}
			// If the back-off timer has reached maximum wait. End the connection.
			if err != nil {
				c.abort()
				break
			}
			c.amb.E(EventInfo, fmt.Sprintf("PARTOPEN backoff %d", btm))
			c.Lock()
			c.inject(c.generateAck())
			c.Unlock()
		}
	}, "gotoPARTOPEN")
}

func (c *Conn) gotoOPEN(hSeqNo int64) {
	c.AssertLocked()
	c.socket.SetOSR(hSeqNo)
	c.socket.SetState(OPEN)
	c.emitSetState()
	c.openCCID()
	c.inject(nil) // Unblocks the writeLoop select, so it can see the state change
}

func (c *Conn) gotoTIMEWAIT() {
	c.AssertLocked()
	c.setError(ErrEOF)
	c.teardownUser()
	c.socket.SetState(TIMEWAIT)
	c.emitSetState()
	c.closeCCID()

	c.env.Go(func() {
		c.env.Sleep(TIMEWAIT_TIMEOUT)
		c.abortQuietly()
	}, "gotoTIMEWAIT")
}

func (c *Conn) gotoCLOSING() {
	c.AssertLocked()
	c.setError(ErrEOF)
	c.teardownUser()
	c.socket.SetState(CLOSING)
	c.emitSetState()
	c.closeCCID()
	c.env.Go(func() {
		c.Lock()
		rtt := c.socket.GetRTT()
		c.Unlock()
		c.amb.E(EventInfo, fmt.Sprintf("CLOSING RTT=%dns", rtt))
		b := newBackOff(c.env, 2*rtt, CLOSING_BACKOFF_TIMEOUT, CLOSING_BACKOFF_FREQ)
		for {
			err, _ := b.Sleep()
			c.Lock()
			state := c.socket.GetState()
			c.Unlock()
			if state != CLOSING {
				break
			}
			if err != nil {
				c.Lock()
				c.gotoTIMEWAIT()
				c.Unlock()
				break
			}
			c.amb.E(EventInfo, "Resend Close")
			c.Lock()
			c.inject(c.generateClose())
			c.Unlock()
		}
	}, "gotoCLOSING")
}

// gotoCLOSED MUST be idempotent
func (c *Conn) gotoCLOSED() {
	c.AssertLocked()
	c.emitSetState()
	c.socket.SetState(CLOSED)
	c.setError(ErrAbort)
	c.teardownUser()
	c.teardownWriteLoop()
	c.closeCCID()
}
