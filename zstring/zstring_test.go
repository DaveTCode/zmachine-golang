package zstring

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

var zstringDecodingTests = []struct {
	in        []uint8
	out       string
	bytesRead uint32
	version   uint8
}{
	{[]uint8{11, 45, 42, 234, 1, 216, 0, 192, 98, 70, 70, 32, 72, 206, 68, 244, 116, 13, 42, 234, 142, 37, 11, 45, 42, 234, 1, 216}, "There is a small mailbox here.", 22, 1}, // normal string with all 3 alphabets
	{[]uint8{12, 193, 248, 165}, ">", 4, 1},             // zscii
	{[]uint8{26, 94, 23, 24, 148, 207}, "amy\"s", 6, 5}, // Partial construction
}

var zstringEncodingTests = []struct {
	in      string
	out     []uint8
	version uint8
}{
	{">", []uint8{12, 193, 248, 165}, 1}, // zscii test
}

func TestZStringDecoding(t *testing.T) {
	for _, tt := range zstringDecodingTests {
		t.Run(string(tt.out), func(t *testing.T) {
			zstr, bytesRead := Decode(tt.in, 0, tt.version, &defaultAlphabetsV1, 0)

			if tt.out != zstr {
				t.Fatalf(`zstr read incorrectly expected=%s, actual=%s`, tt.out, zstr)
			}
			if tt.bytesRead != bytesRead {
				t.Fatalf(`zstr read incorrect number of bytes expected=%d, actual=%d`, tt.bytesRead, bytesRead)
			}
		})
	}
}

func TestZStringEncoding(t *testing.T) {
	for _, tt := range zstringEncodingTests {
		t.Run(string(tt.out), func(t *testing.T) {
			zstr := Encode([]rune(tt.in), tt.version, &defaultAlphabetsV1)

			if !bytes.Equal(tt.out, zstr) {
				t.Fatalf(`zstr encoded incorrectly expected=%s, actual=%s`, tt.out, zstr)
			}
		})
	}
}

func TestV3Abbreviations(t *testing.T) {
	storyFileBytes, err := os.ReadFile("../advent.z3")
	if err != nil {
		panic("test story file missing")
	}

	str, _ := Decode(storyFileBytes, 0x44ef, 3, LoadAlphabets(3, storyFileBytes, 0), binary.BigEndian.Uint16(storyFileBytes[0x18:0x1a]))

	if str != "Welcome to Adventure! Do you need instructions?" {
		t.Fatalf("Invalid welcome string: %s", str)
	}
}

func TestV5PartialConstruction(t *testing.T) {

}
