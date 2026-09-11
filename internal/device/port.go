package device

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
	_ "gitlab.com/gomidi/midi/v2/drivers/rtmididrv" // RtMidi through cgo
)

// DefaultPortName is how the LinnStrument shows up on macOS.
const DefaultPortName = "LinnStrument MIDI"

// Port sends raw MIDI messages and delivers incoming ones to a listener.
type Port interface {
	Send(msg []byte) error
	SetListener(func(msg []byte)) error
	Close() error
}

// OutputPorts lists the MIDI output port names.
func OutputPorts() []string {
	var names []string
	for _, p := range midi.GetOutPorts() {
		names = append(names, p.String())
	}
	return names
}

type midiPort struct {
	out  drivers.Out
	in   drivers.In
	stop func()
}

// OpenPort opens the input and output port whose name contains name.
func OpenPort(name string) (Port, error) {
	out, err := findOut(name)
	if err != nil {
		return nil, err
	}
	if err := out.Open(); err != nil {
		return nil, fmt.Errorf("open output %q: %w", out, err)
	}
	in, err := findIn(name)
	if err != nil {
		out.Close()
		return nil, err
	}
	return &midiPort{out: out, in: in}, nil
}

// OpenOutput opens only the output port whose name contains name.
func OpenOutput(name string) (Port, error) {
	out, err := findOut(name)
	if err != nil {
		return nil, err
	}
	if err := out.Open(); err != nil {
		return nil, fmt.Errorf("open output %q: %w", out, err)
	}
	return &midiPort{out: out}, nil
}

// OpenInput opens only the input port whose name contains name; SetListener
// starts listening.
func OpenInput(name string) (Port, error) {
	in, err := findIn(name)
	if err != nil {
		return nil, err
	}
	return &midiPort{in: in}, nil
}

func findOut(name string) (drivers.Out, error) {
	for _, p := range midi.GetOutPorts() {
		if strings.Contains(p.String(), name) {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no MIDI output named %q; found %v", name, OutputPorts())
}

func findIn(name string) (drivers.In, error) {
	for _, p := range midi.GetInPorts() {
		if strings.Contains(p.String(), name) {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no MIDI input named %q", name)
}

// debugMIDI prints every message sent and received when LINNKIT_MIDI_DEBUG is set.
var debugMIDI = os.Getenv("LINNKIT_MIDI_DEBUG") != ""

func (p *midiPort) Send(msg []byte) error {
	if p.out == nil {
		return errors.New("input-only port")
	}
	if debugMIDI {
		fmt.Fprintf(os.Stderr, "midi out % x\n", msg)
	}
	return p.out.Send(msg)
}

func (p *midiPort) SetListener(f func([]byte)) error {
	stop, err := midi.ListenTo(p.in, func(msg midi.Message, _ int32) {
		if debugMIDI {
			fmt.Fprintf(os.Stderr, "midi in  % x\n", msg.Bytes())
		}
		f(msg.Bytes())
	})
	if err != nil {
		return fmt.Errorf("listen on %q: %w", p.in, err)
	}
	p.stop = stop
	return nil
}

// Close stops listening and closes both ports explicitly: an open input port
// once hung a Python RtMidi process at exit.
func (p *midiPort) Close() error {
	if p.stop != nil {
		p.stop()
	}
	var errIn, errOut error
	if p.in != nil {
		errIn = p.in.Close()
	}
	if p.out != nil {
		errOut = p.out.Close()
	}
	if errIn != nil {
		return errIn
	}
	return errOut
}

// CloseDriver releases the MIDI driver; call once when the program ends.
func CloseDriver() { midi.CloseDriver() }
