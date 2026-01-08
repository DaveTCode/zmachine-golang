package zstring

import "github.com/davetcode/goz/zcore"

var DefaultUnicodeTranslationTable = map[rune]uint8{
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

func unicodeToZscii(r rune, core *zcore.Core) (uint8, bool) {
	unicodeTranslationTable := DefaultUnicodeTranslationTable
	if core.UnicodeExtensionTableBaseAddress != 0 {
		unicodeTranslationTable = parseUnicodeTranslationTable(core)
	}
	zchr, ok := unicodeTranslationTable[r]

	return zchr, ok
}

func ZsciiToUnicode(zchr uint8, core *zcore.Core) (rune, bool) {
	unicodeTranslationTable := DefaultUnicodeTranslationTable
	if core.UnicodeExtensionTableBaseAddress != 0 {
		unicodeTranslationTable = parseUnicodeTranslationTable(core)
	}
	for r, ix := range unicodeTranslationTable {
		if ix == zchr {
			return r, true
		}
	}

	// in theory someone could use zscii codes that were in the normal translation table
	for r, ix := range coreUnicodeTranslationTable {
		if ix == zchr {
			return r, true
		}
	}

	return 0, false
}

func parseUnicodeTranslationTable(core *zcore.Core) map[rune]uint8 {
	var result = make(map[rune]uint8, 0)

	numUnicodeExtensions := core.ReadZByte(uint32(core.UnicodeExtensionTableBaseAddress))
	startAddress := int(core.UnicodeExtensionTableBaseAddress + 1)
	for i := 0; i < int(numUnicodeExtensions); i++ {
		result[rune(core.ReadHalfWord(uint32(i*2+startAddress)))] = uint8(i + 155)
	}

	return result
}
