package main

import (
	"fmt"
	"strings"
)

type DictionaryHeader struct {
	n          uint8
	inputCodes []uint8
	length     uint8
	count      int16
}

type DictionaryEntry struct {
	encodedWord []uint8
	decodedWord string
	data        []uint8
}

type Dictionary struct {
	header  DictionaryHeader
	entries []DictionaryEntry
}

func (z *ZMachine) parseDictionary() *Dictionary {
	dictionaryPtr := z.dictionaryBase()
	numInputCodes := z.readByte(dictionaryPtr)

	header := DictionaryHeader{
		n:          numInputCodes,
		inputCodes: z.memory[dictionaryPtr+1 : dictionaryPtr+uint16(numInputCodes)+1],
		length:     z.readByte(dictionaryPtr + 1 + uint16(numInputCodes)),
		count:      int16(z.readHalfWord(dictionaryPtr + 2 + uint16(numInputCodes))),
	}

	entryPtr := dictionaryPtr + 4 + uint16(numInputCodes)
	var entries = make([]DictionaryEntry, header.count)

	encodedWordLength := 4
	if z.version() > 3 {
		encodedWordLength = 6
	}

	for ix := 0; ix < int(header.count); ix++ {
		encodedWord := z.memory[entryPtr : entryPtr+uint16(encodedWordLength)+1]
		decodedWord, _ := z.readZString(entryPtr)
		entries[ix] = DictionaryEntry{
			encodedWord: encodedWord,
			decodedWord: decodedWord,
			data:        z.memory[entryPtr+uint16(encodedWordLength) : entryPtr+uint16(header.length)+1],
		}

		entryPtr += uint16(header.length)
	}

	return &Dictionary{
		header:  header,
		entries: entries,
	}
}

func (z *ZMachine) LexicalAnalysis(s string) {
	// By adding spaces around the word separators we can treat them as words
	// Spaces by constract don't get lexically analysed as words
	for _, code := range z.dictionary.header.inputCodes {
		s = strings.ReplaceAll(s, string(code), fmt.Sprintf(" %s ", string(code)))
	}

	splitFunc := func(c rune) bool {
		return c == ' '
	}

	// Use FieldsFunc not Split to ignore empty entries
	words := strings.FieldsFunc(s, splitFunc)

	for _, word := range words {
		word = word // TODO - Actually implement something here
	}
}
