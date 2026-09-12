package tui

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/linnkit/internal/device"
	"github.com/biomassa/linnkit/internal/scala"
	"github.com/biomassa/linnkit/internal/theory"
)

const deviceTimeout = 3 * time.Second

type scaleEntry struct {
	path, name, desc string
	size             int
	class            string
	err              error
}

type (
	scalesLoadedMsg struct{ entries []scaleEntry }
	deviceStatusMsg struct{ text string }
	jobDoneMsg      struct {
		text   string
		backup string
		slot   int  // light slot now showing, -1 if unchanged
		sent   bool // a send reached the device (even if verifying failed)
	}
)

// loadScales lists and classifies every .scl file in dirs.
func loadScales(dirs []string) tea.Cmd {
	return func() tea.Msg {
		var entries []scaleEntry
		for _, dir := range dirs {
			filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".scl") {
					return nil
				}
				e := scaleEntry{path: path, name: strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))}
				if s, err := scala.ParseSCLFile(path); err != nil {
					e.err, e.class = err, "error"
				} else {
					a := theory.Analyze(s, theory.DefaultOptions())
					e.desc, e.size, e.class = s.Description, a.Structure.Size, a.Structure.Class.String()
				}
				entries = append(entries, e)
				return nil
			})
		}
		sort.SliceStable(entries, func(i, j int) bool {
			return strings.ToLower(entries[i].name) < strings.ToLower(entries[j].name)
		})
		return scalesLoadedMsg{entries}
	}
}

func openDevice(port string) (*device.Device, error) {
	p, err := device.OpenPort(port)
	if err != nil {
		return nil, err
	}
	d, err := device.New(p)
	if err != nil {
		p.Close()
		return nil, err
	}
	return d, nil
}

// readDeviceStatus reads which light slot is showing (a read only).
func readDeviceStatus(port string) tea.Cmd {
	return func() tea.Msg {
		d, err := openDevice(port)
		if err != nil {
			return deviceStatusMsg{"not connected: " + err.Error()}
		}
		defer d.Close()
		v, _ := d.ReadRetry([]int{device.ParamNoteLights}, 2*time.Second, 1)
		n, ok := v[device.ParamNoteLights]
		switch {
		case !ok:
			return deviceStatusMsg{"connected, not answering (asleep? touch a pad)"}
		case n >= device.NoteLightsCustom0:
			return deviceStatusMsg{fmt.Sprintf("connected, showing light slot %d", n-device.NoteLightsCustom0)}
		default:
			return deviceStatusMsg{fmt.Sprintf("connected, showing built-in note lights %d", n)}
		}
	}
}

// sendCmd backs up the device, sends the job and verifies it.
func sendCmd(port string, job device.Job, rec *device.LightsRecord) tea.Cmd {
	return func() tea.Msg {
		d, err := openDevice(port)
		if err != nil {
			return jobDoneMsg{text: "nothing sent: " + err.Error(), slot: -1}
		}
		defer d.Close()
		path, err := d.SaveBackup(deviceTimeout)
		if err != nil {
			return jobDoneMsg{text: "nothing sent: " + err.Error(), slot: -1}
		}
		n, err := d.Run(job, deviceTimeout)
		if err != nil {
			return jobDoneMsg{text: "sent, but " + err.Error(), backup: path, slot: job.Slot, sent: true}
		}
		if rec != nil && !job.Factory { // the device can't report patterns: keep it for restores
			if dir, err := device.BackupDir(); err == nil {
				device.SaveLightsRecord(dir, *rec)
			}
		}
		return jobDoneMsg{text: fmt.Sprintf("sent to light slot %d, %d values verified; backup %s", job.Slot, n, filepath.Base(path)),
			backup: path, slot: job.Slot, sent: true}
	}
}

func backupCmd(port string) tea.Cmd {
	return func() tea.Msg {
		d, err := openDevice(port)
		if err != nil {
			return jobDoneMsg{text: "backup failed: " + err.Error(), slot: -1}
		}
		defer d.Close()
		path, err := d.SaveBackup(deviceTimeout)
		if err != nil {
			return jobDoneMsg{text: "backup failed: " + err.Error(), slot: -1}
		}
		return jobDoneMsg{text: "backup saved: " + filepath.Base(path), backup: path, slot: -1}
	}
}

func restoreCmd(port, path string) tea.Cmd {
	return func() tea.Msg {
		snap, err := device.LoadSnapshot(path)
		if err != nil {
			return jobDoneMsg{text: "restore failed: " + err.Error(), slot: -1}
		}
		d, err := openDevice(port)
		if err != nil {
			return jobDoneMsg{text: "restore failed: " + err.Error(), slot: -1}
		}
		defer d.Close()
		res, err := d.Restore(snap, deviceTimeout)
		changed, saved := res.Changed, res.Saved
		if err != nil {
			return jobDoneMsg{text: "restore failed: " + err.Error(), slot: -1}
		}
		flash := "saved to flash"
		if !saved {
			flash = "not saved to flash (a built-in scale light is showing)"
		}
		slot := -1
		if v, ok := snap.Values[device.ParamNoteLights]; ok && v >= device.NoteLightsCustom0 {
			slot = v - device.NoteLightsCustom0
		}
		painted := "light slots unchanged (no patterns in this backup)"
		if len(res.Painted) > 0 {
			painted = fmt.Sprintf("light slots %v painted again", res.Painted)
		}
		return jobDoneMsg{text: fmt.Sprintf("restored %d parameters from %s, %s; %s", len(changed), filepath.Base(path), flash, painted), slot: slot}
	}
}
