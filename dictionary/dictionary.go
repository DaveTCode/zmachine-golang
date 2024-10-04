package dictionary

import (
	"encoding/binary"

	"github.com/davetcode/goz/zstring"
)

type DictionaryHeader struct {
	n          uint8
	InputCodes []uint8
	length     uint8
	count      int16
}

type DictionaryEntry struct {
	address     uint16
	encodedWord []uint8
	decodedWord string
	data        []uint8
}

type Dictionary struct {
	Header  DictionaryHeader
	entries []DictionaryEntry
}

func ParseDictionary(bytes []uint8, baseAddress uint16, version uint8) *Dictionary {
	dictionaryPtr := uint16(0)
	numInputCodes := bytes[dictionaryPtr]

	header := DictionaryHeader{
		n:          numInputCodes,
		InputCodes: bytes[dictionaryPtr+1 : dictionaryPtr+uint16(numInputCodes)+1],
		length:     bytes[(dictionaryPtr + 1 + uint16(numInputCodes))],
		count:      int16(binary.BigEndian.Uint16(bytes[dictionaryPtr+2+uint16(numInputCodes) : dictionaryPtr+4+uint16(numInputCodes)])),
	}

	entryPtr := dictionaryPtr + 4 + uint16(numInputCodes)
	var entries = make([]DictionaryEntry, header.count)

	encodedWordLength := 4
	if version > 3 {
		encodedWordLength = 6
	}

	for ix := 0; ix < int(header.count); ix++ {
		encodedWord := bytes[entryPtr : entryPtr+uint16(encodedWordLength)+1]
		decodedWord, _ := zstring.ReadZString(bytes[entryPtr:], version)
		entries[ix] = DictionaryEntry{
			address:     entryPtr + baseAddress,
			encodedWord: encodedWord,
			decodedWord: decodedWord,
			data:        bytes[entryPtr+uint16(encodedWordLength) : entryPtr+uint16(header.length)+1],
		}

		entryPtr += uint16(header.length)
	}

	return &Dictionary{
		Header:  header,
		entries: entries,
	}
}

func (d *Dictionary) Find(zstr string) uint16 {
	for _, entry := range d.entries {
		if entry.decodedWord == zstr {
			return entry.address
		}
	}

	return 0
}
