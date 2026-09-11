package device

// ControlChange returns a control change on channel ch (0-15).
func ControlChange(ch, cc, value int) []byte {
	return []byte{0xB0 | byte(ch&0x0F), byte(cc & 0x7F), byte(value & 0x7F)}
}

// NRPN returns the six control changes that set an NRPN on channel 1:
// CC99/98 = parameter, CC6/38 = value, then CC101 = CC100 = 127 to deselect.
func NRPN(param, value int) [][]byte {
	return sixCC(0, 99, 98, param, value)
}

// RPN returns the six control changes that set an RPN on channel ch (0-15).
func RPN(param, value, ch int) [][]byte {
	return sixCC(ch, 101, 100, param, value)
}

func sixCC(ch, msbCC, lsbCC, param, value int) [][]byte {
	return [][]byte{
		ControlChange(ch, msbCC, param>>7),
		ControlChange(ch, lsbCC, param&0x7F),
		ControlChange(ch, 6, value>>7),
		ControlChange(ch, 38, value&0x7F),
		ControlChange(ch, 101, 127),
		ControlChange(ch, 100, 127),
	}
}
