package font

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// Which transform version a WOFF 2 table may name.
//
// §4.1 defines exactly two transforms. glyf and loca share one: it is version 0
// and it is their default, with version 3 saying "not transformed". hmtx has one
// at version 1, and version 0 says "not transformed" for it and for every other
// table. The remaining values "are reserved for future use and MUST NOT be
// used".
//
// They were read as one of the two anyway. The rule was "glyf and loca are
// transformed at version 0, everything else at any version that is not zero",
// so a glyf at version 1 was taken for an untransformed table and its
// transformed bytes were copied into the font whole; and a cmap at version 2
// was taken for a transformed table and handed to the hmtx transform's reader.
// A file naming a transform this engine does not implement was decoded as
// though it did.

// woff2WithVersion wraps a synthetic font, giving one table a transform version
// of its own and leaving the rest at whatever the format's default is.
func woff2WithVersion(t *testing.T, tag string, version uint8) []byte {
	t.Helper()
	sfnt := fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{
			{Rune: 'A', Advance: 600, HasShape: true},
			{Rune: 'B', Advance: 700, HasShape: true},
		},
	})
	tabs := SFNTTables(sfnt)
	var opts fonttest.WOFF2Options
	for _, k := range []string{"cmap", "glyf", "head", "hhea", "hmtx", "loca", "maxp", "name", "post"} {
		body, ok := tabs[k]
		if !ok {
			t.Fatalf("the synthetic font has no %s table", k)
		}
		e := fonttest.WOFF2Table{Tag: k, Data: body}
		if k == tag {
			e.Version = version
		}
		opts.Tables = append(opts.Tables, e)
	}
	fonttest.SortWOFF2Tables(opts.Tables)
	return fonttest.WOFF2(opts)
}

// TestAReservedTransformVersionIsRefused.
func TestAReservedTransformVersionIsRefused(t *testing.T) {
	for _, c := range []struct {
		tag      string
		version  uint8
		accepted bool
		why      string
	}{
		// The two the format defines, and the "not transformed" spelling each
		// of them has.
		{"glyf", 3, true, "glyf's null transform"},
		{"loca", 3, true, "loca's null transform"},
		{"hmtx", 0, true, "hmtx untransformed"},
		{"cmap", 0, true, "every other table untransformed"},

		// Reserved, and every one of them used to be read as one of the two.
		{"glyf", 1, false, "reserved for glyf"},
		{"glyf", 2, false, "reserved for glyf"},
		{"loca", 1, false, "reserved for loca"},
		{"loca", 2, false, "reserved for loca"},
		{"hmtx", 2, false, "reserved for hmtx"},
		{"hmtx", 3, false, "reserved for hmtx"},
		{"cmap", 1, false, "no transform is defined for cmap"},
		{"cmap", 2, false, "no transform is defined for cmap"},
		{"cmap", 3, false, "no transform is defined for cmap"},
		{"name", 1, false, "no transform is defined for name"},
	} {
		t.Run(c.why+" ("+c.tag+" v"+string('0'+c.version)+")", func(t *testing.T) {
			out, err := DecodeWOFF2(woff2WithVersion(t, c.tag, c.version))
			switch {
			case c.accepted && err != nil:
				t.Errorf("a font the format describes was refused: %v", err)
			case c.accepted && len(out) == 0:
				t.Error("the font came back empty")
			case !c.accepted && err == nil:
				t.Errorf("%s v%d was accepted; the specification says the value "+
					"is reserved and must not be used, and reading it as one of "+
					"the two defined transforms is decoding a file this format "+
					"does not describe", c.tag, c.version)
			case !c.accepted && !strings.Contains(err.Error(), "transform"):
				t.Errorf("refused, but the message does not say why: %v", err)
			}
		})
	}
}
