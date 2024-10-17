package ztable

import (
	"encoding/binary"
	"strings"
)

func PrintTable(memory []uint8, baddr uint32, width uint16, height uint16, skip uint16) string {
	numBytes := memory[baddr]
	s := strings.Builder{}

	for i := uint16(0); i < uint16(numBytes); i++ {
		row := i / width
		col := i % width

		if col == 0 {
			if row != 0 {
				s.WriteByte('\n')

				if row == height {
					break
				}
			}
		}

		s.WriteByte(memory[baddr+uint32(i)+uint32(skip*row)])
	}

	return s.String()
}

func ScanTable(memory []uint8, test uint16, baddr uint32, length uint16, form uint16) uint32 {
	ptr := baddr
	fieldSize := form & 0b0111_1111
	checkWord := form&0b1000_0000 == 0b1000_0000
	if fieldSize == 0 {
		return 0 // Can't have 0 field length in a table - not 100% sure if this is right but probably better than infinite loop
	}

	for i := uint16(0); i < length; i++ {
		if !checkWord {
			if uint16(memory[ptr]) == test { // Note the scaling up of the memory value here to u16 is because the test value can be larger and that should rightly not be found
				return ptr
			}
		} else {
			if binary.BigEndian.Uint16(memory[ptr:ptr+2]) == test {
				return ptr
			}
		}

		ptr += uint32(fieldSize)
	}

	return 0
}

func CopyTable(memory []uint8, first uint16, second uint16, size int16) {
	sizeAbs := uint16(size)
	if size < 0 {
		sizeAbs = uint16(-1 * size)
	}

	switch {
	case second == 0: // special case used to zero a table
		for i := uint16(0); i < sizeAbs; i++ {
			memory[first+i] = 0
		}

	case size >= 0: // Use original values of first table don't allow mid-copy corruption
		tmp := make([]uint8, size)
		copy(tmp, memory[first:first+sizeAbs])
		copy(memory[second:second+sizeAbs], tmp)

	case size < 0: // Allow corruption of existing table as copy occurs
		for i := uint16(0); i < sizeAbs; i++ {
			memory[second+i] = memory[first+i]
		}
	}
}
