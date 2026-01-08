package zstring

import (
	"bytes"
	"os"
	"testing"

	"github.com/davetcode/goz/zcore"
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
	minimalBytes := make([]uint8, 256)
	core := zcore.LoadCore(minimalBytes)

	for _, tt := range zstringDecodingTests {
		t.Run(string(tt.out), func(t *testing.T) {
			core.Version = tt.version
			for i, b := range tt.in {
				core.WriteZByte(uint32(i), b)
			}
			zstr, bytesRead := Decode(0, uint32(len(tt.in)), &core, &defaultAlphabetsV1, false)

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
	minimalBytes := make([]uint8, 256)
	core := zcore.LoadCore(minimalBytes)

	for _, tt := range zstringEncodingTests {
		t.Run(string(tt.out), func(t *testing.T) {
			zstr := Encode([]rune(tt.in), &core, &defaultAlphabetsV1)

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

	core := zcore.LoadCore(storyFileBytes)

	str, _ := Decode(0x44ef, 0x5000, &core, LoadAlphabets(&core), false)

	if str != "Welcome to Adventure! Do you need instructions?" {
		t.Fatalf("Invalid welcome string: %s", str)
	}
}

func TestV5PartialConstruction(t *testing.T) {
	// TODO - Test case where a string has a partial construction which should get ignored
}
