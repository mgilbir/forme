package shape

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// One reader for one table.
//
// fvar was read in two places that did not agree: LoadInstance's parseFvar
// checked the version, the axis count, the record size and that each axis runs
// from its minimum through its default to its maximum, while Axes checked the
// record size alone and stopped at the first record that did not fit rather than
// refusing the table. A caller reads Axes to find out what it may ask for, so
// the two disagreeing means being told about an axis that cannot be asked for.

// fvarTable builds an fvar with the header fields and axis records given, so
// that each of parseFvar's refusals can be written down.
func fvarTable(version, axisCount, axisSize int, axes [][4]interface{}) []byte {
	t := make([]byte, 16)
	binary.BigEndian.PutUint16(t[0:], uint16(version))
	binary.BigEndian.PutUint16(t[2:], 0)
	binary.BigEndian.PutUint16(t[4:], 16) // axesArrayOffset
	binary.BigEndian.PutUint16(t[6:], 2)
	binary.BigEndian.PutUint16(t[8:], uint16(axisCount))
	binary.BigEndian.PutUint16(t[10:], uint16(axisSize))
	for _, a := range axes {
		rec := make([]byte, axisSize)
		copy(rec, a[0].(string))
		for i, v := range []float64{a[1].(float64), a[2].(float64), a[3].(float64)} {
			binary.BigEndian.PutUint32(rec[4+4*i:], uint32(int32(v*65536)))
		}
		t = append(t, rec...)
	}
	return t
}

// axisFace loads a font carrying the given fvar.
func axisFace(t *testing.T, fvar []byte) *Face {
	t.Helper()
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name:   "Varying",
		Glyphs: []fonttest.Glyph{{Rune: 'a', Advance: 500, HasShape: true}},
		Extra:  map[string][]byte{"fvar": fvar},
	})
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// TestAxesNamesOnlyAnAxisThatCanBeAskedFor.
func TestAxesNamesOnlyAnAxisThatCanBeAskedFor(t *testing.T) {
	good := [][4]interface{}{{"wght", 100.0, 400.0, 900.0}}

	// The control first, so the fixture is known to build a table this reads.
	if f := axisFace(t, fvarTable(1, 1, 20, good)); len(f.Axes()) != 1 || !f.IsVariable() {
		t.Fatalf("the good fvar gave %d axes and IsVariable=%v", len(f.Axes()), f.IsVariable())
	}

	many := make([][4]interface{}, 65)
	for i := range many {
		many[i] = [4]interface{}{"ax" + string(rune('0'+i%10)), 0.0, 0.0, 1.0}
	}
	for _, tc := range []struct {
		name string
		fvar []byte
	}{
		{"a version this does not read", fvarTable(2, 1, 20, good)},
		{"no axes at all", fvarTable(1, 0, 20, nil)},
		{"more axes than this reads", fvarTable(1, 65, 20, many)},
		{"records shorter than one axis", fvarTable(1, 1, 16, good)},
		{"a default outside its own range",
			fvarTable(1, 1, 20, [][4]interface{}{{"wght", 500.0, 400.0, 900.0}})},
		{"records that lie outside the table", fvarTable(1, 4, 20, good)},
		{"a header too short to hold itself", []byte{0, 1, 0, 0}},
	} {
		f := axisFace(t, tc.fvar)
		if got := f.Axes(); len(got) != 0 {
			t.Errorf("%s: Axes named %v; LoadInstance refuses this table, so "+
				"nothing may be asked of it", tc.name, got)
		}
		if f.IsVariable() {
			t.Errorf("%s: the face says it is variable", tc.name)
		}
		// And the other reader agrees, which is the whole point.
		if _, err := parseFvar(tc.fvar); err == nil {
			t.Errorf("%s: parseFvar accepted it, so this case is not what it "+
				"says it is", tc.name)
		}
	}
}

// TestAMarkFilteringSetIsReadWhereTheLookupPutIt.
//
// A lookup that filters by a mark glyph set names it after its subtable offsets,
// so where the number sits is decided by how many the lookup *declares*. The
// count is clamped on the way in — by the format's own maximum and by a budget
// shared across the table, which is what keeps a crafted font from asking for
// the product of two maxima — and the index was then read at the clamped
// position, which is two bytes of an offset.
func TestAMarkFilteringSetIsReadWhereTheLookupPutIt(t *testing.T) {
	const declared, want = 3, 0x1234
	lookup := make([]byte, 6+2*declared+2)
	binary.BigEndian.PutUint16(lookup[0:], 1)      // lookupType
	binary.BigEndian.PutUint16(lookup[2:], 0x0010) // UseMarkFilteringSet
	binary.BigEndian.PutUint16(lookup[4:], declared)
	for i := 0; i < declared; i++ {
		// Offsets that point nowhere useful: this is about where the index is
		// read, not about what the subtables hold.
		binary.BigEndian.PutUint16(lookup[6+2*i:], uint16(6+2*declared))
	}
	binary.BigEndian.PutUint16(lookup[6+2*declared:], want)

	// A budget that stops the reader after one subtable, which is the case: the
	// clamp moves where the old code looked and not where the font wrote.
	budget := 1
	if _, _, markSet, _ := subtables(lookup, 7, &budget); markSet != want {
		t.Errorf("the mark filtering set came back as %#x, want %#x: it is "+
			"written after the %d offsets the lookup declares, however many of "+
			"them were read", markSet, want, declared)
	}
	// And with no clamp at all, which has to give the same answer.
	if _, _, markSet, _ := subtables(lookup, 7, nil); markSet != want {
		t.Errorf("unclamped, the mark filtering set came back as %#x, want %#x",
			markSet, want)
	}
}

// TestAnInstanceIsNotShippedUnderTheDefaultsName.
//
// The PostScript name becomes /BaseFont in a PDF, and the name a variable font
// carries is the default instance's — so an instance that keeps it puts a
// document's bold text under a name that says Regular, and two weights of one
// face into one document under one name.
//
// A name table the rewriter cannot rebuild used to give exactly that: it
// returned nil, which the caller read as "nothing to change".
func TestAnInstanceIsNotShippedUnderTheDefaultsName(t *testing.T) {
	axes := []varAxis{{tag: "wght", min: 100, def: 400, max: 900}}
	want := map[string]float64{"wght": 700}

	// A name table whose record count is more than it has room for, which is
	// where replacePostScriptName gives up.
	broken := make([]byte, 6+12)
	binary.BigEndian.PutUint16(broken[0:], 0)  // format
	binary.BigEndian.PutUint16(broken[2:], 40) // count, far more than fits
	binary.BigEndian.PutUint16(broken[4:], 6)  // storage offset

	out, err := instanceName(broken, nil, axes, want)
	if err == nil {
		t.Errorf("a name table that cannot be rebuilt returned %v and no error; "+
			"the instance would go out under the default's name", out)
	} else if !strings.Contains(err.Error(), "name table") {
		t.Errorf("the error is %q, which does not say what went wrong", err)
	}

	// And the two cases that really are "nothing to change" still are.
	for _, tc := range []struct {
		name string
		want map[string]float64
	}{
		{"the default instance", map[string]float64{"wght": 400}},
		{"an axis the font does not have", map[string]float64{"wdth": 50}},
	} {
		out, err := instanceName(broken, nil, axes, tc.want)
		if out != nil || err != nil {
			t.Errorf("%s: got %v, %v; want nothing and no error", tc.name, out, err)
		}
	}
}
