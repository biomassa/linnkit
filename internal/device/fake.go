package device

import (
	"fmt"
	"strings"
	"sync"
)

// FakePort records every message and, when Params is set, behaves like a
// LinnStrument: NRPN writes update Params and NRPN 299 queries are answered.
type FakePort struct {
	mu       sync.Mutex
	Sent     [][]byte
	Params   map[int]int
	listener func([]byte)
	state    [16]ccState
}

type ccState struct {
	msb, lsb, vmsb int
	rpn            bool
}

func (f *FakePort) Send(msg []byte) error {
	f.mu.Lock()
	f.Sent = append(f.Sent, append([]byte(nil), msg...))
	var reply [][]byte
	if f.Params != nil && len(msg) == 3 && msg[0]&0xF0 == 0xB0 {
		reply = f.handleCC(int(msg[0]&0x0F), int(msg[1]), int(msg[2]))
	}
	listener := f.listener
	f.mu.Unlock()
	if listener != nil {
		for _, r := range reply {
			listener(r)
		}
	}
	return nil
}

func (f *FakePort) handleCC(ch, cc, v int) [][]byte {
	st := &f.state[ch]
	switch cc {
	case 99:
		st.msb, st.rpn = v, false
	case 98:
		st.lsb, st.rpn = v, false
	case 101, 100:
		st.rpn = true
	case 6:
		st.vmsb = v
	case 38:
		if st.rpn {
			return nil
		}
		param, value := st.msb<<7|st.lsb, st.vmsb<<7|v
		if param == ParamQuery {
			if cur, ok := f.Params[value]; ok {
				return [][]byte{ControlChange(0, 99, value>>7), ControlChange(0, 98, value&0x7F),
					ControlChange(0, 6, cur>>7), ControlChange(0, 38, cur&0x7F)}
			}
			return nil
		}
		f.Params[param] = value
	}
	return nil
}

func (f *FakePort) SetListener(l func([]byte)) error {
	f.mu.Lock()
	f.listener = l
	f.mu.Unlock()
	return nil
}

func (f *FakePort) Close() error { return nil }

// Log returns the sent messages as "channel cc value" lines (channel 0-15),
// the format of the golden files in testdata.
func (f *FakePort) Log() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var b strings.Builder
	for _, m := range f.Sent {
		if len(m) == 3 && m[0]&0xF0 == 0xB0 {
			fmt.Fprintf(&b, "%d %d %d\n", m[0]&0x0F, m[1], m[2])
		} else {
			fmt.Fprintf(&b, "% x\n", m)
		}
	}
	return b.String()
}
