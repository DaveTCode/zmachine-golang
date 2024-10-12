package zstring

import (
	"bytes"
	"testing"
)

var zstringDecodingTests = []struct {
	in        []uint8
	out       string
	bytesRead uint32
	version   uint8
}{
	{[]uint8{11, 45, 42, 234, 1, 216, 0, 192, 98, 70, 70, 32, 72, 206, 68, 244, 116, 13, 42, 234, 142, 37, 11, 45, 42, 234, 1, 216}, "There is a small mailbox here.", 22, 1},
	{[]uint8{12, 193, 248, 165}, ">", 4, 1},
}

var zstringEncodingTests = []struct {
	in      string
	out     []uint8
	version uint8
}{
	{">", []uint8{12, 193, 248, 165}, 1},      // zscii test
	{"i", []uint8{0x48, 0xa5, 0x14, 0xa5}, 1}, // test padding to 6
}

func TestZStringDecoding(t *testing.T) {
	for _, tt := range zstringDecodingTests {
		t.Run(string(tt.out), func(t *testing.T) {
			zstr, bytesRead := Decode(tt.in, tt.version, &defaultAlphabetsV1)

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
