package device

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/biomassa/linnkit/internal/lights"
)

func TestLightsRecordsInBackups(t *testing.T) {
	dir := t.TempDir()
	var s lights.Surface
	s[0][0].Color, s[7][24].Color = lights.Magenta, lights.White
	if err := SaveLightsRecord(dir, NewLightsRecord(2, "31-edo", "ji", 7, 60, 10, -1, s)); err != nil {
		t.Fatal(err)
	}
	SaveLightsRecord(dir, NewLightsRecord(0, "22edo", "names", 7, 60, 7, 48, s))
	snap := Snapshot{Taken: time.Now(), Values: map[int]int{247: 11}}
	snap.AttachLights(dir)
	path := filepath.Join(dir, "2026-09-12T10-00-00.json")
	if err := snap.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Lights == nil || got.Lights.Slot != 2 || got.Lights.Pattern[0][0] != 6 || got.Lights.Pattern[7][24] != 8 ||
		len(got.LightsSlots) != 1 || got.LightsSlots[0].Slot != 0 || got.LightsSlots[0].Low != 48 {
		t.Fatalf("backup lights: %+v %+v", got.Lights, got.LightsSlots)
	}
	if recs := got.lightRecords(); len(recs) != 2 || recs[0].Slot != 0 || recs[1].Slot != 2 {
		t.Errorf("records %v", recs)
	}
	if !strings.HasSuffix(got.Lights.Taken, "Z") || len(got.Lights.Taken) != len("2026-09-12T10:00:00.000Z") {
		t.Errorf("taken %q should be ISO 8601 UTC like JavaScript's toISOString", got.Lights.Taken)
	}
}

func TestRestorePaintsRecordedSlots(t *testing.T) {
	device := map[int]int{247: 11, 19: 48}
	d, f := newFake(t, device)
	var s lights.Surface
	s[3][4].Color = lights.Green
	r2 := NewLightsRecord(2, "31-edo", "ji", 7, 60, 10, -1, s)
	r0 := NewLightsRecord(0, "22edo", "names", 7, 60, 7, -1, lights.Surface{})
	snap := Snapshot{Values: map[int]int{247: 10, 19: 31}, Lights: &r2, LightsSlots: []LightsRecord{r0}}
	f.Sent = nil
	res, err := d.Restore(snap, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(res.Painted) != "[0 2]" || !res.Saved || device[19] != 31 || device[247] != 10 {
		t.Fatalf("result %+v, device %v", res, device)
	}
	var colours, saves []int
	green := false
	for i, m := range f.Sent {
		if m[0] == 0xB0 && m[1] == 22 {
			colours = append(colours, int(m[2]))
			if m[2] == 3 && f.Sent[i-2][2] == 5 && f.Sent[i-1][2] == 3 { // column 5, row 3
				green = true
			}
		}
		if m[0] == 0xB0 && m[1] == 23 {
			saves = append(saves, int(m[2]))
		}
	}
	if len(colours) != 400 || !green {
		t.Errorf("painted %d pads (want 2 x 200), green pad found: %v", len(colours), green)
	}
	if fmt.Sprint(saves) != "[0 2 1]" { // slot 0, slot 2, then the backup's showing slot 1 re-saved
		t.Errorf("CC23 %v", saves)
	}
}

func TestMaxBackupLoads(t *testing.T) {
	row := "[" + strings.TrimSuffix(strings.Repeat("0,", 24)+"6", ",") + "]"
	pattern := "[" + strings.TrimSuffix(strings.Repeat(row+",", 8), ",") + "]"
	data := `{"taken":"2026-09-12T10:00:00.000Z","values":{"19":48,"247":11},` +
		`"lights":{"slot":2,"scale":"31-edo","scheme":"ji","limit":7,"root":60,"offset":10,"low":-1,` +
		`"taken":"2026-09-12T09:59:00.000Z","pattern":` + pattern + `,"restored":"/x/backup.json"}}` +
		"\nbox/musicstuff/Max 9/linnstrument/reference/backups/backup.json\"\n}\n" // Max leaves an old tail
	path := filepath.Join(t.TempDir(), "backup.json")
	os.WriteFile(path, []byte(data), 0o644)
	snap, err := LoadSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	recs := snap.lightRecords()
	if len(recs) != 1 || recs[0].Slot != 2 || recs[0].Pattern[5][24] != 6 || snap.Values[247] != 11 {
		t.Errorf("Max backup: %+v %v", recs, snap.Values)
	}
	bad := LightsRecord{Slot: 2, Pattern: make([][]int, 7)}
	if (Snapshot{Lights: &bad}).lightRecords() != nil {
		t.Error("a pattern that isn't 8 x 25 is ignored")
	}
}
