// Copyright 2011 GoDCCP Authors. All rights reserved.
// Use of this source code is governed by a 
// license that can be found in the LICENSE file.

package dccp

import "io"

// backOff{}
type backOff struct {
	env         *Env
	sleep       int64 // Duration of next sleep interval
	lifetime    int64 // Total lifetime so far
	timeout     int64 // Maximum time the backoff mechanism stays alive
	backoffFreq int64 // Backoff period. The sleep duration backs off approximately every backoffFreq nanoseconds
	lastBackoff int64 // Last time the sleep interval was backed off, relative to the starting time
}

// newBackOff() creates a new back-off timer whose first wait period is firstSleep
// nanoseconds. Approximately every backoffFreq nanoseconds, the sleep timers backs off
// (increases by a factor of 4/3).  The lifetime of the backoff sleep intervals does not
// exceed timeout.
func newBackOff(env *Env, firstSleep, timeout, backoffFreq int64) *backOff {
	return &backOff{
		env:         env,
		sleep:       firstSleep,
		lifetime:    0,
		timeout:     timeout,
		backoffFreq: backoffFreq,
		lastBackoff: 0,
	}
}

// BackoffMin is the minimum time before two firings of the backoff timers
const BackoffMin = 100e6

// Sleep() blocks for the duration of the next sleep interval in the back-off 
// sequence and return nil. If the maximum total sleep time has been reached,
// Sleep() returns os.EOF without sleeping.
func (b *backOff) Sleep() (error, int64) {
	if b.lifetime >= b.timeout {
		return io.EOF, 0
	}
	effectiveSleep := max64(BackoffMin, b.sleep)
	b.env.Sleep(effectiveSleep)
	b.lifetime += effectiveSleep
	if b.lifetime-b.lastBackoff >= b.backoffFreq {
		b.sleep = (4 * b.sleep) / 3
		b.lastBackoff = b.lifetime
	}
	return nil, b.env.Now()
}
