package zstring

import (
	"encoding/binary"
	"slices"
)

type Alphabets struct {
	a0 []rune
	a1 []rune
	a2 []rune
}

var defaultAlphabetsV1 = Alphabets{
	a0: []rune{'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z'},
	a1: []rune{'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z'},
	a2: []rune{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '.', ',', '!', '?', '_', '#', '\'', '"', '/', '\\', '<', '-', ':', '(', ')'},
}

var defaultAlphabetsV2 = Alphabets{
	a0: []rune{'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z'},
	a1: []rune{'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z'},
	a2: []rune{'\n', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '.', ',', '!', '?', '_', '#', '\'', '"', '/', '\\', '-', ':', '(', ')'},
}

func LoadAlphabets(version uint8, memory []uint8, customAlphabetBase uint16) *Alphabets {
	if version == 1 {
		return &defaultAlphabetsV1
	} else if version < 5 {
		return &defaultAlphabetsV2
	} else if customAlphabetBase == 0 {
		return &defaultAlphabetsV2
	} else {
		panic("TODO - Load custom alphabet")
	}
}

var coreUnicodeTranslationTable = map[rune]uint8{
	'!':  0x21,
	'"':  0x22,
	'#':  0x23,
	'$':  0x24,
	'%':  0x25,
	'&':  0x26,
	'\'': 0x27,
	'(':  0x28,
	')':  0x29,
	'*':  0x2a,
	'+':  0x2b,
	',':  0x2c,
	'-':  0x2d,
	'.':  0x2e,
	'/':  0x2f,
	'0':  0x30,
	'1':  0x31,
	'2':  0x32,
	'3':  0x33,
	'4':  0x34,
	'5':  0x35,
	'6':  0x36,
	'7':  0x37,
	'8':  0x38,
	'9':  0x39,
	':':  0x3a,
	';':  0x3b,
	'<':  0x3c,
	'=':  0x3d,
	'>':  0x3e,
	'?':  0x3f,
	'@':  0x40,
	'A':  0x41,
	'B':  0x42,
	'C':  0x43,
	'D':  0x44,
	'E':  0x45,
	'F':  0x46,
	'G':  0x47,
	'H':  0x48,
	'I':  0x49,
	'J':  0x4a,
	'K':  0x4b,
	'L':  0x4c,
	'M':  0x4d,
	'N':  0x4e,
	'O':  0x4f,
	'P':  0x50,
	'Q':  0x51,
	'R':  0x52,
	'S':  0x53,
	'T':  0x54,
	'U':  0x55,
	'V':  0x56,
	'W':  0x57,
	'X':  0x58,
	'Y':  0x59,
	'Z':  0x5a,
	'[':  0x5b,
	'\\': 0x5c,
	']':  0x5d,
	'^':  0x5e,
	'_':  0x5f,
	'`':  0x60, // TODO - Suggestion this might be wrong in some places
	'a':  0x61,
	'b':  0x62,
	'c':  0x63,
	'd':  0x64,
	'e':  0x65,
	'f':  0x66,
	'g':  0x67,
	'h':  0x68,
	'i':  0x69,
	'j':  0x6a,
	'k':  0x6b,
	'l':  0x6c,
	'm':  0x6d,
	'n':  0x6e,
	'o':  0x6f,
	'p':  0x70,
	'q':  0x71,
	'r':  0x72,
	's':  0x73,
	't':  0x74,
	'u':  0x75,
	'v':  0x76,
	'w':  0x77,
	'x':  0x78,
	'y':  0x79,
	'z':  0x7a,
	'{':  0x7b,
	'|':  0x7c,
	'}':  0x7d,
	'~':  0x7e,
}
var defaultUnicodeTranslationTable = map[rune]uint8{
	'ä': 155,
	'ö': 156,
	'ü': 157,
	'Ä': 158,
	'Ö': 159,
	'Ü': 160,
	'ß': 161,
	'»': 162,
	'«': 163,
	'ë': 164,
	'ï': 165,
	'ÿ': 166,
	'Ë': 167,
	'Ï': 168,
	'á': 169,
	'é': 170,
	'í': 171,
	'ó': 172,
	'ú': 173,
	'ý': 174,
	'Á': 175,
	'É': 176,
	'Í': 177,
	'Ó': 178,
	'Ú': 179,
	'Ý': 180,
	'à': 181,
	'è': 182,
	'ì': 183,
	'ò': 184,
	'ù': 185,
	'À': 186,
	'È': 187,
	'Ì': 188,
	'Ò': 189,
	'Ù': 190,
	'â': 191,
	'ê': 192,
	'î': 193,
	'ô': 194,
	'û': 195,
	'Â': 196,
	'Ê': 197,
	'Î': 198,
	'Ô': 199,
	'Û': 200,
	'å': 201,
	'Å': 202,
	'ø': 203,
	'Ø': 204,
	'ã': 205,
	'ñ': 206,
	'õ': 207,
	'Ã': 208,
	'Ñ': 209,
	'Õ': 210,
	'æ': 211,
	'Æ': 212,
	'ç': 213,
	'Ç': 214,
	'þ': 215,
	'ð': 216,
	'Þ': 217,
	'Ð': 218,
	'£': 219,
	'œ': 220,
	'Œ': 221,
	'¡': 222,
	'¿': 223,
}

type Alphabet int

const (
	a0 Alphabet = 0
	a1 Alphabet = 1
	a2 Alphabet = 2
)

// Encode takes a stream of bytes which are assumed to be in UTF-8 unicode and
// translates them to a z-string.
// In theory this should be the inverse of the zstring.Decode function although
// in practice strings can be constructed for which this isn't true
func Encode(s []rune, version uint8, alphabets *Alphabets) []uint8 {
	zchrs := make([]uint8, 0)

	// The version decides how many zchrs are allowed, we must pad and truncate to get exactly this value
	numZChrs := 6
	if version > 3 {
		numZChrs = 9
	}

	// TODO - I don't bother encoding using shift lock characters on V1-2 here, not 100% sure when they were used
	shiftA1 := uint8(2)
	shiftA2 := uint8(3)
	if version > 2 {
		shiftA1 = 4
		shiftA2 = 5
	}

	for _, chr := range s {
		if chr == ' ' { // SPACE is 0 in all versions, don't need to check alphabets
			zchrs = append(zchrs, 0)
			continue
		}

		if slices.Contains(alphabets.a0, chr) {
			zchrs = append(zchrs, 6+uint8(slices.Index(alphabets.a0, chr)))
		} else if slices.Contains(alphabets.a1, chr) {
			zchrs = append(zchrs, shiftA1)
			zchrs = append(zchrs, 6+uint8(slices.Index(alphabets.a1, chr)))
		} else if slices.Contains(alphabets.a2, chr) {
			zchrs = append(zchrs, shiftA2)
			zchrs = append(zchrs, 7+uint8(slices.Index(alphabets.a2, chr)))
		} else {
			// ZSCII character or invalid
			zchrs = append(zchrs, shiftA2)
			zchrs = append(zchrs, 6)

			if zchr, ok := coreUnicodeTranslationTable[chr]; ok {
				zchrs = append(zchrs, zchr>>5)
				zchrs = append(zchrs, zchr&0b1_1111)
			} else {
				// if version >= 5 {
				// 	// TODO - Handle passing through a custom unicode translation table on V5 if one is set in the story file
				// 	panic("We don't handle custom unicode dictionaries yet")
				// }
				if zchr, ok := defaultUnicodeTranslationTable[chr]; ok {
					zchrs = append(zchrs, zchr>>5)
					zchrs = append(zchrs, zchr&0b1_1111)
				}
			}
		}
	}

	// Pad the string with 5s to ensure exactly 2 byte chunks
	for {
		if len(zchrs)%3 != 0 || len(zchrs) < numZChrs {
			zchrs = append(zchrs, 5)
		} else {
			break
		}
	}

	// Truncate to match fixed length
	zchrs = zchrs[0:numZChrs]

	bytes := make([]uint8, 0)
	chunks := slices.Collect(slices.Chunk(zchrs, 3))
	for ix, chunk := range chunks {
		u16 := (uint16(chunk[2]) & 0b1_1111) | (uint16(chunk[1]&0b1_1111) << 5) | (uint16(chunk[0]&0b1_1111) << 10)
		if len(chunks) == ix+1 {
			u16 |= 0b1000_0000_0000_0000
		}

		bytes = append(bytes, uint8(u16>>8))
		bytes = append(bytes, uint8(u16))
	}

	return bytes
}

func Decode(bytes []uint8, startPtr uint32, version uint8, alphabets *Alphabets, AbbreviationTableBase uint16) (string, uint32) {
	bytesRead := uint32(0)
	ptr := startPtr
	baseAlphabet := a0
	currentAlphabet := a0
	nextAlphabet := a0

	var zchrStream []uint8
	var chrStream []uint8

	// First convert the memory addresses into a stream of 5 bit z characters
	// terminating at the appropriate time.
	for {
		halfWord := binary.BigEndian.Uint16(bytes[ptr : ptr+2])
		bytesRead += 2
		ptr += 2
		isLastHalfWord := (halfWord >> 15) == 1

		zchrStream = append(zchrStream, uint8((halfWord>>10)&0b11111))
		zchrStream = append(zchrStream, uint8((halfWord>>5)&0b11111))
		zchrStream = append(zchrStream, uint8(halfWord&0b11111))

		if isLastHalfWord || ptr >= uint32(len(bytes)-1) {
			break
		}
	}

	for i := 0; i < len(zchrStream); i++ {
		zchr := zchrStream[i]
		currentAlphabet = nextAlphabet
		nextAlphabet = baseAlphabet

		switch zchr {
		case 0: // SPACE in all versions
			chrStream = append(chrStream, ' ')
		case 1: // new line in v1, abbreviations in v2+
			if version == 1 {
				chrStream = append(chrStream, '\n')
			} else {
				abbr := FindAbbreviation(version, AbbreviationTableBase, bytes, alphabets, zchr, zchrStream[i+1])
				chrStream = append(chrStream, abbr...)
			}
		case 2: // Shift 1 in v1-2, abbreviations in v3+
			if version >= 3 {
				abbr := FindAbbreviation(version, AbbreviationTableBase, bytes, alphabets, zchr, zchrStream[i+1])
				chrStream = append(chrStream, abbr...)
			} else {
				nextAlphabet = (nextAlphabet + 1) % 3
			}
		case 3: // Shift 2 in v1-2, abbreviations in v3+
			if version >= 3 {
				abbr := FindAbbreviation(version, AbbreviationTableBase, bytes, alphabets, zchr, zchrStream[i+1])
				chrStream = append(chrStream, abbr...)
			} else {
				nextAlphabet = (nextAlphabet + 2) % 3
			}
		case 4: // Shift-lock 1 in v1-2, shift 1 in v3+
			if version >= 3 {
				nextAlphabet = (nextAlphabet + 1) % 3
			} else {
				baseAlphabet = (baseAlphabet + 1) % 3
				nextAlphabet = baseAlphabet
			}
		case 5: // Shift-lock 2 in v1-2, shift 1 in v3+
			if version >= 3 {
				nextAlphabet = (nextAlphabet + 2) % 3
			} else {
				baseAlphabet = (baseAlphabet + 2) % 3
				nextAlphabet = baseAlphabet
			}
		default:
			// Escape code 6 on alphabet 2 means "ZSCII character" but in practice only 8 bit chars are valid so we can get away
			// with casting down to uint8 here. Maybe not strictly accurate and would be worth revisiting - TODO
			if currentAlphabet == 2 && zchr == 6 {
				chrStream = append(chrStream, zchrStream[i+1]<<5|zchrStream[i+2])
				i += 2
			} else {
				switch currentAlphabet {
				case a0:
					chrStream = append(chrStream, uint8(alphabets.a0[zchr-6]))
				case a1:
					chrStream = append(chrStream, uint8(alphabets.a1[zchr-6]))
				case a2:
					chrStream = append(chrStream, uint8(alphabets.a2[zchr-7]))
				}
			}
		}
	}

	return string(chrStream), bytesRead
}
