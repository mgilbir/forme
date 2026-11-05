package shape

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// postWithNames is a post table of format 2 naming each of n glyphs "gN",
// the form most fonts carry and a subset need not (#979).
func postWithNames(n int) []byte {
	b := make([]byte, 32)
	binary.BigEndian.PutUint32(b, 0x00020000)
	b = binary.BigEndian.AppendUint16(b, uint16(n))
	for g := 0; g < n; g++ {
		b = binary.BigEndian.AppendUint16(b, uint16(258+g))
	}
	for g := 0; g < n; g++ {
		name := fmt.Sprintf("g%d", g)
		b = append(b, byte(len(name)))
		b = append(b, name...)
	}
	return b
}

// keepsNumbering are the two kinds of face whose subset keeps the face's glyph
// numbering, and so carries the tables around its outlines cut to what it
// keeps (subsettables.go): glyf, and a CFF that is not CID-keyed.
var keepsNumbering = map[string]func(fonttest.SFNTOptions) []byte{
	"glyf": fonttest.SFNT,
	"CFF": func(o fonttest.SFNTOptions) []byte {
		return fonttest.OTTO(fonttest.CFF(fonttest.CFFOptions{Glyphs: len(o.Glyphs) + 1}), o)
	},
}

// TestSubsetPostHasNoGlyphNames: a subset carries post as format 3, its
// header and no names, whatever the font's post was: the names of the glyphs
// it drops are most of a large font's subset (#979).
func TestSubsetPostHasNoGlyphNames(t *testing.T) {
	for kind, build := range keepsNumbering {
		gs := alphabet()
		post := postWithNames(len(gs) + 1)
		f, err := Load(build(fonttest.SFNTOptions{Name: "Probe", Glyphs: gs, Extra: map[string][]byte{"post": post}}))
		if err != nil {
			t.Fatal(err)
		}
		f.Encode("abc")
		data, err := f.Subset()
		if err != nil {
			t.Fatal(err)
		}
		got := font.SFNTTables(data)["post"]
		if len(got) != 32 || binary.BigEndian.Uint32(got) != 0x00030000 {
			t.Fatalf("%s: the subset's post is %d bytes, version %#x: want format 3's 32", kind, len(got), binary.BigEndian.Uint32(got))
		}
		if string(got[4:32]) != string(post[4:32]) {
			t.Errorf("%s: the post header past its version changed", kind)
		}
	}
}

// TestSubsetHMetricsStopAtTheLastKeptGlyph: a subset's long horizontal
// metrics stop at its last kept glyph (#979); every glyph it keeps reads back
// with its advance and left side bearing, and the glyphs it drops past them
// read a left side bearing of 0. The numbering is unchanged.
func TestSubsetHMetricsStopAtTheLastKeptGlyph(t *testing.T) {
	for _, tc := range []struct {
		name   string
		glyphs []fonttest.Glyph
		text   string
	}{
		{"proportional", inked(alphabet()), "abk"},
		{"monospace", inked(monospace()), "abk"},
		// Equal advances up to a last kept glyph of another: the run before
		// it is not one the long metrics may stop in.
		{"wide last", inked(wideK()), "abk"},
	} {
		for kind, build := range keepsNumbering {
			t.Run(kind+" "+tc.name, func(t *testing.T) {
				src := build(fonttest.SFNTOptions{Name: "Probe", Glyphs: tc.glyphs})
				f, err := Load(src)
				if err != nil {
					t.Fatal(err)
				}
				f.Encode(tc.text)
				data, kept, err := f.SubsetGlyphs()
				if err != nil {
					t.Fatal(err)
				}
				last := kept[len(kept)-1]
				orig, sub := font.SFNTTables(src), font.SFNTTables(data)
				long := int(binary.BigEndian.Uint16(sub["hhea"][34:]))
				if long > last+1 {
					t.Errorf("numberOfHMetrics = %d past the last kept glyph %d", long, last)
				}
				n := f.prog.NumGlyphs
				if got := len(sub["hmtx"]); got != 4*long+2*(n-long) {
					t.Errorf("hmtx is %d bytes for %d long metrics of %d glyphs", got, long, n)
				}
				origLong := int(binary.BigEndian.Uint16(orig["hhea"][34:]))
				metric := func(hmtx []byte, long, g int) (adv, lsb int16) {
					a := g
					if a >= long {
						a = long - 1
					}
					adv = int16(binary.BigEndian.Uint16(hmtx[4*a:]))
					if g < long {
						lsb = int16(binary.BigEndian.Uint16(hmtx[4*g+2:]))
					} else {
						lsb = int16(binary.BigEndian.Uint16(hmtx[4*long+2*(g-long):]))
					}
					return adv, lsb
				}
				isKept := map[int]bool{}
				for _, g := range kept {
					isKept[g] = true
					wa, wl := metric(orig["hmtx"], origLong, g)
					ga, gl := metric(sub["hmtx"], long, g)
					if ga != wa || gl != wl {
						t.Errorf("kept glyph %d: advance %d, lsb %d; want %d, %d", g, ga, gl, wa, wl)
					}
				}
				for g := long; g < n; g++ {
					if _, gl := metric(sub["hmtx"], long, g); !isKept[g] && gl != 0 {
						t.Errorf("dropped glyph %d past the long metrics: lsb %d, want 0", g, gl)
					}
				}
				// And as this module's reader reads the subset.
				p := font.ParseSFNT(data, 1<<20)
				if p == nil || p.NumGlyphs != n {
					t.Fatalf("the subset does not read back with %d glyphs", n)
				}
				q := font.ParseSFNT(src, 1<<20)
				for _, g := range kept {
					if p.WidthByGID[g] != q.WidthByGID[g] {
						t.Errorf("kept glyph %d reads back %v wide, want %v", g, p.WidthByGID[g], q.WidthByGID[g])
					}
				}
			})
		}
	}
}

// monospace is alphabet with one advance for every letter, so the long
// metrics of a subset of it stop before its last kept glyph.
func monospace() []fonttest.Glyph {
	gs := alphabet()
	for i := range gs {
		gs[i].Advance = 600
	}
	return gs
}

// wideK is monospace with k wider than the rest.
func wideK() []fonttest.Glyph {
	gs := monospace()
	for i := range gs {
		if gs[i].Rune == 'k' {
			gs[i].Advance = 700
		}
	}
	return gs
}

// inked gives every glyph ink starting 40 units in, so its left side
// bearing is 40, not the 0 a dropped glyph's must read.
func inked(gs []fonttest.Glyph) []fonttest.Glyph {
	for i := range gs {
		gs[i].Ink = [4]int{40, 0, 400, 700}
	}
	return gs
}

// TestSubsetCmapMapsOnlyWhatItKept: a subset's cmap maps exactly the
// characters of the face's own map whose glyph it kept, components included,
// for each kind of face that keeps its numbering; and a face embedded as a
// simple font, which a reader finds glyphs in through the cmap, keeps the
// font's whole.
func TestSubsetCmapMapsOnlyWhatItKept(t *testing.T) {
	for kind, build := range keepsNumbering {
		src := build(fonttest.SFNTOptions{Name: "Probe", Glyphs: alphabet()})
		f, err := Load(src)
		if err != nil {
			t.Fatal(err)
		}
		f.Encode("abc")
		data, kept, err := f.SubsetGlyphs()
		if err != nil {
			t.Fatal(err)
		}
		isKept := map[int]bool{}
		for _, g := range kept {
			isKept[g] = true
		}
		want := map[rune]int{}
		for r, g := range f.Cmap() {
			if g > 0 && isKept[g] {
				want[r] = g
			}
		}
		p := font.ParseSFNT(data, 1<<20)
		if p == nil {
			t.Fatalf("%s: the subset does not parse", kind)
		}
		if len(p.Cmap) != len(want) || len(want) != 3 {
			t.Errorf("%s: the subset maps %d characters, want the %d of its kept glyphs", kind, len(p.Cmap), len(want))
		}
		for r, g := range want {
			if p.Cmap[r] != g {
				t.Errorf("%s: %q maps to %d in the subset, want %d", kind, r, p.Cmap[r], g)
			}
		}
		if got, whole := len(font.SFNTTables(data)["cmap"]), len(font.SFNTTables(src)["cmap"]); got >= whole {
			t.Errorf("%s: the subset's cmap is %d bytes and the font's %d", kind, got, whole)
		}
	}

	src := fonttest.SFNT(fonttest.SFNTOptions{Name: "Ascii", Glyphs: asciiGlyphs()})
	simple, err := LoadSimple(src)
	if err != nil {
		t.Fatal(err)
	}
	simple.Encode("abc")
	data, err := simple.Subset()
	if err != nil {
		t.Fatal(err)
	}
	if got, whole := font.SFNTTables(data)["cmap"], font.SFNTTables(src)["cmap"]; string(got) != string(whole) {
		t.Errorf("a simple face's subset cmap is %d bytes, want the font's own %d", len(got), len(whole))
	}
}

// TestASubsetOfARealFaceCarriesWhatItKeeps is #979's measurement, on the
// bundled face: the glyphs of "Measurement 0123456789" made a subset of
// 109,562 bytes, of which post was 53,822, cmap 14,496 and hmtx 18,058. Each
// of those now describes the kept glyphs: post names none, cmap maps their
// characters, and hmtx stops at the last of them.
func TestASubsetOfARealFaceCarriesWhatItKeeps(t *testing.T) {
	data := notoSansBytes(t)
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	f.Encode("Measurement 0123456789")
	prog, kept, err := f.SubsetGlyphs()
	if err != nil {
		t.Fatal(err)
	}
	tables := font.SFNTTables(prog)
	if got := len(tables["post"]); got != 32 {
		t.Errorf("post is %d bytes, want format 3's 32", got)
	}
	// One segment per character at most, and a format 4's header.
	if got := len(tables["cmap"]); got > 12+16+8*len(kept) {
		t.Errorf("cmap is %d bytes for %d kept glyphs", got, len(kept))
	}
	long := int(binary.BigEndian.Uint16(tables["hhea"][34:]))
	n := f.NumGlyphs()
	if got := len(tables["hmtx"]); long > kept[len(kept)-1]+1 || got != 4*long+2*(n-long) {
		t.Errorf("hmtx is %d bytes with %d long metrics; the last kept glyph is %d", got, long, kept[len(kept)-1])
	}
	if len(prog) > 50000 {
		t.Errorf("the subset is %d bytes", len(prog))
	}
}
