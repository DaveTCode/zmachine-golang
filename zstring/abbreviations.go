package zstring

import "encoding/binary"

func FindAbbreviation(version uint8, AbbreviationTableBase uint16, memory []uint8, alphabets *Alphabets, z uint8, x uint8) string {
	abbrIx := 32*(z-1) + x
	addr := uint32(AbbreviationTableBase + 2*uint16(abbrIx))
	strAddr := 2 * binary.BigEndian.Uint16(memory[addr:addr+2])

	str, _ := Decode(memory, uint32(strAddr), version, alphabets, AbbreviationTableBase)

	return str
}
