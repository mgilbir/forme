package font

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// TestAWOFF2CannotDeclareMoreThanThisEngineWillDecompress is the bound between
// a font a page links and an allocation the size of the machine.
//
// The size decompressed into is not measured, it is *declared*: the last
// table's offset plus its length, read from the font's own directory as numbers
// a font may write anything into. A file of a few hundred bytes can say its
// tables come to two gigabytes, and that figure is what brotli.Decode is handed
// as the limit it may reach — so without a bound the limit that is supposed to
// stop a decompression bomb is set by the bomb.
//
// Two checks stand behind this, and neither had ever been seen to hold: the
// directory reader refuses a running total past the cap as it reads each table,
// and the size handed to the decoder is checked again before it is. **The whole
// font suite passes with both of them deleted**, because every WOFF 2 in it
// declares a truthful size. This is the document that declares an untruthful
// one.
//
// Either check alone refuses this font, so this test sees the pair go rather
// than either one of them — which is worth stating because it means a plant
// against one of them passes. What it does see is the thing that matters: with
// both gone the font is still refused, but by the check that compares the
// decompressed length with the declared one, which runs *after* the decompression
// it was supposed to prevent. Refusing a bomb once it has been allocated is not
// refusing it, and that is why this asserts which refusal came back and not
// merely that one did.
func TestAWOFF2CannotDeclareMoreThanThisEngineWillDecompress(t *testing.T) {
	huge := uint32(2 << 30) // two gigabytes, from a file of a few hundred bytes
	data := fonttest.WOFF2(fonttest.WOFF2Options{
		Tables: []fonttest.WOFF2Table{
			{Tag: "cmap", Data: []byte("cmapcmapcmapcmap")},
			{Tag: "head", Data: []byte("headheadheadhead"), StatedOrigLength: &huge},
		},
	})
	if len(data) > 1<<16 {
		t.Fatalf("the fixture is %d bytes; it is meant to be small enough that "+
			"what it declares is the whole of the threat", len(data))
	}

	out, err := DecodeWOFF2(data)
	if err == nil {
		t.Fatalf("a WOFF 2 declaring %d bytes of tables was decoded into %d bytes; "+
			"the size a font declares is the size it is allowed to decompress to, "+
			"so it has to be bounded before it is believed", huge, len(out))
	}
	if !strings.Contains(err.Error(), "more than this engine will decompress") {
		t.Errorf("it was refused with %q; the refusal that belongs here is the "+
			"bound on what will be decompressed, not a later check that happens "+
			"to catch it", err)
	}
}
