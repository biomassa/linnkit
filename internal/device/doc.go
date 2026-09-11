// Package device talks to the LinnStrument over MIDI: ports, NRPN/RPN/CC encoding,
// the parameter table, readback (NRPN 299), backup and restore, sending layouts and
// lights, and loading presets with a light-slot refresh.
//
// Facts come from reference/linnstrument-facts.md and refs/linnstrument-firmware.
package device
