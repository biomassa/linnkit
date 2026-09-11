package device

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/biomassa/linnkit/internal/layout"
	"github.com/biomassa/linnkit/internal/lights"
)

// ErrNoAnswer means readback got nothing back, usually because the unit is asleep.
var ErrNoAnswer = errors.New("the LinnStrument did not answer; if it is asleep, touch a pad to wake it")

// Job is one send: lights into a custom slot, optionally the row layout and
// the MIDI setup.
type Job struct {
	Surface lights.Surface
	Slot    int            // custom light slot 0-2
	Layout  *layout.Layout // nil leaves the rows as they are
	Config  *ChannelConfig // nil leaves the MIDI setup as it is
}

// Run sends the job, then reads back what it set. It returns how many values
// were verified; mismatches or missing answers come back as an error.
func (d *Device) Run(j Job, timeout time.Duration) (int, error) {
	expect := map[int]int{ParamNoteLights: NoteLightsCustom0 + j.Slot}
	if j.Config != nil {
		if err := d.Configure(*j.Config); err != nil {
			return 0, err
		}
		if j.Config.Bend > 0 {
			expect[ParamBendRange], expect[ParamBendRange+RightSplit] = j.Config.Bend, j.Config.Bend
		}
	}
	if j.Layout != nil {
		if err := d.SendLayout(*j.Layout); err != nil {
			return 0, err
		}
		expect[ParamRowOffset] = RowOffsetGuitar
		for row := range layout.Rows {
			expect[ParamGuitarRow1+row] = max(0, min(127, j.Layout.RowStart[row]))
		}
	}
	if err := d.PaintLights(j.Surface, j.Slot); err != nil {
		return 0, err
	}
	// CC23 has just written settings to flash; the device drops queries while busy.
	time.Sleep(500 * time.Millisecond)
	var nums []int
	for n := range expect {
		nums = append(nums, n)
	}
	slices.Sort(nums)
	got, err := d.ReadRetry(nums, timeout, 2)
	var bad []string
	for _, n := range nums {
		if v, ok := got[n]; ok && v != expect[n] {
			bad = append(bad, fmt.Sprintf("%s: device %d, expected %d", Describe(n), v, expect[n]))
		}
	}
	if len(bad) > 0 {
		return len(got) - len(bad), fmt.Errorf("readback mismatch: %s", strings.Join(bad, "; "))
	}
	if err != nil {
		return len(got), fmt.Errorf("verify: %w", err)
	}
	return len(nums), nil
}

// BackupDir is where automatic backups go: ~/.config/linnkit/backups.
func BackupDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "linnkit", "backups"), nil
}

// SaveBackup reads every parameter and writes a timestamped snapshot into BackupDir.
func (d *Device) SaveBackup(timeout time.Duration) (string, error) {
	dir, err := BackupDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	snap, _ := d.Backup(timeout)
	if len(snap.Values) == 0 {
		return "", ErrNoAnswer
	}
	path := filepath.Join(dir, snap.Taken.Format("2006-01-02T15-04-05")+".json")
	return path, snap.Save(path)
}

// LatestBackup returns the newest file in BackupDir.
func LatestBackup() (string, error) {
	dir, err := BackupDir()
	if err != nil {
		return "", err
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	if len(files) == 0 {
		return "", errors.New("no backups yet")
	}
	slices.Sort(files) // names are timestamps
	return files[len(files)-1], nil
}
