package fonttest

import (
	"testing"

	"github.com/mgilbir/forme/font"
)

func TestCFFFixtureParses(t *testing.T) {
	for _, c := range []struct {
		name   string
		opts   CFFOptions
		cid    bool
		reg    string
		ord    string
		sup    int
		glyphs int
	}{
		{"plain", CFFOptions{Glyphs: 3}, false, "", "", 0, 3},
		{"cid", CFFOptions{Glyphs: 4, CIDKeyed: true}, true, "Adobe", "Identity", 0, 4},
		{"cid named", CFFOptions{Glyphs: 2, CIDKeyed: true, Registry: "Acme", Ordering: "Japan9", Supplement: 3}, true, "Acme", "Japan9", 3, 2},
		{"unnamed", CFFOptions{Glyphs: 2, CIDKeyed: true, UnnamedCollection: true}, true, "", "", 0, 2},
		{"negative supplement", CFFOptions{Glyphs: 2, CIDKeyed: true, NegativeSupplement: true}, true, "", "", 0, 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := font.ParseCFF(CFF(c.opts))
			if p == nil {
				t.Fatal("the fixture does not parse")
			}
			if (p.GIDToCID != nil) != c.cid {
				t.Errorf("CID-keyed = %v, want %v", p.GIDToCID != nil, c.cid)
			}
			if p.NumGlyphs != c.glyphs {
				t.Errorf("%d glyphs, want %d", p.NumGlyphs, c.glyphs)
			}
			if p.Registry != c.reg || p.Ordering != c.ord || p.Supplement != c.sup {
				t.Errorf("collection %q-%q-%d, want %q-%q-%d",
					p.Registry, p.Ordering, p.Supplement, c.reg, c.ord, c.sup)
			}
		})
	}
}
