package font

import (
	"encoding/binary"
	"strconv"
	"testing"
)

// TestOff32 (#989): where int is 64 bits Off32 is int(Be32); where it is 32
// bits a value past MaxProgram32 is read as maxOff32, which is past every
// table that platform reads and positive however a few of it are summed.
func TestOff32(t *testing.T) {
	for _, v := range []uint32{0, 1, MaxProgram32, MaxProgram32 + 1, maxOff32, 1<<31 - 1, 1 << 31, 0xFFFFFFFF} {
		b := binary.BigEndian.AppendUint32(nil, v)
		got := Off32(b, 0)
		want := int(uint64(v)) // what a 64-bit int holds
		if strconv.IntSize == 32 && v > maxOff32 {
			want = maxOff32
		}
		if got != want {
			t.Errorf("Off32 of %#x is %d, want %d", v, got, want)
		}
		if got < 0 || got+got+got+MaxProgram32 < 0 {
			t.Errorf("Off32 of %#x is %d, which three of and a table overflow", v, got)
		}
	}
	if Off32([]byte{1, 2, 3}, 0) != 0 {
		t.Error("Off32 past the end of its bytes is not 0, as Be32's is")
	}
}
