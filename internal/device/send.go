package device

import (
	"fmt"

	"github.com/biomassa/linnkit/internal/layout"
	"github.com/biomassa/linnkit/internal/lights"
)

// PaintLights paints a surface into custom light slot 0-2 and saves it with
// CC23, which also writes all settings to flash. The LinnStrument must show
// its normal play screen (CC22 is ignored on settings screens). Pads are sent
// column by column, as the Python scripts do.
func (d *Device) PaintLights(s lights.Surface, slot int) error {
	if slot < 0 || slot > 2 {
		return fmt.Errorf("light slot %d: must be 0, 1 or 2", slot)
	}
	if err := d.SetNRPN(ParamNoteLights, NoteLightsCustom0+slot); err != nil {
		return err
	}
	for col := 1; col <= layout.Cols; col++ {
		for row := range layout.Rows {
			if err := d.send(ControlChange(0, 20, col), ControlChange(0, 21, row), ControlChange(0, 22, int(s[row][col-1].Color))); err != nil {
				return err
			}
		}
	}
	return d.CC(0, 23, slot)
}

// ShowLightSlot selects custom light slot 0-2. Loading a preset does not
// refresh the custom lights; this does.
func (d *Device) ShowLightSlot(slot int) error {
	return d.SetNRPN(ParamNoteLights, NoteLightsCustom0+slot)
}

// SendLayout sets Guitar row offset mode with each row's starting note
// (clamped to 0..127, as the firmware stores 0..127).
func (d *Device) SendLayout(l layout.Layout) error {
	if err := d.SetNRPN(ParamRowOffset, RowOffsetGuitar); err != nil {
		return err
	}
	for row := range layout.Rows {
		if err := d.SetNRPN(ParamGuitarRow1+row, max(0, min(127, l.RowStart[row]))); err != nil {
			return err
		}
	}
	return nil
}

// ChannelConfig is the MIDI setup for both splits.
type ChannelConfig struct {
	Main    int   // main channel 1-16
	PerNote []int // per-note channels 1-16
	Bend    int   // LinnStrument Bend Range 1-96; 0 leaves it unchanged
	// MPE uses the MPE Configuration Message instead of plain Channel Per Note;
	// the device then sends its bend range to the synth (RPN 0). Per-note
	// channels must be Main+1..Main+N with Main = 1.
	MPE bool
}

// Configure sets MIDI mode, channels, bend range and Y/Z expression on both
// splits (the same sequence as reference/linnstrument_edo_just.py --configure,
// without the layout, which SendLayout adds).
func (d *Device) Configure(c ChannelConfig) error {
	perNote := map[int]bool{}
	for _, ch := range c.PerNote {
		if ch != c.Main {
			perNote[ch] = true
		}
	}
	if c.MPE {
		for ch := range perNote {
			if c.Main != 1 || ch < 2 || ch > len(perNote)+1 {
				return fmt.Errorf("MPE needs main channel 1 and per-note channels 2..N")
			}
		}
		if err := d.SendRPN(6, len(perNote)<<7, 0); err != nil {
			return err
		}
		if err := d.SendRPN(6, len(perNote)<<7, 15); err != nil {
			return err
		}
	}
	type set struct{ param, value int }
	for _, base := range []int{0, RightSplit} {
		var sets []set
		if !c.MPE {
			sets = append(sets, set{ParamMIDIMode, 1}, set{ParamMainChannel, c.Main})
			for ch := 1; ch <= 16; ch++ {
				v := 0
				if perNote[ch] {
					v = 1
				}
				sets = append(sets, set{ParamPerNoteFirst + ch - 1, v})
			}
		}
		if c.Bend > 0 {
			sets = append(sets, set{ParamBendRange, c.Bend})
		}
		sets = append(sets, set{ParamSendX, 1}, set{ParamSendY, 1}, set{ParamYExpression, 2},
			set{ParamYCC, 74}, set{ParamSendZ, 1}, set{ParamZExpression, 1})
		for _, s := range sets {
			if err := d.SetNRPN(base+s.param, s.value); err != nil {
				return err
			}
		}
	}
	return nil
}
