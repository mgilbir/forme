package font

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// TestAWOFFCannotDeclareMoreTablesThanAnSfntCanAddress is the bound between two
// bytes of a font and an allocation sized by them.
//
// The reader's own comment says what stands where: "numTables is two
// attacker-controlled bytes and this is the only thing standing between them and
// a 4096-entry allocation." Nothing had ever written those two bytes: every WOFF
// fixture in this package declares as many tables as it carries, so the font
// suite passes with maxWOFFTables raised past what the field can even hold.
//
// The file is padded past the directory the count describes on purpose. A small
// file declaring sixty thousand tables is refused by the check immediately
// below this one — the directory would run past the end — and a test that
// tripped that one would be asserting the wrong bound. Twenty bytes an entry is
// the directory's own stride, so the padding makes the count the only thing
// wrong with this font.
func TestAWOFFCannotDeclareMoreTablesThanAnSfntCanAddress(t *testing.T) {
	const stated = 60000
	n := uint16(stated)
	data := fonttest.WOFF(fonttest.WOFFOptions{
		Tables:          []fonttest.WOFFTable{{Tag: "cmap", Data: []byte("cmap")}},
		StatedNumTables: &n,
		Padding:         20 * stated,
	})

	_, err := DecodeWOFF(data)
	if err == nil {
		t.Fatalf("a WOFF declaring %d tables was decoded; two bytes of header must "+
			"not size the walk that follows them", stated)
	}
	if !strings.Contains(err.Error(), "more tables than an sfnt can address") {
		t.Errorf("it was refused with %q; the refusal that belongs here is the bound "+
			"on the count, not a later check that happens to catch it", err)
	}
}
