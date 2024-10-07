package zstring

import (
	"testing"
)

var zstringTests = []struct {
	in        []uint8
	out       string
	bytesRead uint32
	version   uint8
}{
	{[]uint8{11, 45, 42, 234, 1, 216, 0, 192, 98, 70, 70, 32, 72, 206, 68, 244, 116, 13, 42, 234, 142, 37, 11, 45, 42, 234, 1, 216}, "There is a small mailbox here.", 22, 1},
	{[]uint8{12, 193, 248, 165}, ">", 4, 1},
}

func TestZStringDecoding(t *testing.T) {
	for _, tt := range zstringTests {
		t.Run(string(tt.out), func(t *testing.T) {
			zstr, bytesRead := ReadZString(tt.in, tt.version)

			if tt.out != zstr {
				t.Fatalf(`zstr read incorrectly expected=%s, actual=%s`, tt.out, zstr)
			}
			if tt.bytesRead != bytesRead {
				t.Fatalf(`zstr read incorrect number of bytes expected=%d, actual=%d`, tt.bytesRead, bytesRead)
			}
		})
	}
}
