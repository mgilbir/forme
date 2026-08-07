package shape

import (
	"encoding/binary"
	"math"
	"strings"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// What LoadInstance does, what it refuses, and where it stops.
//
// The two oracle tests next door answer whether the arithmetic is right, over
// four real faces and eight locations. They cannot answer the rest of it: no
// real font names a point its glyph does not have, declares sixty-five axes, or
// builds a composite out of itself, and a font that does is what a reader has to
// survive rather than trust. Nor can they show *which* of two disagreeing
// sources an advance came from, since a real font's two sources mostly agree.
//
// So the fixtures here are built rather than found, and every number a test
// asserts is worked out from the format in the comment above it — an expectation
// copied out of this package's own output would agree with whatever this package
// decided.

// varFont is a fixture: a design space, some glyphs, and how they move.
type varFont struct {
	axes      []fonttest.VarAxis
	instances []fonttest.VarInstance
	// glyphs are glyf entries, glyph 0 first. A nil entry is a blank glyph.
	glyphs   [][]byte
	advances []int
	// tuples is one list per glyph, in glyph order; a nil list is a glyph that
	// does not vary. Leaving the whole field nil leaves gvar out of the font.
	tuples [][]fonttest.VarTuple
	// avar is one segment map per axis, in normalized coordinates.
	avar [][][2]float64
	// hvar is the table verbatim, since what it has to say is which of two
	// sources an advance comes from and that needs the delta stated exactly.
	hvar []byte
	// names are extra name-table records by ID, for the named instances.
	names map[int]string
	// shortLoca writes loca in its two-byte form, which an instance never emits
	// and roughly half the fonts in the world are stored in.
	shortLoca bool
	// extra is written over everything above; a nil value removes a table.
	extra map[string][]byte
}

// wghtWdth is the design space the fixtures use.
//
// Two axes and not one, because a single axis cannot show a coordinate being
// read against the wrong one: with one axis every index is zero and every
// mistake about which axis a number belongs to is invisible. The ranges are the
// registered axes' own, so that 700 in a test reads as the weight it is.
var wghtWdth = []fonttest.VarAxis{
	{Tag: "wght", Min: 100, Def: 400, Max: 900},
	{Tag: "wdth", Min: 50, Def: 100, Max: 200},
}

// rectGlyph is the glyph most cases are about: one contour, four on-curve
// points, its origin at x=0 so that the left side bearing is its xMin.
func rectGlyph() []byte {
	return fonttest.SimpleGlyph([]int{0, 100, 100, 0}, []int{0, 0, 200, 200})
}

func (v varFont) build(t *testing.T) []byte {
	t.Helper()
	n := len(v.glyphs)
	if len(v.advances) != n {
		t.Fatalf("the fixture has %d glyphs and %d advances", n, len(v.advances))
	}

	// The scaffolding — cmap, maxp, post, head — comes from the synthetic font
	// the rest of this package's tests use, and the tables instancing reads are
	// then written over it.
	shapes := make([]fonttest.Glyph, n-1)
	for i := range shapes {
		shapes[i] = fonttest.Glyph{Rune: rune('a' + i), Advance: v.advances[i+1], HasShape: true}
	}
	base := fonttest.SFNT(fonttest.SFNTOptions{Name: "VarTest", Glyphs: shapes})
	out := map[string][]byte{}
	for tag, b := range font.SFNTTables(base) {
		out[tag] = b
	}

	var glyf []byte
	loca := make([]byte, 4*(n+1))
	for i, g := range v.glyphs {
		binary.BigEndian.PutUint32(loca[4*i:], uint32(len(glyf)))
		glyf = append(glyf, g...)
		for len(glyf)%4 != 0 { // glyf entries are long-aligned
			glyf = append(glyf, 0)
		}
	}
	binary.BigEndian.PutUint32(loca[4*n:], uint32(len(glyf)))
	out["glyf"], out["loca"] = glyf, loca
	if v.shortLoca {
		// The short form states half the offset, which is why every entry is
		// long-aligned; head has to say which form is in use.
		short := make([]byte, 2*(n+1))
		for i := 0; i <= n; i++ {
			binary.BigEndian.PutUint16(short[2*i:], uint16(binary.BigEndian.Uint32(loca[4*i:])/2))
		}
		out["loca"] = short
		head := append([]byte(nil), out["head"]...)
		binary.BigEndian.PutUint16(head[50:], 0)
		out["head"] = head
	}

	hmtx := make([]byte, 4*n)
	for i, a := range v.advances {
		binary.BigEndian.PutUint16(hmtx[4*i:], uint16(a))
	}
	out["hmtx"] = hmtx
	hhea := append([]byte(nil), out["hhea"]...)
	binary.BigEndian.PutUint16(hhea[34:], uint16(n))
	out["hhea"] = hhea
	out["OS/2"] = varTestOS2(400, 5)
	out["name"] = varTestName("VarTest", v.names)

	if len(v.axes) > 0 {
		out["fvar"] = fonttest.FVAR(v.axes, v.instances)
	}
	if v.tuples != nil {
		out["gvar"] = fonttest.GVAR(len(v.axes), v.tuples)
	}
	if v.avar != nil {
		out["avar"] = fonttest.AVAR(v.avar)
	}
	if v.hvar != nil {
		out["HVAR"] = v.hvar
	}
	for tag, b := range v.extra {
		if b == nil {
			delete(out, tag)
			continue
		}
		out[tag] = b
	}
	return assembleSFNT(out)
}

// varTestOS2 builds a version 4 OS/2 table, which is what carries the weight and
// width classes an instance has to rewrite.
func varTestOS2(weight, width int) []byte {
	os2 := make([]byte, 96)
	binary.BigEndian.PutUint16(os2[0:], 4) // version
	binary.BigEndian.PutUint16(os2[4:], uint16(weight))
	binary.BigEndian.PutUint16(os2[6:], uint16(width))
	binary.BigEndian.PutUint16(os2[68:], 800)    // sTypoAscender
	binary.BigEndian.PutUint16(os2[70:], 0xFF38) // sTypoDescender, -200
	binary.BigEndian.PutUint16(os2[86:], 500)    // sxHeight
	binary.BigEndian.PutUint16(os2[88:], 700)    // sCapHeight
	return os2
}

// varTestName builds a name table: the PostScript name under ID 6, and whatever
// else a fixture states — the subfamily and PostScript names fvar's instance
// records point at.
func varTestName(psName string, extra map[int]string) []byte {
	ids := []int{6}
	values := map[int]string{6: psName}
	for id, s := range extra {
		if id != 6 {
			ids = append(ids, id)
		}
		values[id] = s
	}
	for i := 1; i < len(ids); i++ { // the records must be in name-ID order
		for j := i; j > 0 && ids[j] < ids[j-1]; j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
	head := make([]byte, 6+12*len(ids))
	binary.BigEndian.PutUint16(head[0:], 0) // format
	binary.BigEndian.PutUint16(head[2:], uint16(len(ids)))
	binary.BigEndian.PutUint16(head[4:], uint16(len(head)))
	var strs []byte
	for i, id := range ids {
		rec := 6 + 12*i
		binary.BigEndian.PutUint16(head[rec:], 3)   // Windows
		binary.BigEndian.PutUint16(head[rec+2:], 1) // Unicode BMP
		binary.BigEndian.PutUint16(head[rec+4:], 0x409)
		binary.BigEndian.PutUint16(head[rec+6:], uint16(id))
		var utf16be []byte
		for _, r := range values[id] {
			utf16be = append(utf16be, byte(r>>8), byte(r))
		}
		binary.BigEndian.PutUint16(head[rec+8:], uint16(len(utf16be)))
		binary.BigEndian.PutUint16(head[rec+10:], uint16(len(strs)))
		strs = append(strs, utf16be...)
	}
	return append(head, strs...)
}

// patch16 rewrites one sixteen-bit field of a table, which is how a fixture
// states a font that is malformed in exactly one way.
func patch16(b []byte, off, v int) []byte {
	out := append([]byte(nil), b...)
	binary.BigEndian.PutUint16(out[off:], uint16(v))
	return out
}

// TestLoadInstanceRefuses is what LoadInstance declines and why.
//
// A refusal is a feature here rather than a shortfall: a font this cannot
// instance correctly would otherwise be instanced *incorrectly*, and an outline
// drawn from a misread delta is a glyph in the wrong place with nothing to say
// so. Each case names one thing that cannot be honoured, and asserts the words
// a caller is told.
func TestLoadInstanceRefuses(t *testing.T) {
	// The font every case starts from, so that a case differs from a working
	// instance in exactly the one way it is about.
	good := varFont{
		axes:     wghtWdth,
		glyphs:   [][]byte{nil, rectGlyph(), fonttest.CompositeGlyph([]fonttest.CompositeComponent{{Glyph: 1, DX: 300}})},
		advances: []int{0, 500, 900},
		tuples: [][]fonttest.VarTuple{
			nil,
			{{Peak: []float64{1, 0}, Points: []int{0, 2}, DX: []int{10, 30}, DY: []int{0, 0}}},
			nil,
		},
	}
	bold := map[string]float64{"wght": 900}

	// A font whose only defect is in one table: the table is built correct and
	// then the one field is written over.
	withTable := func(tag string, patch func([]byte) []byte) []byte {
		f := good
		f.extra = map[string][]byte{tag: patch(tableOfFont(t, good.build(t), tag))}
		return f.build(t)
	}

	for _, tc := range []struct {
		what   string
		data   []byte
		coords map[string]float64
		want   string
	}{
		{"a font that is not an sfnt at all", []byte("this is not a font"), bold,
			"not an sfnt font program"},

		{"a static font, which has no design space to be loaded at",
			func() []byte { f := good; f.axes = nil; f.tuples = nil; return f.build(t) }(), nil,
			"no fvar table"},

		{"a CFF2 font, whose outlines vary by another mechanism",
			func() []byte { f := good; f.extra = map[string][]byte{"CFF2": {0, 2, 0, 0}}; return f.build(t) }(), bold,
			"CFF2"},

		{"a font with no glyf outlines to move",
			func() []byte { f := good; f.extra = map[string][]byte{"glyf": nil}; return f.build(t) }(), bold,
			"no glyf outlines"},

		// The one refusal that is not about a malformed font. A caller naming an
		// axis the font does not have would otherwise be handed the default
		// silently, which is the whole complaint LoadInstance answers.
		{"an axis the font does not have", good.build(t), map[string]float64{"opsz": 12},
			`no "opsz" axis; it has "wght", "wdth"`},
		{"a coordinate that is not a number", good.build(t), map[string]float64{"wght": math.NaN()},
			`the "wght" coordinate is not a number`},
		{"a coordinate that is infinite", good.build(t), map[string]float64{"wght": math.Inf(1)},
			`the "wght" coordinate is not a number`},

		{"an fvar of a version this does not read",
			withTable("fvar", func(b []byte) []byte { return patch16(b, 0, 2) }), bold,
			"fvar is version 2"},
		{"an fvar declaring no axes",
			withTable("fvar", func(b []byte) []byte { return patch16(b, 8, 0) }), bold,
			"declares no axes"},
		{"an fvar whose axis records are shorter than an axis record",
			withTable("fvar", func(b []byte) []byte { return patch16(b, 10, 16) }), bold,
			"axis records of 16 bytes"},
		{"an fvar whose axis records lie outside it",
			withTable("fvar", func(b []byte) []byte { return patch16(b, 4, 0xFF00) }), bold,
			"axis records lie outside"},
		{"an axis whose default is outside its own range",
			// minValue, the second field of the first axis record, put above the
			// default.
			withTable("fvar", func(b []byte) []byte { return patch16(b, 20, 500) }), bold,
			"axis runs 500..900 with a default of 400"},

		{"an avar of a minor version whose extra mapping this does not read",
			func() []byte {
				f := good
				f.avar = [][][2]float64{{{-1, -1}, {0, 0}, {1, 1}}, {{-1, -1}, {0, 0}, {1, 1}}}
				f.extra = map[string][]byte{"avar": patch16(fonttest.AVAR(f.avar), 2, 1)}
				return f.build(t)
			}(), bold,
			"avar is version 1.1"},
		{"an avar mapping a different number of axes than fvar declares",
			func() []byte {
				f := good
				f.avar = [][][2]float64{{{-1, -1}, {0, 0}, {1, 1}}}
				return f.build(t)
			}(), bold,
			"avar maps 1 axes and fvar declares 2"},

		{"a gvar of a version this does not read",
			withTable("gvar", func(b []byte) []byte { return patch16(b, 0, 2) }), bold,
			"gvar is version 2"},
		{"a gvar stating a different axis count than fvar",
			withTable("gvar", func(b []byte) []byte { return patch16(b, 4, 3) }), bold,
			"gvar states 3 axes and fvar states 2"},
		{"a gvar carrying data for more glyphs than the font has",
			withTable("gvar", func(b []byte) []byte { return patch16(b, 12, 99) }), bold,
			"gvar carries data for 99 glyphs and the font has 3"},
		{"a tuple naming a point the glyph does not have",
			func() []byte {
				f := good
				f.tuples = [][]fonttest.VarTuple{nil,
					{{Peak: []float64{1, 0}, Points: []int{99}, DX: []int{1}, DY: []int{1}}}, nil}
				return f.build(t)
			}(), bold,
			"it names point 99 and the glyph has 8"},

		{"a composite placed by matching points rather than at an offset",
			func() []byte {
				f := good
				f.glyphs = [][]byte{nil, rectGlyph(),
					fonttest.CompositeGlyph([]fonttest.CompositeComponent{{Glyph: 1, DX: 2, DY: 3, MatchPoints: true}})}
				return f.build(t)
			}(), bold,
			"matching points, which cannot be instanced"},
		{"a composite naming a component the font does not have",
			func() []byte {
				f := good
				f.glyphs = [][]byte{nil, rectGlyph(),
					fonttest.CompositeGlyph([]fonttest.CompositeComponent{{Glyph: 99}})}
				return f.build(t)
			}(), bold,
			"names component 99, which the font does not have"},

		{"an HVAR of a version this does not read",
			func() []byte {
				f := good
				f.hvar = patch16(oneRegionHVAR(50), 0, 2)
				return f.build(t)
			}(), bold,
			"HVAR is version 2"},
		{"an HVAR whose item variation store is of a format this does not read",
			func() []byte {
				f := good
				f.hvar = patch16(oneRegionHVAR(50), 20, 2)
				return f.build(t)
			}(), bold,
			"item variation store is format 2"},
		{"an HVAR whose advance mapping is of a format this does not read",
			func() []byte {
				f := good
				h := oneRegionHVAR(50)
				binary.BigEndian.PutUint32(h[8:], uint32(len(h)))
				f.hvar = append(h, 3, 0, 0, 1) // format 3
				return f.build(t)
			}(), bold,
			"advance mapping: the delta-set index map is format 3"},
	} {
		t.Run(tc.what, func(t *testing.T) {
			_, err := LoadInstance(tc.data, tc.coords)
			if err == nil {
				t.Fatalf("instancing succeeded, and %s cannot be instanced", tc.what)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the error is %q, which does not say %q", err, tc.want)
			}
		})
	}
}

// oneRegionHVAR is an HVAR with one region — the upper half of the weight axis —
// and one delta per glyph, the second glyph's being the one given.
func oneRegionHVAR(delta int) []byte {
	return fonttest.HVAR(2,
		[]fonttest.VarRegion{{Start: []float64{0, 0}, Peak: []float64{1, 0}, End: []float64{1, 0}}},
		[][]int{{0}, {delta}, {0}}, nil)
}

// tableOfFont reads one table back out of an assembled font.
func tableOfFont(t *testing.T, data []byte, tag string) []byte {
	t.Helper()
	b := font.SFNTTables(data)[tag]
	if b == nil {
		t.Fatalf("the fixture has no %s table", tag)
	}
	return append([]byte(nil), b...)
}

// TestLoadInstanceCaps: each bound, seen to fire.
//
// A cap that has never been watched refuse anything is not a cap, it is a
// constant. None of these can be reached by a font anybody publishes — that is
// the point of where they are set — so each fixture here is built to reach one
// of them and nothing else.
func TestLoadInstanceCaps(t *testing.T) {
	rect := rectGlyph()

	t.Run("more axes than the design space of any font", func(t *testing.T) {
		// The format allows 65535 axes. Every axis multiplies what each tuple
		// costs to weigh, and the fonts that exist have between one and five.
		axes := make([]fonttest.VarAxis, maxInstanceAxes+1)
		for i := range axes {
			axes[i] = fonttest.VarAxis{Tag: fmtTag(i), Min: 0, Def: 0, Max: 1}
		}
		f := varFont{axes: axes, glyphs: [][]byte{nil, rect}, advances: []int{0, 500}}
		mustRefuse(t, f.build(t), nil, "declares 65 axes, more than the 64")
	})

	t.Run("a composite with more components than any glyph has", func(t *testing.T) {
		// The format states components as a chain rather than a count, so in a
		// malformed font the only thing that ends the chain is running out of
		// bytes.
		comps := make([]fonttest.CompositeComponent, maxComponents+1)
		for i := range comps {
			comps[i] = fonttest.CompositeComponent{Glyph: 1, DX: i % 100}
		}
		f := varFont{
			axes:     wghtWdth,
			glyphs:   [][]byte{nil, rect, fonttest.CompositeGlyph(comps)},
			advances: []int{0, 500, 900},
		}
		mustRefuse(t, f.build(t), nil, "names more than 4096 components")
	})

	t.Run("composites that refer to each other in a circle", func(t *testing.T) {
		// Neither glyph is malformed on its own, and the chain has no end: this
		// is the one shape that recurses forever rather than running out of
		// bytes.
		f := varFont{
			axes: wghtWdth,
			glyphs: [][]byte{nil,
				fonttest.CompositeGlyph([]fonttest.CompositeComponent{{Glyph: 2}}),
				fonttest.CompositeGlyph([]fonttest.CompositeComponent{{Glyph: 1}})},
			advances: []int{0, 500, 900},
		}
		mustRefuse(t, f.build(t), nil, "components nest more than 8 deep")
	})

	t.Run("more point arithmetic than a font may ask for", func(t *testing.T) {
		// A tuple costs one unit per point of its glyph whatever it lists,
		// because the points it does not list are inferred from the ones it
		// does and that walks the glyph. So a glyph of sixty thousand points
		// and four thousand tuples, which is a few kilobytes of gvar, asks for
		// a quarter of a billion operations — and the budget is shared, so two
		// such glyphs are what it takes to exceed it.
		//
		// Each tuple here lists a single point, which is the cheapest thing to
		// state and the most expensive thing to apply.
		const points = 60000
		xs := make([]int, points)
		ys := make([]int, points)
		for i := range xs {
			xs[i], ys[i] = i%128, (i*7)%128
		}
		big := fonttest.SimpleGlyph(xs, ys)
		tuples := make([]fonttest.VarTuple, gvarCountMask)
		for i := range tuples {
			tuples[i] = fonttest.VarTuple{
				Peak: []float64{1, 0}, Points: []int{i % points},
				DX: []int{1}, DY: []int{1},
			}
		}
		f := varFont{
			axes:     wghtWdth,
			glyphs:   [][]byte{nil, big, big},
			advances: []int{0, 500, 500},
			tuples:   [][]fonttest.VarTuple{nil, tuples, tuples},
		}
		// One glyph is under the budget and two are over it, which is what
		// shows the budget is shared rather than per glyph.
		one := varFont{
			axes:     wghtWdth,
			glyphs:   [][]byte{nil, big},
			advances: []int{0, 500},
			tuples:   [][]fonttest.VarTuple{nil, tuples},
		}
		if _, err := LoadInstance(one.build(t), map[string]float64{"wght": 900}); err != nil {
			t.Fatalf("one glyph of %d tuples is inside the budget and was refused: %v", len(tuples), err)
		}
		mustRefuse(t, f.build(t), map[string]float64{"wght": 900},
			"needs more than 268435456 point operations")
	})
}

// mustRefuse asserts that a font cannot be instanced, and why.
func mustRefuse(t *testing.T, data []byte, coords map[string]float64, want string) {
	t.Helper()
	_, err := LoadInstance(data, coords)
	if err == nil {
		t.Fatal("instancing succeeded, and this font is past a bound that exists to stop it")
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("the error is %q, which does not say %q", err, want)
	}
}

// fmtTag is a distinct four-character axis tag per index, for a fixture that
// needs more axes than there are registered ones.
func fmtTag(i int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz"
	return string([]byte{'x', 'x', alphabet[i/26%26], alphabet[i%26]})
}

// instancedGlyph is one glyph of an instanced face, read back out of the font
// program with the independent reader the oracle test uses.
func instancedGlyph(t *testing.T, f *Face, gid int) outlineGlyph {
	t.Helper()
	glyf, loca, n := instancedOutlines(t, f)
	g := readGlyphOutline(t, glyf, loca, gid)
	advances, bearings := instancedMetrics(t, f, n)
	g.advance, g.bearing = advances[gid], bearings[gid]
	return g
}

// instanced is LoadInstance, and then one glyph of it.
func instanced(t *testing.T, data []byte, coords map[string]float64, gid int) outlineGlyph {
	t.Helper()
	f, err := LoadInstance(data, coords)
	if err != nil {
		t.Fatalf("instancing at %v: %v", coords, err)
	}
	return instancedGlyph(t, f, gid)
}

// xs is a glyph's x coordinates, which is what most of these cases are about:
// the fixtures move points sideways so that one row of numbers says everything.
func xs(g outlineGlyph) []int {
	out := make([]int, len(g.points))
	for i, p := range g.points {
		out[i] = p.x
	}
	return out
}

// listedPointsFont is the fixture the inference cases are built on.
//
// The rectangle runs (0,0) (100,0) (100,200) (0,200) as one contour, and the
// tuple lists only points 0 and 2 — diagonally opposite corners — moving them
// right by 10 and 30 at the top of the weight axis. A second tuple, peaked on
// the width axis, moves everything a long way, so a location that names only the
// weight has something to be wrong about.
func listedPointsFont(t *testing.T) []byte {
	t.Helper()
	return varFont{
		axes:     wghtWdth,
		glyphs:   [][]byte{nil, rectGlyph()},
		advances: []int{0, 500},
		tuples: [][]fonttest.VarTuple{nil, {
			{Peak: []float64{1, 0}, Points: []int{0, 2}, DX: []int{10, 30}, DY: []int{0, 0}},
			{Peak: []float64{0, 1}, Points: []int{0, 2}, DX: []int{500, 500}, DY: []int{0, 0}},
		}},
	}.build(t)
}

// TestInstancingMovesTheListedPointsAndInfersTheRest.
//
// Points 1 and 3 are not "unmoved": what the format says of a point no tuple
// lists is that it goes where its position between its two listed neighbours
// puts it. Point 1 sits at x=100, which is at the further neighbour (point 2,
// x=100), so it takes that neighbour's whole delta of 30; point 3 sits at x=0,
// at the nearer one (point 0, x=0), so it takes 10. Leaving them where they were
// does not fail — it draws a rectangle with two corners pulled off it, which is
// why this needs a test rather than an eyeball.
func TestInstancingMovesTheListedPointsAndInfersTheRest(t *testing.T) {
	data := listedPointsFont(t)
	for _, tc := range []struct {
		why    string
		coords map[string]float64
		want   []int
	}{
		{"every axis at its default draws what glyf stores", nil, []int{0, 100, 100, 0}},
		{"the top of the axis applies the tuple whole",
			map[string]float64{"wght": 900}, []int{10, 130, 130, 10}},
		{"halfway up the axis applies half of it",
			map[string]float64{"wght": 650}, []int{5, 115, 115, 5}},
		{"a coordinate past the top of the axis is clamped to it",
			map[string]float64{"wght": 5000}, []int{10, 130, 130, 10}},
		{"a tuple peaked at the top contributes nothing at the bottom",
			map[string]float64{"wght": 100}, []int{0, 100, 100, 0}},
		{"a coordinate below the bottom is clamped to it",
			map[string]float64{"wght": -400}, []int{0, 100, 100, 0}},
		{"an axis the caller does not name stays at its own default",
			map[string]float64{"wght": 900}, []int{10, 130, 130, 10}},
		// The second tuple is peaked on the width axis and moves the same two
		// points a long way, so a location that names only the width has to
		// reach it and only it.
		{"the other axis has a tuple of its own", map[string]float64{"wdth": 200},
			[]int{500, 600, 600, 500}},
		// The weight tuple's peak on the width axis is zero, which means that
		// axis takes no part in weighing it. That is not the same as a *width*
		// of zero, where a tuple built around any other peak contributes
		// nothing: reading the one as the other silences the tuple as soon as
		// any other axis moves.
		{"an axis a tuple gives no peak takes no part in weighing it",
			map[string]float64{"wght": 900, "wdth": 50}, []int{10, 130, 130, 10}},
	} {
		g := instanced(t, data, tc.coords, 1)
		if got := xs(g); !sameInts(got, tc.want) {
			t.Errorf("%s: the points are at x %v, want %v", tc.why, got, tc.want)
		}
	}
}

// TestInstancingLeavesThePhantomPointsToThemselves.
//
// The four phantom points are counted among a glyph's points — a delta list is
// indexed against them — but they are not part of any contour, so a tuple that
// lists none of them must leave them where they were rather than interpolate
// them from the outline points around them. Getting that wrong changes the
// advance of every glyph in the font by a plausible-looking amount.
func TestInstancingLeavesThePhantomPointsToThemselves(t *testing.T) {
	g := instanced(t, listedPointsFont(t), map[string]float64{"wght": 900}, 1)
	if g.advance != 500 {
		t.Errorf("the advance is %d and the tuple lists no phantom point, so it should still be 500", g.advance)
	}
	// The box and the bearing follow the points that did move.
	if g.xMin != 10 || g.xMax != 130 || g.yMin != 0 || g.yMax != 200 {
		t.Errorf("the bounding box is %d %d %d %d, want 10 0 130 200", g.xMin, g.yMin, g.xMax, g.yMax)
	}
	if g.bearing != 10 {
		t.Errorf("the left side bearing is %d, want 10 — the glyph's xMin measured from its origin", g.bearing)
	}
}

// TestInstancingMovesTheOriginWithThePhantomPoints.
//
// The first phantom point is the glyph's origin, and it varies like any other:
// a font may move the outline and the origin by different amounts across an
// axis. The left side bearing an instance writes is the distance from that
// origin to the glyph's xMin, so it cannot be read off the outline afterwards —
// and taking xMin for it works on almost every font, because almost every font
// has the two in the same place, which is what makes it worth pinning here.
func TestInstancingMovesTheOriginWithThePhantomPoints(t *testing.T) {
	data := varFont{
		axes:     wghtWdth,
		glyphs:   [][]byte{nil, rectGlyph()},
		advances: []int{0, 500},
		// The origin and the advance point both move left by 20, so the advance
		// is unchanged and only the origin has gone anywhere.
		tuples: [][]fonttest.VarTuple{nil, {{
			Peak: []float64{1, 0},
			DX:   []int{0, 0, 0, 0, -20, -20, 0, 0},
			DY:   []int{0, 0, 0, 0, 0, 0, 0, 0},
		}}},
	}.build(t)

	g := instanced(t, data, map[string]float64{"wght": 900}, 1)
	if got, want := xs(g), []int{0, 100, 100, 0}; !sameInts(got, want) {
		t.Fatalf("the outline is at x %v, want %v — this fixture moves only the phantom points", got, want)
	}
	if g.advance != 500 {
		t.Errorf("the advance is %d, want 500 — both horizontal phantom points moved together", g.advance)
	}
	if g.bearing != 20 {
		t.Errorf("the left side bearing is %d, want 20 — the glyph's xMin of 0 measured from an origin at -20", g.bearing)
	}
}

// TestInstancingBendsTheAxisThroughAvar.
//
// avar exists because the middle of an axis is rarely the middle of what it
// draws. The fixture's map sends the middle of the weight axis to a quarter of
// the way up, so the same user coordinate has to produce a quarter of the tuple
// rather than half of it — and a reader that skipped avar would produce a face
// that is visibly the wrong weight rather than one that fails.
func TestInstancingBendsTheAxisThroughAvar(t *testing.T) {
	f := varFont{
		axes:     wghtWdth,
		glyphs:   [][]byte{nil, rectGlyph()},
		advances: []int{0, 500},
		tuples: [][]fonttest.VarTuple{nil,
			{{Peak: []float64{1, 0}, Points: []int{0, 2}, DX: []int{20, 40}, DY: []int{0, 0}}}},
	}
	straight := instanced(t, f.build(t), map[string]float64{"wght": 650}, 1)
	if got, want := xs(straight), []int{10, 120, 120, 10}; !sameInts(got, want) {
		t.Fatalf("without avar the points are at x %v, want %v", got, want)
	}

	f.avar = [][][2]float64{
		{{-1, -1}, {0, 0}, {0.5, 0.25}, {1, 1}},
		{{-1, -1}, {0, 0}, {1, 1}},
	}
	bent := instanced(t, f.build(t), map[string]float64{"wght": 650}, 1)
	if got, want := xs(bent), []int{5, 110, 110, 5}; !sameInts(got, want) {
		t.Errorf("with avar the points are at x %v, want %v — a quarter of the tuple, not half", got, want)
	}
}

// advanceFont states a glyph's advance twice and differently: gvar's phantom
// points make it 600 at the top of the weight axis, and HVAR makes it 550.
//
// No published font disagrees with itself on purpose, and Noto Sans disagrees by
// a unit on thirteen glyphs by accident. The fixture exaggerates it so that
// which source was read is unmistakable, because that choice decides whether a
// document is laid out the way a browser renders the same text.
func advanceFont(t *testing.T, withHVAR bool) []byte {
	t.Helper()
	f := varFont{
		axes:     wghtWdth,
		glyphs:   [][]byte{nil, rectGlyph()},
		advances: []int{0, 500},
		// Every point, so that the four phantom points are in the list: index 5
		// is the second horizontal one, which is the advance.
		tuples: [][]fonttest.VarTuple{nil, {{
			Peak: []float64{1, 0},
			DX:   []int{0, 0, 0, 0, 0, 100, 0, 0},
			DY:   []int{0, 0, 0, 0, 0, 0, 0, 0},
		}}},
	}
	if withHVAR {
		f.hvar = oneRegionHVAR(50)
	}
	return f.build(t)
}

func TestInstanceAdvancesComeFromHVARWhenTheFontHasIt(t *testing.T) {
	data := advanceFont(t, true)
	for _, tc := range []struct {
		coords map[string]float64
		want   int
		why    string
	}{
		{map[string]float64{"wght": 900}, 550, "HVAR's answer, not the phantom points' 600"},
		// Halfway up the axis the region is half applied, which the phantom
		// points would make 550 and HVAR makes 525.
		{map[string]float64{"wght": 650}, 525, "half of HVAR's delta, not half of the phantom points'"},
		{nil, 500, "the default instance, where nothing applies"},
	} {
		if got := instanced(t, data, tc.coords, 1).advance; got != tc.want {
			t.Errorf("at %v the advance is %d, want %d — %s", tc.coords, got, tc.want, tc.why)
		}
	}
}

func TestInstanceAdvancesComeFromThePhantomPointsWithoutHVAR(t *testing.T) {
	data := advanceFont(t, false)
	for coords, want := range map[float64]int{900: 600, 650: 550, 400: 500} {
		if got := instanced(t, data, map[string]float64{"wght": coords}, 1).advance; got != want {
			t.Errorf("at wght=%g the advance is %d, want %d — the phantom points', the font having no HVAR",
				coords, got, want)
		}
	}
}

// TestInstancingWeighsAnIntermediateTuple.
//
// A tuple's region is normally the one its peak implies: from the default
// instance out to the peak, and nothing beyond. A tuple may instead state where
// its influence starts and ends, which is how a font gives one stretch of an axis
// a correction that does not bleed into the rest. Reading the peak and ignoring
// the stated region does not fail — it applies the correction over a different
// stretch, at a different strength, which is a face subtly wrong in the middle of
// its own axis.
func TestInstancingWeighsAnIntermediateTuple(t *testing.T) {
	f := varFont{
		axes:     wghtWdth,
		glyphs:   [][]byte{nil, rectGlyph()},
		advances: []int{0, 500},
		tuples: [][]fonttest.VarTuple{nil, {{
			Peak:  []float64{0.5, 0},
			Start: []float64{0.25, 0}, End: []float64{0.75, 0},
			Points: []int{0, 2}, DX: []int{40, 40}, DY: []int{0, 0},
		}}},
	}
	data := f.build(t)
	// The weight axis runs 100..400..900, so a user coordinate normalizes to
	// (v-400)/500.
	for _, tc := range []struct {
		wght float64
		norm float64
		want []int
	}{
		{650, 0.5, []int{40, 140, 140, 40}},     // at the peak, applied whole
		{525, 0.25, []int{0, 100, 100, 0}},      // at the start, not yet applied
		{587.5, 0.375, []int{20, 120, 120, 20}}, // halfway from start to peak
		{775, 0.75, []int{0, 100, 100, 0}},      // at the end, applied no longer
		{900, 1, []int{0, 100, 100, 0}},         // past it
	} {
		got := xs(instanced(t, data, map[string]float64{"wght": tc.wght}, 1))
		if !sameInts(got, tc.want) {
			t.Errorf("at wght=%g, which normalizes to %g, the points are at x %v, want %v",
				tc.wght, tc.norm, got, tc.want)
		}
	}
}

// TestInstancingIgnoresAnAxisWhoseRegionSpansTheDefault.
//
// The specification reserves the shape — a region running from below the default
// instance to above it — and says a reader is to ignore that axis rather than the
// tuple. It reads as a mistake and is the rule every implementation follows, so
// pinning it is what stops it being tidied into something more plausible: with
// the axis ignored the tuple applies in full *everywhere*, including at the
// default instance, which is the one place a tuple normally cannot reach.
func TestInstancingIgnoresAnAxisWhoseRegionSpansTheDefault(t *testing.T) {
	data := varFont{
		axes:     wghtWdth,
		glyphs:   [][]byte{nil, rectGlyph()},
		advances: []int{0, 500},
		tuples: [][]fonttest.VarTuple{nil, {{
			Peak:  []float64{0.5, 0},
			Start: []float64{-1, 0}, End: []float64{1, 0},
			Points: []int{0, 2}, DX: []int{40, 40}, DY: []int{0, 0},
		}}},
	}.build(t)
	for _, wght := range []float64{100, 400, 650, 900} {
		got := xs(instanced(t, data, map[string]float64{"wght": wght}, 1))
		if want := []int{40, 140, 140, 40}; !sameInts(got, want) {
			t.Errorf("at wght=%g the points are at x %v, want %v", wght, got, want)
		}
	}
}

// TestInstanceAdvancesStayPutWithNeitherSource: a variable font need carry
// neither, and then hmtx already says what the advance is at every location.
func TestInstanceAdvancesStayPutWithNeitherSource(t *testing.T) {
	data := varFont{
		axes:     wghtWdth,
		glyphs:   [][]byte{nil, rectGlyph()},
		advances: []int{0, 500},
	}.build(t)
	g := instanced(t, data, map[string]float64{"wght": 900}, 1)
	if g.advance != 500 {
		t.Errorf("the advance is %d, want 500 — the font says nothing about how it varies", g.advance)
	}
	if got, want := xs(g), []int{0, 100, 100, 0}; !sameInts(got, want) {
		t.Errorf("the points are at x %v, want %v — a font with no gvar has nothing to move them", got, want)
	}
}

// TestInstanceFollowsHVARsAdvanceMapping.
//
// HVAR need not give every glyph a row of its own: an advance mapping lets many
// glyphs share one delta set, which is how a font with a long tail of identical
// advances is stored. A glyph past the end of the mapping takes the last entry,
// which is what makes such a tail free to state.
func TestInstanceFollowsHVARsAdvanceMapping(t *testing.T) {
	data := varFont{
		axes:     wghtWdth,
		glyphs:   [][]byte{nil, rectGlyph(), rectGlyph()},
		advances: []int{0, 500, 700},
		hvar: fonttest.HVAR(2,
			[]fonttest.VarRegion{{Start: []float64{0, 0}, Peak: []float64{1, 0}, End: []float64{1, 0}}},
			[][]int{{0}, {50}},
			// Two entries for three glyphs: .notdef takes row 0 and glyph 1 row
			// 1, and glyph 2 is past the end so it takes the last entry, row 1.
			[]int{0, 1}),
	}.build(t)
	f, err := LoadInstance(data, map[string]float64{"wght": 900})
	if err != nil {
		t.Fatal(err)
	}
	for gid, want := range map[int]int{1: 550, 2: 750} {
		if got := instancedGlyph(t, f, gid).advance; got != want {
			t.Errorf("glyph %d's advance is %d, want %d", gid, got, want)
		}
	}
}

// TestInstancingMovesCompositeComponents.
//
// A composite has no points of its own: what gvar varies for one is its
// components' offsets, and its bounding box is whatever its components come to
// once *they* have been instanced — which they have not been when the composite
// itself is written, so it takes a second pass. A reader that measured a
// composite as it went would box it around the default instance's components.
func TestInstancingMovesCompositeComponents(t *testing.T) {
	data := varFont{
		axes: wghtWdth,
		glyphs: [][]byte{nil, rectGlyph(), fonttest.CompositeGlyph([]fonttest.CompositeComponent{
			{Glyph: 1}, {Glyph: 1, DX: 300},
		})},
		advances: []int{0, 500, 900},
		tuples: [][]fonttest.VarTuple{nil,
			{{Peak: []float64{1, 0}, Points: []int{0, 2}, DX: []int{10, 30}, DY: []int{0, 0}}},
			// Two components and four phantom points: index 1 is the second
			// component's offset.
			{{Peak: []float64{1, 0}, DX: []int{0, 50, 0, 0, 0, 0}, DY: []int{0, 0, 0, 0, 0, 0}}},
		},
	}.build(t)

	g := instanced(t, data, map[string]float64{"wght": 900}, 2)
	if g.kind != 'c' {
		t.Fatalf("glyph 2 came out a %q glyph; a composite must stay one", g.kind)
	}
	want := []outlinePoint{{gid: 1, x: 0, y: 0}, {gid: 1, x: 350, y: 0}}
	if len(g.points) != 2 || g.points[0] != want[0] || g.points[1] != want[1] {
		t.Errorf("the components are %v, want %v", g.points, want)
	}
	// The rectangle runs x 10..130 in this instance, so the two copies of it
	// cover 10..130 and 360..480.
	if g.xMin != 10 || g.xMax != 480 || g.yMin != 0 || g.yMax != 200 {
		t.Errorf("the bounding box is %d %d %d %d, want 10 0 480 200 — measured from the components as instanced",
			g.xMin, g.yMin, g.xMax, g.yMax)
	}
}

// TestInstanceIsAStaticFont.
//
// A design point, once chosen, *is* a static font — which is why LoadInstance
// hands one back rather than a face with a setting on it. Leaving the variation
// tables in would be worse than useless: everything downstream reads them as
// deltas from a default instance that is no longer the stored one.
func TestInstanceIsAStaticFont(t *testing.T) {
	dummy := []byte{0, 1, 0, 0, 0, 0, 0, 0}
	data := varFont{
		axes:     wghtWdth,
		glyphs:   [][]byte{nil, withInstructions(rectGlyph(), []byte{0x4B, 0x4B, 0x4B})},
		advances: []int{0, 500},
		tuples: [][]fonttest.VarTuple{nil,
			{{Peak: []float64{1, 0}, Points: []int{0, 2}, DX: []int{10, 30}, DY: []int{0, 0}}}},
		avar: [][][2]float64{{{-1, -1}, {0, 0}, {1, 1}}, {{-1, -1}, {0, 0}, {1, 1}}},
		hvar: oneRegionHVAR(50),
		extra: map[string][]byte{
			"MVAR": dummy, "STAT": dummy, "VVAR": dummy, "cvar": dummy,
			"cvt ": dummy, "fpgm": dummy, "prep": dummy,
		},
	}.build(t)

	before, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	if !before.IsVariable() || len(before.Axes()) != 2 {
		t.Fatalf("the fixture loads with %d axes and IsVariable %v; it is meant to be variable",
			len(before.Axes()), before.IsVariable())
	}

	f, err := LoadInstance(data, map[string]float64{"wght": 900})
	if err != nil {
		t.Fatal(err)
	}
	if f.IsVariable() || f.Axes() != nil {
		t.Errorf("the instance reports %d axes and IsVariable %v; it has one location and no design space",
			len(f.Axes()), f.IsVariable())
	}
	tables := font.SFNTTables(f.data)
	for _, tag := range []string{"fvar", "gvar", "avar", "cvar", "HVAR", "VVAR", "MVAR", "STAT", "cvt ", "fpgm", "prep"} {
		if _, ok := tables[tag]; ok {
			t.Errorf("the instance still carries %s, which describes a design space it no longer has", tag)
		}
	}
	for _, tag := range []string{"glyf", "loca", "head", "hhea", "hmtx", "maxp", "cmap", "name", "OS/2", "post"} {
		if _, ok := tables[tag]; !ok {
			t.Errorf("the instance has no %s", tag)
		}
	}

	// The hinting instructions go with cvt and prep: 'cvar' is not read, so
	// keeping them would hint the new outlines by the old instance's control
	// values.
	glyf, loca, _ := instancedOutlines(t, f)
	entry := glyf[loca[1]:loca[2]]
	nc := int(int16(binary.BigEndian.Uint16(entry)))
	if n := int(binary.BigEndian.Uint16(entry[10+2*nc:])); n != 0 {
		t.Errorf("the instanced glyph carries %d bytes of instructions, hinted for the weight it is no longer at", n)
	}
}

// withInstructions splices hinting instructions into a simple glyf entry, which
// the fixture builder writes without.
func withInstructions(entry, instr []byte) []byte {
	nc := int(int16(binary.BigEndian.Uint16(entry)))
	at := 10 + 2*nc // instructionLength
	out := append([]byte(nil), entry[:at]...)
	var n [2]byte
	binary.BigEndian.PutUint16(n[:], uint16(len(instr)))
	out = append(out, n[:]...)
	out = append(out, instr...)
	return append(out, entry[at+2:]...)
}

// TestInstanceReportsTheWeightItDraws.
//
// This is the complaint that started all of this, in one assertion. OS/2's
// weight class is not variation data and instancing does not touch the outlines
// it describes, so nothing would have made it change — and the face would then
// say Regular while drawing Bold, which is the state of five faces published
// under the OFL and exactly what a consumer cannot detect.
func TestInstanceReportsTheWeightItDraws(t *testing.T) {
	data := varFont{axes: wghtWdth, glyphs: [][]byte{nil, rectGlyph()}, advances: []int{0, 500}}.build(t)

	stored, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := stored.Descriptor().Weight; got != 400 {
		t.Fatalf("the stored font's weight class is %d; the fixture states 400", got)
	}

	for _, tc := range []struct {
		coords        map[string]float64
		weight, width int
	}{
		{map[string]float64{"wght": 700}, 700, 5},
		{map[string]float64{"wght": 100}, 100, 5},
		// Clamped to the axis, so a caller asking for more than the font offers
		// is told what it got rather than what it asked for.
		{map[string]float64{"wght": 5000}, 900, 5},
		// The width classes are the nine OS/2 names, and 75% is Condensed.
		{map[string]float64{"wdth": 75}, 400, 3},
		{map[string]float64{"wdth": 100}, 400, 5},
		{map[string]float64{"wght": 700, "wdth": 200}, 700, 9},
		// An axis nobody named leaves its class alone.
		{nil, 400, 5},
	} {
		f, err := LoadInstance(data, tc.coords)
		if err != nil {
			t.Fatalf("instancing at %v: %v", tc.coords, err)
		}
		if got := f.Descriptor().Weight; got != tc.weight {
			t.Errorf("at %v the weight class is %d, want %d", tc.coords, got, tc.weight)
		}
		os2 := font.SFNTTables(f.data)["OS/2"]
		if got := font.Be16(os2, 6); got != tc.width {
			t.Errorf("at %v the width class is %d, want %d", tc.coords, got, tc.width)
		}
	}
}

// TestInstanceNamesTheInstance.
//
// The PostScript name becomes /BaseFont in a PDF, and the one a variable font
// carries is its *default* instance's. Leaving it alone would put a document's
// bold text under a name that says Regular, and two weights of one face into one
// document under one name, where nothing downstream could tell them apart.
func TestInstanceNamesTheInstance(t *testing.T) {
	data := varFont{
		axes: wghtWdth,
		instances: []fonttest.VarInstance{
			{SubfamilyNameID: 257, Coords: []float64{700, 100}, PostScriptNameID: 258},
			// An instance with no PostScript name of its own, which the format
			// states as the absent field this record is too short to hold — here
			// as the reserved 0xFFFF.
			{SubfamilyNameID: 259, Coords: []float64{300, 100}, PostScriptNameID: 0xFFFF},
		},
		names:    map[int]string{257: "Bold", 258: "VarTest-Bold", 259: "Light"},
		glyphs:   [][]byte{nil, rectGlyph()},
		advances: []int{0, 500},
	}.build(t)

	for _, tc := range []struct {
		why    string
		coords map[string]float64
		want   string
	}{
		{"a location the font names takes the publisher's own name",
			map[string]float64{"wght": 700}, "VarTest-Bold"},
		{"a location the font names but does not name a PostScript name for falls back to its coordinates",
			map[string]float64{"wght": 300}, "VarTest-wght300"},
		{"a location the font does not name is named from its coordinates",
			map[string]float64{"wght": 650}, "VarTest-wght650"},
		{"two axes off their defaults name both",
			map[string]float64{"wght": 650, "wdth": 75}, "VarTest-wght650-wdth75"},
		{"the default instance keeps the name the font already has",
			nil, "VarTest"},
		{"and so does a location that names the defaults explicitly",
			map[string]float64{"wght": 400, "wdth": 100}, "VarTest"},
	} {
		f, err := LoadInstance(data, tc.coords)
		if err != nil {
			t.Fatalf("%s: %v", tc.why, err)
		}
		if f.Name() != tc.want {
			t.Errorf("%s: the instance is called %q, want %q", tc.why, f.Name(), tc.want)
		}
	}
}

// TestInstanceWritesTheLongLocaForm: the short form states half an offset, and
// an instance's glyphs are not the stored ones, so nothing says the offsets
// still fit. Writing the long form always is what makes that a non-question.
func TestInstanceWritesTheLongLocaForm(t *testing.T) {
	data := varFont{
		axes: wghtWdth, shortLoca: true,
		glyphs:   [][]byte{nil, rectGlyph()},
		advances: []int{0, 500},
		tuples: [][]fonttest.VarTuple{nil,
			{{Peak: []float64{1, 0}, Points: []int{0, 2}, DX: []int{10, 30}, DY: []int{0, 0}}}},
	}.build(t)
	if got := font.Be16(font.SFNTTables(data)["head"], 50); got != 0 {
		t.Fatalf("the fixture's indexToLocFormat is %d; it is meant to be the short form", got)
	}

	f, err := LoadInstance(data, map[string]float64{"wght": 900})
	if err != nil {
		t.Fatal(err)
	}
	if got := font.Be16(font.SFNTTables(f.data)["head"], 50); got != 1 {
		t.Errorf("the instance's indexToLocFormat is %d, want 1", got)
	}
	// And the glyph read back through the long form is the one the short form
	// held, moved.
	if got, want := xs(instancedGlyph(t, f, 1)), []int{10, 130, 130, 10}; !sameInts(got, want) {
		t.Errorf("the points are at x %v, want %v", got, want)
	}
}

// TestInstanceRewritesTheHorizontalHeader.
//
// hmtx does not state an advance for every glyph: the trailing glyphs that share
// one carry a bearing alone, and hhea says how many advances there are. The
// count is about the instance's advances rather than the stored font's, and a
// header left saying the old number describes a table that is no longer that
// shape.
func TestInstanceRewritesTheHorizontalHeader(t *testing.T) {
	data := varFont{
		axes:     wghtWdth,
		glyphs:   [][]byte{nil, rectGlyph(), rectGlyph(), rectGlyph()},
		advances: []int{0, 500, 700, 700},
	}.build(t)
	f, err := LoadInstance(data, map[string]float64{"wght": 900})
	if err != nil {
		t.Fatal(err)
	}
	tables := font.SFNTTables(f.data)
	if got := font.Be16(tables["hhea"], 34); got != 3 {
		t.Errorf("the instance states %d horizontal metrics, want 3 — the last two glyphs share an advance", got)
	}
	if got := len(tables["hmtx"]); got != 4*3+2 {
		t.Errorf("hmtx is %d bytes, want %d — three full metrics and one bearing", got, 4*3+2)
	}
	if got := font.Be16(tables["hhea"], 10); got != 700 {
		t.Errorf("advanceWidthMax is %d, want 700", got)
	}
}

// TestInferringUnlistedPoints is the inference on its own, at the cases the
// fixtures above cannot reach.
//
// Each is a shape the specification names and a real font produces: a contour
// no tuple touched, one it touched once, and two listed points that sit on top
// of each other and move apart — which says nothing about where a point between
// them goes, so it goes nowhere.
func TestInferringUnlistedPoints(t *testing.T) {
	for _, tc := range []struct {
		why    string
		ends   []int
		ox     []float64
		dx     []float64
		known  []bool
		wantDX []float64
	}{
		{
			why:    "a contour no tuple listed does not move",
			ends:   []int{3},
			ox:     []float64{0, 10, 20, 30},
			dx:     []float64{0, 0, 0, 0},
			known:  []bool{false, false, false, false},
			wantDX: []float64{0, 0, 0, 0},
		},
		{
			why:    "a contour with one listed point moves whole",
			ends:   []int{3},
			ox:     []float64{0, 10, 20, 30},
			dx:     []float64{0, 7, 0, 0},
			known:  []bool{false, true, false, false},
			wantDX: []float64{7, 7, 7, 7},
		},
		{
			why:    "a point between two listed ones moves in proportion",
			ends:   []int{2},
			ox:     []float64{0, 10, 20},
			dx:     []float64{0, 0, 20},
			known:  []bool{true, false, true},
			wantDX: []float64{0, 10, 20},
		},
		{
			why:   "a point outside the two listed ones takes the nearer one's delta",
			ends:  []int{3},
			ox:    []float64{0, 5, 10, 100},
			dx:    []float64{2, 0, 8, 0},
			known: []bool{true, false, true, false},
			// Point 1 is between them; point 3 is past both, so it takes the
			// delta of the point it is nearest along the coordinate.
			wantDX: []float64{2, 5, 8, 8},
		},
		{
			why:    "two listed points at one coordinate moving apart say nothing about a point between",
			ends:   []int{2},
			ox:     []float64{10, 4, 10},
			dx:     []float64{3, 0, -3},
			known:  []bool{true, false, true},
			wantDX: []float64{3, 0, -3},
		},
		{
			why: "the points past the last contour — the phantom four — are left alone",
			// Two contour points and two beyond, which is what a glyph's point
			// list looks like once the phantom points are counted.
			ends:   []int{1},
			ox:     []float64{0, 10, 500, 600},
			dx:     []float64{4, 0, 0, 0},
			known:  []bool{true, false, false, false},
			wantDX: []float64{4, 4, 0, 0},
		},
	} {
		dx := append([]float64(nil), tc.dx...)
		dy := make([]float64, len(dx))
		known := append([]bool(nil), tc.known...)
		oy := make([]float64, len(tc.ox))
		inferPoints(tc.ends, tc.ox, oy, dx, dy, known)
		for i := range tc.wantDX {
			if dx[i] != tc.wantDX[i] {
				t.Errorf("%s: the deltas came to %v, want %v", tc.why, dx, tc.wantDX)
				break
			}
		}
	}
}

// TestFeatureVariationsFollowTheInstancedLocation.
//
// A face can state different rules for different parts of its design space, and
// which set applies is decided by conditions on the axis coordinates. Loading at
// a location and then reading the rules for another one would set the text by
// rules meant for a different weight — every glyph a real one, none of them the
// right one, and nothing to show for it but wrong-looking words.
//
// The third case is the one that needs two axes. Its condition names the width
// axis while the location moves the weight: a reader that compared the range
// against whichever coordinate came first would find -1 in range and apply a
// rule stated for a font half as wide.
func TestFeatureVariationsFollowTheInstancedLocation(t *testing.T) {
	data := varyingVariableFont(t, []fonttest.FeatureVariation{{
		// The narrow end of the width axis, which is axis 1.
		Conditions: []fonttest.Condition{{Axis: 1, Min: -1, Max: -0.5}},
		Substitute: map[int][]int{0: {1}},
	}})

	for _, tc := range []struct {
		why      string
		instance bool
		coords   map[string]float64
		want     int
	}{
		{"the stored font is the default instance, where the record does not hold", false, nil, scY},
		{"instanced at the default, likewise", true, nil, scY},
		{"instanced at the narrow end, where it does", true, map[string]float64{"wdth": 50}, scZ},
		{"instanced part of the way there, short of the range", true, map[string]float64{"wdth": 90}, scY},
		{"a light weight is not a narrow width, though both normalize to -1",
			true, map[string]float64{"wght": 100}, scY},
	} {
		var f *Face
		var err error
		if tc.instance {
			f, err = LoadInstance(data, tc.coords)
		} else {
			f, err = Load(data)
		}
		if err != nil {
			t.Fatalf("%s: %v", tc.why, err)
		}
		if got := lastGID(t, f, "x"); got != tc.want {
			t.Errorf("%s: x shaped to glyph %d, want %d", tc.why, got, tc.want)
		}
	}
}

// varyingVariableFont is the FeatureVariations fixture with a design space to
// be instanced in: 'ccmp' names the lookup that turns x into y, and a variation
// record may name the one that turns it into z.
func varyingVariableFont(t *testing.T, records []fonttest.FeatureVariation) []byte {
	t.Helper()
	gsub := fonttest.GSUBTableVarying(
		[]fonttest.Lookup{
			{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{scX}, []int{scY})}},
			{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{scX}, []int{scZ})}},
		},
		[]fonttest.Feature{{Tag: "ccmp", Lookups: []int{0}}},
		map[string]fonttest.Script{"DFLT": fonttest.AllFeatures(1)},
		records,
	)
	return fonttest.SFNT(fonttest.SFNTOptions{
		Name: "Varying",
		Glyphs: []fonttest.Glyph{
			{Rune: 'x', Advance: 500, HasShape: true},
			{Rune: 'y', Advance: 500, HasShape: true},
			{Rune: 'z', Advance: 500, HasShape: true},
		},
		Extra: map[string][]byte{"GSUB": gsub, "fvar": fonttest.FVAR(wghtWdth, nil)},
	})
}
