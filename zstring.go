package main

var a0_default = [...]uint8{'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z'}
var a1_default = [...]uint8{'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z'}
var a2_v1 = [...]uint8{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '.', ',', '!', '?', '_', '#', '\'', '"', '/', '\\', '<', '-', ':', '(', ')'}
var a2_v2_default = [...]uint8{'\n', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '.', ',', '!', '?', '_', '#', '\'', '"', '/', '\\', '-', ':', '(', ')'}

type Alphabet int

const (
	a0 Alphabet = 0
	a1 Alphabet = 1
	a2 Alphabet = 2
)

func (z *ZMachine) readZString(ptr uint16) (string, uint16) {
	bytesRead := uint16(0)
	baseAlphabet := a0
	currentAlphabet := a0
	nextAlphabet := a0

	var zchrStream []uint8
	var chrStream []uint8

	// First convert the memory addresses into a stream of 5 bit z characters
	// terminating at the appropriate time.
	// Note this could be an infinite loop in a poorly constructed story file - TODO
	for {
		halfWord := z.readHalfWord(ptr + bytesRead)
		bytesRead += 2
		isLastHalfWord := (halfWord >> 15) == 1

		zchrStream = append(zchrStream, uint8((halfWord>>10)&0b11111))
		zchrStream = append(zchrStream, uint8((halfWord>>5)&0b11111))
		zchrStream = append(zchrStream, uint8(halfWord&0b11111))

		if isLastHalfWord {
			break
		}
	}

	for i := 0; i < len(zchrStream); i++ {
		zchr := zchrStream[i]
		var chr uint8

		currentAlphabet = nextAlphabet
		nextAlphabet = baseAlphabet

		switch zchr {
		case 0: // SPACE in all versions
			chr = ' '
		case 1: // new line in v1, abbreviations in v2+
			if z.version() == 1 {
				chr = '\n'
			} else {
				panic("TODO - Abbreviations not handled")
			}
		case 2: // Shift 1 in v1-2, abbreviations in v3+
			if z.version() >= 3 {
				panic("TODO - Abbreviations not handled")
			} else {
				nextAlphabet = (nextAlphabet + 1) % 3
			}
		case 3: // Shift 2 in v1-2, abbreviations in v3+
			if z.version() >= 3 {
				panic("TODO - Abbreviations not handled")
			} else {
				nextAlphabet = (nextAlphabet + 2) % 3
			}
		case 4: // Shift-lock 1 in v1-2, shift 1 in v3+
			if z.version() >= 3 {
				nextAlphabet = (nextAlphabet + 1) % 3
			} else {
				baseAlphabet = (baseAlphabet + 1) % 3
				nextAlphabet = baseAlphabet
			}
		case 5: // Shift-lock 2 in v1-2, shift 1 in v3+
			if z.version() >= 3 {
				nextAlphabet = (nextAlphabet + 2) % 3
			} else {
				baseAlphabet = (baseAlphabet + 2) % 3
				nextAlphabet = baseAlphabet
			}
		default:
			// Escape code 6 on alphabet 2 means "ZSCII character" but in practice only 8 bit chars are valid so we can get away
			// with casting down to uint8 here. Maybe not strictly accurate and would be worth revisiting - TODO
			if currentAlphabet == 2 && zchr == 6 {
				chr = uint8(zchrStream[i+1]<<5 | zchrStream[i+2])
			} else {
				switch currentAlphabet {
				case a0:
					if z.version() <= 4 {
						chr = a0_default[zchr-6]
					} else {
						panic("TODO - Handle custom alphabets")
					}
				case a1:
					if z.version() <= 4 {
						chr = a1_default[zchr-6]
					} else {
						panic("TODO - Handle custom alphabets")
					}
				case a2:
					if z.version() == 1 {
						chr = a2_v1[zchr-7]
					} else if z.version() <= 4 {
						chr = a2_v2_default[zchr-7]
					} else {
						panic("TODO - Handle custom alphabets")
					}
				}
			}
		}

		chrStream = append(chrStream, chr)
	}

	return string(chrStream), bytesRead
}
