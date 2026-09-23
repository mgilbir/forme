package notosans

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

// TestRegularHandsOutACopy. Regular was the embedded array, which is the
// array every face this package hands out was loaded from. A caller writing
// into what it was given — patching a table, zeroing a checksum — changed the
// font under all of them.
func TestRegularHandsOutACopy(t *testing.T) {
	before := sha256.Sum256(notoSansRegular)
	got := Regular()
	if !bytes.Equal(got, notoSansRegular) {
		t.Fatal("Regular is not the bundled font")
	}
	for i := range got[:1024] {
		got[i] ^= 0xFF
	}
	if sha256.Sum256(notoSansRegular) != before {
		t.Fatal("writing into what Regular returned changed the bundled font")
	}
	if again := Regular(); !bytes.Equal(again, notoSansRegular) {
		t.Error("a second call does not return the font as it was")
	}
	face, err := Face()
	if err != nil {
		t.Fatalf("the bundled font no longer loads: %v", err)
	}
	if face.NumGlyphs() == 0 {
		t.Error("the face has no glyphs")
	}
}
