// Copyright 2011 GoDCCP Authors. All rights reserved.
// Use of this source code is governed by a 
// license that can be found in the LICENSE file.

package dccp

// muxMsg{} contains the source and destination labels of a flow.
type muxMsg struct {
	Source, Sink *Label
}

const muxMsgFootprint = 2 * labelFootprint

// readMuxMsg() decodes a muxMsg{} from wire format
func readMuxMsg(p []byte) (msg *muxMsg, n int, err error) {
	source, n0, err := ReadLabel(p)
	if err != nil {
		return nil, 0, err
	}
	dest, n1, err := ReadLabel(p[n0:])
	if err != nil {
		return nil, 0, err
	}
	return &muxMsg{source, dest}, n0 + n1, nil
}

// Write() encodes the muxMsg{} to p@ in wire format
func (msg *muxMsg) Write(p []byte) (n int, err error) {
	n0, err := msg.Source.Write(p)
	if err != nil {
		return 0, err
	}
	n1, err := msg.Sink.Write(p[n0:])
	if err != nil {
		return 0, err
	}
	return n0 + n1, nil
}
