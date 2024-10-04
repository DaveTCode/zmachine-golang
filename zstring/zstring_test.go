package zstring

import (
	"testing"
)

func TestZStringCreation(t *testing.T) {
	bytes := []uint8{11, 45, 42, 234, 1, 216, 0, 192, 98, 70, 70, 32, 72, 206, 68, 244, 116, 13, 42, 234, 142, 37, 11, 45, 42, 234, 1, 216}
	expectedString := "There is a small mailbox here."
	expectedBytesRead := uint16(22)
	zstr, bytesRead := ReadZString(bytes, 1)
	if expectedString != zstr {
		t.Fatalf(`zstr read incorrectly expected=%s, actual=%s`, expectedString, zstr)
	}
	if expectedBytesRead != bytesRead {
		t.Fatalf(`zstr read incorrect number of bytes expected=%d, actual=%d`, expectedBytesRead, bytesRead)
	}
}
