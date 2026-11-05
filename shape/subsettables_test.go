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

// TestSubsetPostHasNoGlyphNames: a subset carries post as format 3, its
// header and no names, whatever the font's post was: the names of the glyphs
// it drops are most of a large font's subset (#979).
func TestSubsetPostHasNoGlyphNames(t *testing.T) {
	gs := alphabet()
	post := postWithNames(len(gs) + 1)
	f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{Name: "Probe", Glyphs: gs, Extra: map[string][]byte{"post": post}}))
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
		t.Fatalf("the subset's post is %d bytes, version %#x: want format 3's 32", len(got), binary.BigEndian.Uint32(got))
	}
	if string(got[4:32]) != string(post[4:32]) {
		t.Error("the post header past its version changed")
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
		t.Run(tc.name, func(t *testing.T) {
			src := fonttest.SFNT(fonttest.SFNTOptions{Name: "Probe", Glyphs: tc.glyphs})
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
