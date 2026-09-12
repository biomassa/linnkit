package device

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"time"
)

// Snapshot is a readback of every readable parameter.
type Snapshot struct {
	Taken       time.Time      `json:"taken"`
	Values      map[int]int    `json:"values"`
	Lights      *LightsRecord  `json:"lights,omitempty"`       // the slot 2 pattern (linn.lights format)
	LightsSlots []LightsRecord `json:"lights_slots,omitempty"` // the slot 0 and 1 patterns
}

// RestoreResult says what a restore did.
type RestoreResult struct {
	Changed []int // parameters written back
	Saved   bool  // settings saved to flash (a custom slot was showing)
	Painted []int // light slots painted again from the backup's patterns
}

// Backup reads every readable parameter.
func (d *Device) Backup(timeout time.Duration) (Snapshot, error) {
	values, err := d.ReadRetry(ReadableParams(), timeout, 2)
	return Snapshot{Taken: time.Now(), Values: values}, err
}

// Restore writes back the parameters that differ from the snapshot, in
// ascending order, skipping actions (see noRestore). Then it paints every
// light slot the snapshot has a pattern for (show it, paint, CC23) and shows
// the slot the snapshot had showing. Finally it saves to flash with CC23 when
// a custom light slot is showing; otherwise the settings stay active until
// power-off and Saved is false.
func (d *Device) Restore(s Snapshot, timeout time.Duration) (RestoreResult, error) {
	var res RestoreResult
	var nums []int
	for n := range s.Values {
		if _, skip := noRestore[n]; !skip {
			nums = append(nums, n)
		}
	}
	slices.Sort(nums)
	current, err := d.Read(nums, timeout)
	if err != nil {
		return res, fmt.Errorf("reading current values: %w", err)
	}
	for _, n := range nums {
		if current[n] == s.Values[n] {
			continue
		}
		if err := d.SetNRPN(n, s.Values[n]); err != nil {
			return res, err
		}
		res.Changed = append(res.Changed, n)
	}
	records := s.lightRecords()
	for _, r := range records {
		if err := d.PaintLights(r.surface(), r.Slot); err != nil {
			return res, err
		}
		res.Painted = append(res.Painted, r.Slot)
	}
	if len(records) > 0 {
		last := NoteLightsCustom0 + records[len(records)-1].Slot
		if shown, ok := s.Values[ParamNoteLights]; ok && shown != last {
			if err := d.SetNRPN(ParamNoteLights, shown); err != nil {
				return res, err
			}
		}
		time.Sleep(500 * time.Millisecond) // CC23 wrote to flash; the device drops queries while busy
	}
	lightsNow, err := d.ReadRetry([]int{ParamNoteLights}, timeout, 2)
	if err != nil {
		return res, err
	}
	if slot := lightsNow[ParamNoteLights] - NoteLightsCustom0; slot >= 0 && slot <= 2 {
		// CC23 re-saves the slot that is showing (same picture) and calls storeSettings().
		res.Saved = true
		return res, d.CC(0, 23, slot)
	}
	return res, nil
}

// Save writes the snapshot as JSON.
func (s Snapshot) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// LoadSnapshot reads a snapshot written by Save.
func LoadSnapshot(path string) (Snapshot, error) {
	var s Snapshot
	data, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	if err := decodeFirst(data, &s); err != nil { // tolerates a leftover tail (see decodeFirst)
		return s, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

// Describe returns "num name" for a parameter number.
func Describe(num int) string {
	if p, ok := LookupParam(num); ok {
		return strconv.Itoa(num) + " " + p.Name
	}
	return strconv.Itoa(num)
}
