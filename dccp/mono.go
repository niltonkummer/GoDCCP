// Copyright 2011 GoDCCP Authors. All rights reserved.
// Use of this source code is governed by a 
// license that can be found in the LICENSE file.

package dccp

import (
	"sync"
)

// An instance of monotoneTime has the singular function of returning the time Now.
// It has the property that consecutive invokations return strictly increasing numbers.
// This may provide superfluous logic, but we use it for piece of mind.
type monotoneTime struct {
	sync.Mutex
	env  *Env
	last int64
}

func (x *monotoneTime) Init(env *Env) {
	x.env = env
	x.last = 0
}

func (x *monotoneTime) Now() int64 {
	x.Lock()
	defer x.Unlock()
	now := x.env.Now()
	// TODO: If now - x.last is hugely negative we might want to report some sort of error
	if now < x.last {
		panic("negative time in mono")
	}
	x.last = max64(now, x.last)
	return x.last
}
