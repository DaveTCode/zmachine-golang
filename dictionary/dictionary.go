package dictionary

import (
	"bytes"
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

func ParseDictionary(bytes []uint8, baseAddress uint32, version uint8, alphabets *zstring.Alphabets, abbreviationBase uint16) *Dictionary {
	dictionaryPtr := uint32(0)
	numInputCodes := bytes[dictionaryPtr]

	header := DictionaryHeader{
		n:          numInputCodes,
		InputCodes: bytes[dictionaryPtr+1 : dictionaryPtr+uint32(numInputCodes)+1],
		length:     bytes[(dictionaryPtr + 1 + uint32(numInputCodes))],
		count:      int16(binary.BigEndian.Uint16(bytes[dictionaryPtr+2+uint32(numInputCodes) : dictionaryPtr+4+uint32(numInputCodes)])),
	}

	entryPtr := dictionaryPtr + 4 + uint32(numInputCodes)
	var entries = make([]DictionaryEntry, header.count)

	encodedWordLength := 4
	if version > 3 {
		encodedWordLength = 6
	}

	for ix := 0; ix < int(header.count); ix++ {
		encodedWord := bytes[entryPtr : entryPtr+uint32(encodedWordLength)]
		decodedWord, _ := zstring.Decode(bytes, entryPtr, version, alphabets, abbreviationBase)
		entries[ix] = DictionaryEntry{
			address:     uint16(entryPtr + baseAddress),
			encodedWord: encodedWord,
			decodedWord: decodedWord,
			data:        bytes[entryPtr+uint32(encodedWordLength) : entryPtr+uint32(header.length)],
		}

		entryPtr += uint32(header.length)
	}

	return &Dictionary{
		Header:  header,
		entries: entries,
	}
}

func (d *Dictionary) Find(zstr []uint8) uint16 {
	for _, entry := range d.entries {
		if bytes.Equal(entry.encodedWord, zstr) {
			return entry.address
		}
	}

	return 0
}
