package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/biomassa/linnkit/internal/device"
	"github.com/biomassa/linnkit/internal/layout"
)

const readTimeout = 3 * time.Second

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

func runDevice(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: linnkit device ports | read [NUM...] | backup FILE | restore FILE  [--port NAME]")
		return 2
	}
	defer device.CloseDriver()
	cmd := args[0]
	fs := flag.NewFlagSet("device "+cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	port := fs.String("port", device.DefaultPortName, "MIDI port name (substring)")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if cmd == "ports" {
		for _, name := range device.OutputPorts() {
			fmt.Fprintln(stdout, name)
		}
		return 0
	}
	d, err := openDevice(*port)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	defer d.Close()

	switch cmd {
	case "read":
		nums := device.ReadableParams()
		if fs.NArg() > 0 {
			nums = nil
			for _, a := range fs.Args() {
				n, err := strconv.Atoi(a)
				if err != nil {
					fmt.Fprintf(stderr, "error: %q is not a parameter number\n", a)
					return 2
				}
				nums = append(nums, n)
			}
		}
		values, err := d.Read(nums, readTimeout)
		writeValues(stdout, values)
		if len(values) == 0 {
			fmt.Fprintln(stderr, "error: the LinnStrument did not answer; if it is asleep, touch a pad to wake it")
			return 1
		}
		if err != nil {
			fmt.Fprintln(stderr, "warning:", err)
		}
	case "backup":
		if fs.NArg() != 1 {
			fmt.Fprintln(stderr, "usage: linnkit device backup FILE")
			return 2
		}
		snap, err := d.Backup(readTimeout)
		if err != nil {
			fmt.Fprintln(stderr, "warning:", err)
		}
		if err := snap.Save(fs.Arg(0)); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		fmt.Fprintf(stdout, "saved %d values to %s\n", len(snap.Values), fs.Arg(0))
	case "restore":
		if fs.NArg() != 1 {
			fmt.Fprintln(stderr, "usage: linnkit device restore FILE")
			return 2
		}
		snap, err := device.LoadSnapshot(fs.Arg(0))
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		changed, saved, err := d.Restore(snap, readTimeout)
		for _, n := range changed {
			fmt.Fprintf(stdout, "restored %s = %d\n", device.Describe(n), snap.Values[n])
		}
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		fmt.Fprintf(stdout, "%d parameters changed; saved to flash: %s\n", len(changed), yesNo(saved))
	default:
		fmt.Fprintf(stderr, "error: unknown device command %q\n", cmd)
		return 2
	}
	return 0
}

func writeValues(w io.Writer, values map[int]int) {
	var nums []int
	for n := range values {
		nums = append(nums, n)
	}
	slices.Sort(nums)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, n := range nums {
		p, _ := device.LookupParam(n)
		fmt.Fprintf(tw, "%d\t%s\t%d\n", n, p.Name, values[n])
	}
	tw.Flush()
}

func runSend(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	fs.SetOutput(stderr)
	g := addGridFlags(fs)
	port := fs.String("port", device.DefaultPortName, "MIDI port name (substring)")
	slot := fs.Int("slot", -1, "custom light slot 0-2 to paint and save (required)")
	sendLayout := fs.Bool("layout", false, "also send the row layout (Guitar rows)")
	configure := fs.Bool("configure", false, "also set MIDI mode, channels, bend range and Y/Z")
	perNote := fs.String("per-note", "2-16", "with --configure: per-note channels, e.g. 2-16 or 2-5")
	bend := fs.Int("bend", 0, "with --configure: LinnStrument Bend Range (0 = number of scale degrees, if 1-96)")
	mpe := fs.Bool("mpe", false, "with --configure: MPE state instead of plain Channel Per Note")
	noBackup := fs.Bool("no-backup", false, "skip the settings backup before sending")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 || *slot < 0 || *slot > 2 {
		fmt.Fprintln(stderr, "usage: linnkit send --slot 0|1|2 [--layout] [--configure] [grid flags] FILE")
		fs.PrintDefaults()
		return 2
	}
	r, err := g.build(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	writeSurface(stdout, r, g, false)

	defer device.CloseDriver()
	d, err := openDevice(*port)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	defer d.Close()

	if !*noBackup {
		path, err := backupNow(d)
		if err != nil {
			fmt.Fprintln(stderr, "error: backup failed, nothing sent:", err)
			return 1
		}
		fmt.Fprintf(stdout, "\nbackup: %s (restore with: linnkit device restore %s)\n", path, path)
	}

	expect := map[int]int{device.ParamNoteLights: device.NoteLightsCustom0 + *slot}
	if *configure {
		cfg := device.ChannelConfig{Main: 1, Bend: *bend, MPE: *mpe}
		if cfg.PerNote, err = parseChannels(*perNote); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 2
		}
		if n := r.analysis.Structure.Size; cfg.Bend == 0 && n <= 96 {
			cfg.Bend = n
		}
		if err := d.Configure(cfg); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		if cfg.Bend > 0 {
			expect[device.ParamBendRange], expect[device.ParamBendRange+device.RightSplit] = cfg.Bend, cfg.Bend
		}
	}
	if *sendLayout || *configure {
		if err := d.SendLayout(r.layout); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		expect[device.ParamRowOffset] = device.RowOffsetGuitar
		for row := range layout.Rows {
			expect[device.ParamGuitarRow1+row] = max(0, min(127, r.layout.RowStart[row]))
		}
	}
	if err := d.PaintLights(r.surface, *slot); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	fmt.Fprintf(stdout, "sent; lights saved to slot %d\n", *slot)
	return verify(d, expect, stdout, stderr)
}

// verify reads back the expected values and reports differences.
func verify(d *device.Device, expect map[int]int, stdout, stderr io.Writer) int {
	var nums []int
	for n := range expect {
		nums = append(nums, n)
	}
	slices.Sort(nums)
	// CC23 has just written settings to flash; the device drops queries while busy.
	time.Sleep(500 * time.Millisecond)
	got, err := d.ReadRetry(nums, readTimeout, 2)
	if err != nil {
		fmt.Fprintln(stderr, "verify:", err)
	}
	bad := 0
	for _, n := range nums {
		if v, ok := got[n]; ok && v != expect[n] {
			fmt.Fprintf(stdout, "MISMATCH %s: device %d, expected %d\n", device.Describe(n), v, expect[n])
			bad++
		}
	}
	if bad > 0 || err != nil {
		return 1
	}
	fmt.Fprintf(stdout, "verified %d values by readback\n", len(nums))
	return 0
}

func backupNow(d *device.Device) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", "linnkit", "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	snap, err := d.Backup(readTimeout)
	if len(snap.Values) == 0 {
		return "", fmt.Errorf("the LinnStrument did not answer; if it is asleep, touch a pad to wake it (%v)", firstLine(err))
	}
	path := filepath.Join(dir, snap.Taken.Format("2006-01-02T15-04-05")+".json")
	return path, snap.Save(path)
}

// firstLine shortens long errors (such as a list of 200 missing parameters).
func firstLine(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > 80 {
		s = s[:77] + "..."
	}
	return s
}

func parseChannels(spec string) ([]int, error) {
	var out []int
	for _, part := range strings.Split(spec, ",") {
		lo, hi, isRange := strings.Cut(part, "-")
		a, err := strconv.Atoi(strings.TrimSpace(lo))
		if err != nil {
			return nil, fmt.Errorf("bad channel list %q", spec)
		}
		b := a
		if isRange {
			if b, err = strconv.Atoi(strings.TrimSpace(hi)); err != nil {
				return nil, fmt.Errorf("bad channel list %q", spec)
			}
		}
		for ch := a; ch <= b; ch++ {
			if ch < 1 || ch > 16 {
				return nil, fmt.Errorf("channel %d outside 1-16", ch)
			}
			out = append(out, ch)
		}
	}
	return out, nil
}
