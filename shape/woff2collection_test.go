package shape

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// encodedWOFF2Collections is each WOFF 2 collection google/woff2's encoder
// made (font/testdata/woff2-collections), with the collection it was made
// from.
func encodedWOFF2Collections(t testing.TB) map[string][2][]byte {
	t.Helper()
	dir := filepath.Join("..", "font", "testdata", "woff2-collections")
	files, err := filepath.Glob(filepath.Join(dir, "*.woff2"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][2][]byte{}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".woff2")
		woff2, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		ttc, err := os.ReadFile(filepath.Join(dir, name+".ttc"))
		if err != nil {
			t.Fatal(err)
		}
		out[name] = [2][]byte{woff2, ttc}
	}
	return out
}

// TestAFaceOfAnEncodedWOFF2CollectionIsTheFaceItWasMadeFrom loads each face
// of each WOFF 2 collection google/woff2's encoder made, and requires that it
// is described, shaped, painted and subsetted as the same face of the
// collection the encoder was given.
func TestAFaceOfAnEncodedWOFF2CollectionIsTheFaceItWasMadeFrom(t *testing.T) {
	colls := encodedWOFF2Collections(t)
	if len(colls) < 3 {
		t.Fatalf("%d encoded collections; font/testdata/woff2-collections/make.py makes three", len(colls))
	}
	for name, pair := range colls {
		woff2, ttc := pair[0], pair[1]
		got, err := CollectionFaces(woff2)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want, err := CollectionFaces(ttc)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s is described as %+v, and the collection it was made from as %+v", name, got, want)
		}
		for _, d := range want {
			a, err := LoadCollection(woff2, d.Index)
			if err != nil {
				t.Fatalf("%s face %d: %v", name, d.Index, err)
			}
			b, err := LoadCollection(ttc, d.Index)
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range []string{"ABC", "office", "AB"} {
				ga, ma := a.ShapeGlyphs(s)
				gb, mb := b.ShapeGlyphs(s)
				if ma != mb || !reflect.DeepEqual(ga, gb) {
					t.Errorf("%s face %d shapes %q differently", name, d.Index, s)
				}
			}
			for gid := range b.NumGlyphs() {
				pa, pb := &recordingPainter{}, &recordingPainter{}
				_ = a.PaintGlyph(gid, PaintOptions{}, pa)
				_ = b.PaintGlyph(gid, PaintOptions{}, pb)
				if !reflect.DeepEqual(pa.lines, pb.lines) {
					t.Errorf("%s face %d glyph %d paints differently", name, d.Index, gid)
				}
			}
			if _, err := a.Subset(); err != nil {
				t.Errorf("%s face %d does not subset: %v", name, d.Index, err)
			}
		}
	}
}
