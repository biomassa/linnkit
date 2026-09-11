package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/biomassa/linnkit/internal/device"
	"github.com/biomassa/linnkit/internal/layout"
	"github.com/biomassa/linnkit/internal/lights"
	"github.com/biomassa/linnkit/internal/scala"
	"github.com/biomassa/linnkit/internal/synth"
	"github.com/biomassa/linnkit/internal/theory"
)

// MinWidth and MinHeight are the smallest terminal the dashboard draws in.
const (
	MinWidth  = 160
	MinHeight = 50
)

type pane int

const (
	paneScales pane = iota
	paneScale
	paneLayout
	paneLights
	paneSynth
	paneSend
	paneCount
)

type overlay int

const (
	noOverlay overlay = iota
	overlayMatrix
	overlayTable
	overlayHelp
	overlayConfirmSend
	overlayConfirmRestore
)

// Config is how the dashboard starts.
type Config struct {
	ScaleDirs []string // folders searched for .scl files
	Port      string   // MIDI port name (substring)
	Root      int      // MIDI note of degree 0
}

var (
	schemes      = []string{"ji", "names", "mos", "root"}
	schemeTitles = []string{"just-interval families", "note names", "MOS inside the scale", "root only"}
	limits       = []int{3, 5, 7, 11, 13}
)

// Model is the dashboard state.
type Model struct {
	cfg           Config
	width, height int
	focus         pane
	overlay       overlay
	scrollY       int // overlay scroll
	scrollX       int

	entries   []scaleEntry
	loading   bool
	filter    string
	filtering bool
	sel       int // cursor in the visible scale list

	analysis    *theory.Analysis
	loaded      string // path of the analysed scale
	tableScroll int

	cands   []layout.Candidate
	candSel int
	low     int // bottom-left note chosen with [ ], -1 = from the candidate
	lay     layout.Layout

	scheme  int
	limitIx int
	surface lights.Surface
	legend  string

	synthIx   int // index into synth.Profiles
	synthBend int // the synth's per-note bend range S

	slot        int
	withLayout  bool
	withConfig  bool
	busy        bool
	status      string
	deviceInfo  string
	restoreFrom string
	lastBackup  string
}

// New returns the dashboard model. Sends default to scratch light slot 2.
func New(cfg Config) Model {
	if cfg.Root == 0 {
		cfg.Root = 60
	}
	if cfg.Port == "" {
		cfg.Port = device.DefaultPortName
	}
	return Model{cfg: cfg, loading: true, limitIx: 2, slot: 2, withLayout: true, withConfig: true, low: -1,
		synthBend: synth.Profiles[0].DefaultBend,
		status:    "loading scales", deviceInfo: "checking device"}
}

// Init loads the scale list and checks the device.
func (m Model) Init() tea.Cmd {
	return tea.Batch(loadScales(m.cfg.ScaleDirs), readDeviceStatus(m.cfg.Port))
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case scalesLoadedMsg:
		m.entries, m.loading = msg.entries, false
		m.status = fmt.Sprintf("%d scales", len(m.entries))
		if m.analysis == nil && len(m.visible()) > 0 {
			m = m.load(0)
		}
	case deviceStatusMsg:
		m.deviceInfo = msg.text
	case jobDoneMsg:
		m.busy = false
		m.status = msg.text
		if msg.backup != "" {
			m.lastBackup = msg.backup
		}
		if msg.slot >= 0 {
			m.deviceInfo = fmt.Sprintf("connected, showing light slot %d", msg.slot)
		}
	case tea.KeyPressMsg:
		return m.key(msg.String())
	}
	return m, nil
}

func (m Model) key(k string) (tea.Model, tea.Cmd) {
	if k == "ctrl+c" {
		return m, tea.Quit
	}
	if m.overlay != noOverlay {
		return m.overlayKey(k)
	}
	if m.filtering {
		return m.filterKey(k), nil
	}
	switch k {
	case "q":
		return m, tea.Quit
	case "tab", "right":
		m.focus = (m.focus + 1) % paneCount
	case "shift+tab", "left":
		m.focus = (m.focus + paneCount - 1) % paneCount
	case "?":
		m.overlay, m.scrollY, m.scrollX = overlayHelp, 0, 0
	case "m":
		if m.analysis != nil {
			m.overlay, m.scrollY, m.scrollX = overlayMatrix, 0, 0
		}
	case "t":
		if m.analysis != nil {
			m.overlay, m.scrollY, m.scrollX = overlayTable, 0, 0
		}
	case "/":
		m.focus, m.filtering = paneScales, true
	case "0", "1", "2":
		m.slot = int(k[0] - '0')
	case "l":
		m.withLayout = !m.withLayout
	case "c":
		m.withConfig = !m.withConfig
	case "e":
		m.status = "export to Madrona Labs: planned for M5"
	case "s":
		if m.busy {
			m.status = "busy"
		} else if m.analysis != nil {
			m.overlay = overlayConfirmSend
		}
	case "b":
		if !m.busy {
			m.busy, m.status = true, "backing up device settings"
			return m, backupCmd(m.cfg.Port)
		}
	case "r":
		if !m.busy {
			path := m.lastBackup
			if path == "" {
				var err error
				if path, err = device.LatestBackup(); err != nil {
					m.status = "restore: " + err.Error()
					return m, nil
				}
			}
			m.restoreFrom, m.overlay = path, overlayConfirmRestore
		}
	default:
		m = m.paneKey(k)
	}
	return m, nil
}

func (m Model) paneKey(k string) Model {
	switch m.focus {
	case paneScales:
		n := len(m.visible())
		switch k {
		case "up":
			m.sel = max(0, m.sel-1)
		case "down":
			m.sel = min(max(0, n-1), m.sel+1)
		case "enter":
			if n > 0 {
				m = m.load(m.sel)
			}
		}
	case paneScale:
		switch k {
		case "up":
			m.tableScroll = max(0, m.tableScroll-1)
		case "down":
			m.tableScroll++
		}
	case paneLayout:
		switch k {
		case "up":
			m.candSel, m.low = max(0, m.candSel-1), -1
		case "down":
			m.candSel, m.low = min(len(m.cands)-1, m.candSel+1), -1
		case "[":
			m.low = max(0, m.lay.RowStart[0]-1)
		case "]":
			m.low = min(127, m.lay.RowStart[0]+1)
		}
		m = m.paint()
	case paneLights:
		switch k {
		case "up":
			m.scheme = max(0, m.scheme-1)
		case "down":
			m.scheme = min(len(schemes)-1, m.scheme+1)
		case "<":
			m.limitIx = max(0, m.limitIx-1)
		case ">":
			m.limitIx = min(len(limits)-1, m.limitIx+1)
		}
		m = m.paint()
	case paneSynth:
		switch k {
		case "up", "down":
			if k == "up" {
				m.synthIx = max(0, m.synthIx-1)
			} else {
				m.synthIx = min(len(synth.Profiles)-1, m.synthIx+1)
			}
			m.synthBend = synth.Profiles[m.synthIx].DefaultBend
		case "<", ">":
			allowed := m.profile().Allowed()
			i := 0
			for j, s := range allowed {
				if s == m.synthBend {
					i = j
				}
			}
			if k == "<" {
				i = max(0, i-1)
			} else {
				i = min(len(allowed)-1, i+1)
			}
			m.synthBend = allowed[i]
		}
	}
	return m
}

func (m Model) profile() synth.Profile { return synth.Profiles[m.synthIx] }

// bendPlan is the pitch-bend setup for the loaded scale and chosen synth.
func (m Model) bendPlan() synth.Plan {
	return synth.PlanFor(m.synthBend, m.analysis.Structure.Steps, m.analysis.PeriodCents)
}

func (m Model) filterKey(k string) Model {
	switch k {
	case "enter", "esc":
		m.filtering = false
	case "backspace":
		if m.filter != "" {
			m.filter = m.filter[:len(m.filter)-1]
		}
	default:
		if len([]rune(k)) == 1 {
			m.filter += k
		}
	}
	m.sel = 0
	return m
}

func (m Model) overlayKey(k string) (tea.Model, tea.Cmd) {
	switch m.overlay {
	case overlayConfirmSend:
		switch k {
		case "y":
			m.overlay, m.busy = noOverlay, true
			m.status = fmt.Sprintf("sending to light slot %d", m.slot)
			return m, sendCmd(m.cfg.Port, m.job())
		case "n", "esc", "q":
			m.overlay = noOverlay
		}
		return m, nil
	case overlayConfirmRestore:
		switch k {
		case "y":
			m.overlay, m.busy = noOverlay, true
			m.status = "restoring " + m.restoreFrom
			return m, restoreCmd(m.cfg.Port, m.restoreFrom)
		case "n", "esc", "q":
			m.overlay = noOverlay
		}
		return m, nil
	}
	switch k {
	case "esc", "q", "m", "t", "?":
		m.overlay = noOverlay
	case "up":
		m.scrollY = max(0, m.scrollY-1)
	case "down":
		m.scrollY++
	case "left":
		m.scrollX = max(0, m.scrollX-16)
	case "right":
		m.scrollX += 16
	}
	return m, nil
}

// visible returns the scale entries that match the filter.
func (m Model) visible() []scaleEntry {
	if m.filter == "" {
		return m.entries
	}
	f := strings.ToLower(m.filter)
	var out []scaleEntry
	for _, e := range m.entries {
		if strings.Contains(strings.ToLower(e.name), f) || strings.Contains(strings.ToLower(e.desc), f) {
			out = append(out, e)
		}
	}
	return out
}

// load analyses the visible entry i and rebuilds candidates and lights.
func (m Model) load(i int) Model {
	e := m.visible()[i]
	s, err := scala.ParseSCLFile(e.path)
	if err != nil {
		m.status = "error: " + err.Error()
		return m
	}
	m.analysis = theory.Analyze(s, theory.DefaultOptions())
	m.loaded, m.sel = e.path, i
	m.cands = layout.Candidates(m.analysis, layout.Options{Root: m.cfg.Root})
	m.candSel, m.low, m.tableScroll = 0, -1, 0
	m.status = "loaded " + e.name
	return m.paint()
}

// paint applies the chosen layout and light scheme.
func (m Model) paint() Model {
	if m.analysis == nil || len(m.cands) == 0 {
		return m
	}
	m.lay = m.cands[m.candSel].Layout
	if m.low >= 0 {
		m.lay = layout.Uniform(m.lay.Offset, m.low)
	}
	n := m.analysis.Structure.Size
	var sw []lights.Swatch
	switch schemes[m.scheme] {
	case "ji":
		sw = lights.JIFamilies(m.analysis, limits[m.limitIx], 0)
		m.legend = fmt.Sprintf("R root  3 3-limit  5 5-limit  7 7-limit  11 11-limit  13 13-limit   (limit %d)", limits[m.limitIx])
	case "names":
		sw = lights.NoteNames(m.analysis, lights.Magenta, lights.White, lights.Blue, lights.Green)
		m.legend = "C root  naturals white  sharps blue  flats green"
	case "mos":
		if mos, ok := lights.DefaultMOS(m.analysis); ok {
			sw = lights.MOSPattern(n, mos, lights.Magenta, lights.White, lights.Blue)
			m.legend = fmt.Sprintf("R root  o MOS (%d notes, generator %d degrees)  x other", mos.Size, mos.Generator)
		} else {
			sw = lights.RootOnly(n, lights.Magenta)
			m.legend = "no MOS found in this scale: root only"
		}
	default:
		sw = lights.RootOnly(n, lights.Magenta)
		m.legend = "R root"
	}
	m.surface = lights.Paint(m.lay, m.cfg.Root, sw)
	return m
}

// job describes what a send does with the current settings.
func (m Model) job() device.Job {
	j := device.Job{Surface: m.surface, Slot: m.slot}
	if m.withLayout || m.withConfig {
		l := m.lay
		j.Layout = &l
	}
	if m.withConfig {
		cfg := device.ChannelConfig{Main: 1, Bend: m.bendPlan().LinnBend, OneChannel: !m.profile().MPE}
		for ch := 2; ch <= 16; ch++ {
			cfg.PerNote = append(cfg.PerNote, ch)
		}
		j.Config = &cfg
	}
	return j
}
