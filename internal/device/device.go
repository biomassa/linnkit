package device

import (
	"fmt"
	"slices"
	"sync"
	"time"
)

// Device sends and receives LinnStrument messages through a Port.
type Device struct {
	port    Port
	Delay   time.Duration // pause after each message; 2 ms like the Python scripts
	mu      sync.Mutex
	state   [16]ccState
	replies chan [2]int // parameter, value
}

// New wraps a port and starts listening for readback replies.
func New(p Port) (*Device, error) {
	d := &Device{port: p, Delay: 2 * time.Millisecond, replies: make(chan [2]int, 1024)}
	if err := p.SetListener(d.receive); err != nil {
		return nil, err
	}
	return d, nil
}

// Close closes the port.
func (d *Device) Close() error { return d.port.Close() }

func (d *Device) send(msgs ...[]byte) error {
	for _, m := range msgs {
		if err := d.port.Send(m); err != nil {
			return err
		}
		if d.Delay > 0 {
			time.Sleep(d.Delay)
		}
	}
	return nil
}

// CC sends a control change on channel ch (0-15).
func (d *Device) CC(ch, cc, value int) error { return d.send(ControlChange(ch, cc, value)) }

// SetNRPN sets a parameter. Values outside the firmware's range are rejected,
// since the firmware would ignore them silently.
func (d *Device) SetNRPN(param, value int) error {
	if p, ok := LookupParam(param); ok && p.HasRange && (value < p.Min || value > p.Max) {
		return fmt.Errorf("NRPN %d (%s): value %d outside %d..%d", param, p.Name, value, p.Min, p.Max)
	}
	return d.send(NRPN(param, value)...)
}

// SendRPN sends an RPN on channel ch (0-15).
func (d *Device) SendRPN(param, value, ch int) error { return d.send(RPN(param, value, ch)...) }

// receive collects NRPN replies (CC99, CC98, CC6, CC38). RPN traffic from the
// device (CC101/100, e.g. the MPE bend range) is ignored.
func (d *Device) receive(msg []byte) {
	if len(msg) != 3 || msg[0]&0xF0 != 0xB0 {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	st := &d.state[msg[0]&0x0F]
	v := int(msg[2])
	switch msg[1] {
	case 99:
		st.msb, st.rpn = v, false
	case 98:
		st.lsb, st.rpn = v, false
	case 101, 100:
		st.rpn = true
	case 6:
		st.vmsb = v
	case 38:
		if !st.rpn {
			select {
			case d.replies <- [2]int{st.msb<<7 | st.lsb, st.vmsb<<7 | v}:
			default:
			}
		}
	}
}

// ReadRetry is Read followed by up to retries more rounds for parameters that
// did not answer: the device drops replies when many queries stream back to back,
// and while it writes settings to flash.
func (d *Device) ReadRetry(params []int, timeout time.Duration, retries int) (map[int]int, error) {
	got, err := d.Read(params, timeout)
	for try := 0; err != nil && try < retries; try++ {
		var missing []int
		for _, p := range params {
			if _, ok := got[p]; !ok {
				missing = append(missing, p)
			}
		}
		time.Sleep(300 * time.Millisecond)
		var more map[int]int
		more, err = d.Read(missing, timeout)
		for p, v := range more {
			got[p] = v
		}
	}
	return got, err
}

// Read asks for parameters with NRPN 299 and waits up to timeout for the answers.
// It returns what arrived; the error lists parameters that did not answer.
func (d *Device) Read(params []int, timeout time.Duration) (map[int]int, error) {
	for len(d.replies) > 0 {
		<-d.replies
	}
	want := map[int]bool{}
	for _, p := range params {
		want[p] = true
		if err := d.send(NRPN(ParamQuery, p)...); err != nil {
			return nil, err
		}
	}
	got := map[int]int{}
	deadline := time.After(timeout)
	for len(got) < len(want) {
		select {
		case r := <-d.replies:
			if want[r[0]] {
				got[r[0]] = r[1]
			}
		case <-deadline:
			var missing []int
			for p := range want {
				if _, ok := got[p]; !ok {
					missing = append(missing, p)
				}
			}
			slices.Sort(missing)
			return got, fmt.Errorf("no answer for %d parameters: %v", len(missing), missing)
		}
	}
	return got, nil
}
