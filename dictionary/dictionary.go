package dictionary

import (
	"bytes"

	"github.com/davetcode/goz/zcore"
	"github.com/davetcode/goz/zstring"
)

type Header struct {
	n          uint8
	InputCodes []uint8
	length     uint8
	count      int16
}

type Entry struct {
	address     uint16
	encodedWord []uint8
	decodedWord string
	data        []uint8
}

type Dictionary struct {
	Header  Header
	entries []Entry
}

func (d *Dictionary) GetWords() []string {
	var words = make([]string, len(d.entries))
	for i, entry := range d.entries {
		words[i] = entry.decodedWord
	}
	return words
}

func ParseDictionary(baseAddress uint32, core *zcore.Core, alphabets *zstring.Alphabets) *Dictionary {
	dictionaryPtr := baseAddress
	numInputCodes := core.ReadByte(dictionaryPtr)

	header := Header{
		n:          numInputCodes,
		InputCodes: core.ReadSlice(dictionaryPtr+1, dictionaryPtr+uint32(numInputCodes)+1),
		length:     core.ReadByte((dictionaryPtr + 1 + uint32(numInputCodes))),
		count:      int16(core.ReadHalfWord(dictionaryPtr + 2 + uint32(numInputCodes))),
	}

	entryPtr := dictionaryPtr + 4 + uint32(numInputCodes)
	var entries = make([]Entry, header.count)

	encodedWordLength := 4
	if core.Version > 3 {
		encodedWordLength = 6
	}

	for ix := 0; ix < int(header.count); ix++ {
		encodedWord := core.ReadSlice(entryPtr, entryPtr+uint32(encodedWordLength))
		decodedWord, _ := zstring.Decode(entryPtr, entryPtr+uint32(encodedWordLength), core, alphabets, false)
		entries[ix] = Entry{
			address:     uint16(entryPtr),
			encodedWord: encodedWord,
			decodedWord: decodedWord,
			data:        core.ReadSlice(entryPtr+uint32(encodedWordLength), entryPtr+uint32(header.length)),
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
