package shape

import (
	"math"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/segment"
)

// Apple's tracking table, trak: how much space a face adds after each
// character at each size it is set at, which Apple's fonts state for their
// text to be set tighter as it grows.
//
// What is here is HarfBuzz's at the release the oracle is pinned to
// (hb-aat-layout-trak-table.hh): it applies the table only to a face that
// also has a STAT table, which is how it tells the "modern" fonts CoreText
// tracks by default from the older ones; the normal track, the one at zero;
// the value at the size the run is set at, interpolated between the sizes the
// table states and held at the ends, and at CoreText's default of 12 points
// where the run states no size (Features.PointSize); added, rounded to a whole
// unit, to the advance of the first glyph of each grapheme — a base and its
// marks, an emoji and what joins it — and not to the glyphs that continue
// one.

// coreTextDefaultSize is HB_CORETEXT_DEFAULT_FONT_SIZE: the size, in points,
// a face is tracked at where nothing says what size it is set at.
const coreTextDefaultSize = 12

// readTrak is a face's trak where HarfBuzz applies it — the face has a STAT
// too — and nil otherwise: no trak, one whose major version is not 1, or no
// STAT.
func readTrak(tables map[string][]byte) []byte {
	t, stat := tables["trak"], tables["STAT"]
	if len(t) < 12 || font.Be16(t, 0) != 1 || len(stat) < 4 || font.Be32(stat, 0) == 0 {
		return nil
	}
	return t
}

// tracking is trak::get_h_tracking or get_v_tracking: the normal track at a
// size, in font units, rounded as HarfBuzz rounds it.
func (f *Face) tracking(vertical bool, size float64) int {
	t := f.trak
	data := font.Be16(t, 6)
	if vertical {
		data = font.Be16(t, 8)
	}
	ptem := float32(size)
	if ptem <= 0 {
		ptem = coreTextDefaultSize
	}
	if data == 0 {
		return 0
	}
	return int(math.Round(float64(trackingAt(t, data, ptem))))
}

// trackingAt is TrackData::get_tracking for the normal track, 0: the value
// of the track, or of the two either side of it interpolated, at a size.
func trackingAt(t []byte, at int, ptem float32) float32 {
	if len(t)-at < 8 {
		return 0
	}
	n := font.Be16(t, at)
	if n == 0 {
		return 0
	}
	sizes := trackSizes{t: t, at: int(font.Be32(t, at+4)), n: font.Be16(t, at+2)}
	entry := func(i int) int { return at + 8 + 8*i }
	trackValue := func(i int) float32 {
		p := entry(i)
		if len(t)-p < 8 {
			return 0
		}
		return float32(int32(font.Be32(t, p))) / 65536
	}
	if n == 1 {
		return sizes.value(entry(0), ptem)
	}
	const track = 0
	i, j := 0, n-1
	for i+1 < n && trackValue(i+1) <= track {
		i++
	}
	for j > 0 && trackValue(j-1) >= track {
		j--
	}
	if i == j {
		return sizes.value(entry(i), ptem)
	}
	t0, t1 := trackValue(i), trackValue(j)
	k := float32((track - t0) / (t1 - t0))
	a, b := sizes.value(entry(i), ptem), sizes.value(entry(j), ptem)
	return a + k*(b-a)
}

// trackSizes is a TrackData's size table: the sizes, in points, its tracks
// state values at.
type trackSizes struct {
	t     []byte
	at, n int
}

// size is the i-th size, and zero past what the table holds.
func (s trackSizes) size(i int) float32 {
	p := s.at + 4*i
	if p < 0 || len(s.t)-p < 4 {
		return 0
	}
	return float32(int32(font.Be32(s.t, p))) / 65536
}

// valueAt is a track's i-th value, and zero past what the table holds.
func (s trackSizes) valueAt(entry, i int) int {
	if len(s.t)-entry < 8 {
		return 0
	}
	p := font.Be16(s.t, entry+6) + 2*i
	if len(s.t)-p < 2 {
		return 0
	}
	return signed16(font.Be16(s.t, p))
}

// value is TrackTableEntry::get_value: a track's value at a size, the
// nearest at either end, and interpolated between the two sizes either side.
func (s trackSizes) value(entry int, ptem float32) float32 {
	if s.n == 0 {
		return 0
	}
	if s.n == 1 {
		return float32(s.valueAt(entry, 0))
	}
	i := 0
	for i < s.n && s.size(i) < ptem {
		i++
	}
	switch {
	case i == 0:
		return float32(s.valueAt(entry, 0))
	case i == s.n:
		return float32(s.valueAt(entry, s.n-1))
	case s.size(i) == ptem:
		return float32(s.valueAt(entry, i))
	}
	// interpolate_at.
	s0, s1 := s.size(i-1), s.size(i)
	v0, v1 := s.valueAt(entry, i-1), s.valueAt(entry, i)
	if s1 < s0 {
		s0, s1 = s1, s0
		v0, v1 = v1, v0
	}
	switch {
	case ptem < s0:
		return float32(v0)
	case ptem > s1:
		return float32(v1)
	case s0 == s1:
		return float32(v0+v1) * 0.5
	}
	k := (ptem - s0) / (s1 - s0)
	return float32(v0) + k*float32(v1-v0)
}

// applyTrak adds the face's tracking to the first glyph of each grapheme of a
// run, in the order its characters are written.
func (sh shaper) applyTrak(buf []Glyph) {
	vertical := sh.features.Vertical
	v := sh.f.scale(sh.f.tracking(vertical, sh.features.PointSize))
	if v == 0 {
		return
	}
	for i := range buf {
		if i > 0 && buf[i].cont {
			continue
		}
		if vertical {
			buf[i].addYAdvance(v)
		} else {
			buf[i].addXAdvance(v)
		}
	}
}

// graphemeContinues is hb_set_unicode_props's continuation bit for each
// character of a run: a mark, an emoji modifier, the second of a pair of
// regional indicators, a zero width joiner and the pictograph after it, a tag
// character, and the two halfwidth katakana sound marks continue the grapheme
// before them.
//
// The answer is written into dst's array where it has room, as the run's
// shaping keeps one on the face (runScratch); nil makes a new one.
func graphemeContinues(dst []bool, runes []rune) []bool {
	out := reuse(dst, len(runes))
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r < 0x80:
		case isCombiningMark(r):
			out[i] = true
		case r >= 0x1F3FB && r <= 0x1F3FF:
			out[i] = true
		case r >= 0x1F1E6 && r <= 0x1F1FF:
			if i > 0 && runes[i-1] >= 0x1F1E6 && runes[i-1] <= 0x1F1FF && !out[i-1] {
				out[i] = true
			}
		case r == 0x200D:
			out[i] = true
			if i+1 < len(runes) && segment.ExtendedPictographic(runes[i+1]) {
				i++
				out[i] = true
			}
		case r >= 0xFF9E && r <= 0xFF9F, r >= 0xE0020 && r <= 0xE007F:
			out[i] = true
		}
	}
	return out
}
