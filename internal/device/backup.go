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
	Taken  time.Time   `json:"taken"`
	Values map[int]int `json:"values"`
}

// Backup reads every readable parameter.
func (d *Device) Backup(timeout time.Duration) (Snapshot, error) {
	values, err := d.ReadRetry(ReadableParams(), timeout, 2)
	return Snapshot{Taken: time.Now(), Values: values}, err
}

// Restore writes back the parameters that differ from the snapshot, in
// ascending order, skipping actions (see noRestore). It then saves to flash
// with CC23 when a custom light slot is showing; otherwise the settings stay
// active until power-off and saved is false.
func (d *Device) Restore(s Snapshot, timeout time.Duration) (changed []int, saved bool, err error) {
	var nums []int
	for n := range s.Values {
		if _, skip := noRestore[n]; !skip {
			nums = append(nums, n)
		}
	}
	slices.Sort(nums)
	current, err := d.Read(nums, timeout)
	if err != nil {
		return nil, false, fmt.Errorf("reading current values: %w", err)
	}
	for _, n := range nums {
		if current[n] == s.Values[n] {
			continue
		}
		if err := d.SetNRPN(n, s.Values[n]); err != nil {
			return changed, false, err
		}
		changed = append(changed, n)
	}
	lightsNow, err := d.Read([]int{ParamNoteLights}, timeout)
	if err != nil {
		return changed, false, err
	}
	if slot := lightsNow[ParamNoteLights] - NoteLightsCustom0; slot >= 0 && slot <= 2 {
		// CC23 re-saves the slot that is showing (same picture) and calls storeSettings().
		return changed, true, d.CC(0, 23, slot)
	}
	return changed, false, nil
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
	if err := json.Unmarshal(data, &s); err != nil {
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
