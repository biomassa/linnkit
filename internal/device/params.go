package device

//go:generate go run ../../tools/genparams -src ../../refs/linnstrument-firmware/ls_midi.ino -out params_gen.go

// Param is one NRPN parameter as the firmware handles it.
type Param struct {
	Num      int // 0-99 split (left split; the right split is Num+100), 200+ global
	Name     string
	Min, Max int
	HasRange bool // the firmware ignores values outside Min..Max
	Readable bool // answered by an NRPN 299 query
}

// Parameter numbers used by the app. See reference/linnstrument-facts.md.
const (
	ParamMIDIMode      = 0
	ParamMainChannel   = 1
	ParamPerNoteFirst  = 2 // channels 1..16 are 2..17
	ParamBendRange     = 19
	ParamSendX         = 20
	ParamQuantize      = 21
	ParamQuantHold     = 22
	ParamSendY         = 24
	ParamYCC           = 25
	ParamSendZ         = 27
	ParamZExpression   = 28
	ParamYExpression   = 39
	ParamPlayedMode    = 61
	ParamRowOffset     = 227
	ParamPresetLoad    = 243
	ParamNoteLights    = 247
	ParamGuitarRow1    = 263 // rows 1..8 are 263..270
	ParamQuery         = 299
	RowOffsetGuitar    = 13
	NoteLightsCustom0  = 9 // custom slots 0..2 are note-lights presets 9..11
	RightSplit         = 100
	firstGlobalParam   = 200
	lastSplitParamNums = 99
)

// noRestore lists readable parameters that are actions or would break the
// connection, so Restore never writes them.
var noRestore = map[int]string{
	62:              "sequencer play toggle",
	63:              "sequencer previous pattern",
	64:              "sequencer next pattern",
	65:              "sequencer pattern select",
	66:              "sequencer mute toggle",
	162:             "sequencer play toggle (right)",
	163:             "sequencer previous pattern (right)",
	164:             "sequencer next pattern (right)",
	165:             "sequencer pattern select (right)",
	166:             "sequencer mute toggle (right)",
	234:             "MIDI I/O (switching would cut the USB connection)",
	ParamPresetLoad: "preset load (would overwrite every setting)",
	245:             "user firmware mode",
	ParamQuery:      "query",
}

// noAnswer lists parameters that sendNrpnParameter handles but never answers
// (sequencer actions without a value); observed on firmware 2.3.4.
var noAnswer = map[int]bool{62: true, 63: true, 64: true}

var paramIndex = func() map[int]Param {
	m := map[int]Param{}
	for _, p := range firmwareParams {
		m[p.Num] = p
	}
	return m
}()

// LookupParam returns the parameter with this number; 100-199 are the right split.
func LookupParam(num int) (Param, bool) {
	if num >= RightSplit && num < firstGlobalParam {
		p, ok := paramIndex[num-RightSplit]
		if !ok || p.Num > lastSplitParamNums {
			return Param{}, false
		}
		p.Num = num
		p.Name = "Right: " + p.Name
		return p, true
	}
	p, ok := paramIndex[num]
	return p, ok
}

// ReadableParams returns every parameter number NRPN 299 can read, both splits included.
func ReadableParams() []int {
	var out []int
	for _, p := range firmwareParams {
		if !p.Readable || p.Num == ParamQuery || noAnswer[p.Num] {
			continue
		}
		out = append(out, p.Num)
	}
	var right []int
	for _, n := range out {
		if n <= lastSplitParamNums {
			right = append(right, n+RightSplit)
		}
	}
	return append(out, right...)
}
