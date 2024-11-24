package zstring

import (
	"github.com/davetcode/goz/zcore"
)

func FindAbbreviation(core *zcore.Core, alphabets *Alphabets, z uint8, x uint8) string {
	abbrIx := 32*(z-1) + x
	addr := uint32(core.AbbreviationTableBase + 2*uint16(abbrIx))
	strAddr := 2 * core.ReadHalfWord(addr)

	str, _ := Decode(uint32(strAddr), uint32(core.FileLength()), core, alphabets, true)

	return str
}
