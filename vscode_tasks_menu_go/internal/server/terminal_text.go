package server

// stripTerminalControlSequences removes terminal presentation controls from raw
// PTY output before path parsing. Patch Tool deliberately highlights primary
// upload artifacts with ANSI SGR/underline sequences on TTYs, so detectors must
// parse the visible text rather than the raw escape bytes.
func stripTerminalControlSequences(text string) string {
	if text == "" {
		return text
	}
	in := []byte(text)
	out := make([]byte, 0, len(in))
	for i := 0; i < len(in); {
		if in[i] != 0x1b {
			out = append(out, in[i])
			i++
			continue
		}
		if i+1 >= len(in) {
			break
		}
		switch in[i+1] {
		case '[': // CSI: ESC [ ... final-byte
			i += 2
			for i < len(in) {
				b := in[i]
				i++
				if b >= 0x40 && b <= 0x7e {
					break
				}
			}
		case ']': // OSC: ESC ] ... BEL or ST
			i += 2
			for i < len(in) {
				if in[i] == 0x07 {
					i++
					break
				}
				if in[i] == 0x1b && i+1 < len(in) && in[i+1] == '\\' {
					i += 2
					break
				}
				i++
			}
		case 'P', 'X', '^', '_': // DCS/SOS/PM/APC: terminated by ST
			i += 2
			for i < len(in) {
				if in[i] == 0x1b && i+1 < len(in) && in[i+1] == '\\' {
					i += 2
					break
				}
				i++
			}
		default:
			// Two-byte ESC sequence such as DECSC/DECRC. It is presentation or
			// terminal state, never part of a filesystem path.
			i += 2
		}
	}
	return string(out)
}
