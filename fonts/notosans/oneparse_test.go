package notosans

import (
	"runtime"
	"testing"
)

// The bundled face is two megabytes, and reading it costs about 16 ms and 9.6 MB.
//
// Face() has been sharing one parse since the day that was measured, handing
// each caller a Clone; Simple() read the bytes again on every call, so a program
// writing a hundred documents parsed the same font a hundred times. Nothing
// about the parse depends on the document — what does is the record of which
// glyphs it used, which is what the clone carries.

// bytesPerCall is how much a function allocates, averaged over enough calls that
// the first one's share is small.
func bytesPerCall(t *testing.T, n int, f func()) uint64 {
	t.Helper()
	f() // the first read, which is the one that is shared
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := 0; i < n; i++ {
		f()
	}
	runtime.ReadMemStats(&after)
	return (after.TotalAlloc - before.TotalAlloc) / uint64(n)
}

// TestTheBundledFaceIsReadOnce.
func TestTheBundledFaceIsReadOnce(t *testing.T) {
	for _, tc := range []struct {
		name string
		get  func() (interface{ NumGlyphs() int }, error)
	}{
		{"Simple", func() (interface{ NumGlyphs() int }, error) { return Simple() }},
		{"Face", func() (interface{ NumGlyphs() int }, error) { return Face() }},
	} {
		var err error
		got := bytesPerCall(t, 20, func() {
			var f interface{ NumGlyphs() int }
			f, err = tc.get()
			if f == nil || f.NumGlyphs() == 0 {
				t.Fatalf("%s returned nothing", tc.name)
			}
		})
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		// A clone is thousands of bytes; a parse is millions. One megabyte is
		// far above the one and far below the other.
		if got > 1<<20 {
			t.Errorf("%s allocates %d bytes a call, which is a parse of the "+
				"two-megabyte face rather than a clone of it", tc.name, got)
		}
	}
}
