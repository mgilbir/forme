package brotli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// FuzzDecode. A font arrives from the network and this is the first thing that
// looks at it, so the properties worth stating are the ones that hold for
// arbitrary bytes rather than for streams: it finishes, it stays inside the
// limit it was given, and it gives the same answer twice.
//
// What it deliberately does not assert is that a stream decodes. Most inputs
// are not streams and being refused is the right answer for them; the check
// that real streams come out right is the reference comparison in brotli_test.go.
func FuzzDecode(f *testing.F) {
	files, err := filepath.Glob(filepath.Join("testdata", "*.br"))
	if err != nil {
		f.Fatal(err)
	}
	for _, name := range files {
		b, err := os.ReadFile(name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	// The hand-built shapes no compressor emits, so that the stored and
	// metadata paths are seeded too.
	w := &bitWriter{}
	w.write(0, 1)
	w.metadataBlock([]byte("skip me"), 0)
	w.storedBlock([]byte(storedText), 0)
	w.end()
	f.Add(w.out)
	// The stream that used to spin: legal, and every command it holds produces
	// nothing. See TestAStreamThatProducesNothingForEverIsRefused.
	w = &bitWriter{}
	w.write(0, 1)
	w.write(0, 1)
	w.write(0, 2)
	w.write(65535, 16)
	w.write(0, 1)
	for i := 0; i < 3; i++ {
		w.write(0, 1)
	}
	w.write(0, 6)
	w.write(0, 2)
	w.write(0, 1)
	w.write(0, 1)
	for _, sym := range []struct {
		v     uint32
		width uint
	}{{'A', 8}, {130, 10}, {44, 6}} {
		w.write(1, 2)
		w.write(0, 2)
		w.write(sym.v, sym.width)
	}
	f.Add(w.out)
	f.Add([]byte(nil))

	const limit = 1 << 20
	f.Fuzz(func(t *testing.T, src []byte) {
		got, err := Decode(src, limit)
		if len(got) > limit {
			t.Fatalf("%d bytes past a limit of %d", len(got), limit)
		}
		if err != nil {
			if len(got) != 0 {
				t.Fatalf("refused with %v and returned %d bytes", err, len(got))
			}
			return
		}
		// The same bytes twice: nothing here may depend on anything but the
		// input, because a font decoded differently on a second page is a
		// document that changes as it is printed.
		again, err := Decode(src, limit)
		if err != nil {
			t.Fatalf("decoded once and then failed with %v", err)
		}
		if !bytes.Equal(got, again) {
			t.Fatalf("decoding twice gave %d and %d bytes", len(got), len(again))
		}
		// The limit is a ceiling and not a margin: exactly enough is enough,
		// and one short is not.
		exact, err := Decode(src, len(got))
		if err != nil {
			t.Fatalf("a limit of exactly %d bytes: %v", len(got), err)
		}
		if !bytes.Equal(exact, got) {
			t.Fatalf("an exact limit changed the output")
		}
		if len(got) > 0 {
			if short, err := Decode(src, len(got)-1); err == nil {
				t.Fatalf("a limit of %d let %d bytes through", len(got)-1, len(short))
			}
		}
	})
}
