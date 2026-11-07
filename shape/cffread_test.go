package shape

import (
	"bytes"
	"fmt"
	"sync"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// cffFaces are a CFF face that keeps its numbering and a CID-keyed one, which
// a subset renumbers: the two ways subsetCFF writes a table.
func cffFaces(t *testing.T) map[string][]byte {
	return map[string][]byte{
		"SourceSans3-Regular": fonttest.CFFFile(t, "SourceSans3-Regular.otf"),
		"NotoSansJP-Regular":  fonttest.CJKFile(t, "NotoSansJP-Regular.otf"),
	}
}

// TestClonesShareOneReadOfTheCFF (#987): a CFF face's clones read its CFF
// table through one reader, and a subset served from what an earlier subset
// read is the subset a face read cold writes.
func TestClonesShareOneReadOfTheCFF(t *testing.T) {
	for name, data := range cffFaces(t) {
		t.Run(name, func(t *testing.T) {
			warm, err := Load(data)
			if err != nil {
				t.Fatal(err)
			}
			for i, text := range []string{"Measurement 0123456789 日本語", "ÀÉÎõü", "a"} {
				c := warm.Clone()
				c.ShapeGlyphs(text)
				got, _, err := c.SubsetGlyphs()
				if err != nil {
					t.Fatal(err)
				}
				cold, err := Load(data)
				if err != nil {
					t.Fatal(err)
				}
				cold.ShapeGlyphs(text)
				want, _, err := cold.SubsetGlyphs()
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, want) {
					t.Errorf("subset %d: %d bytes from the shared read, %d from a cold one", i, len(got), len(want))
				}
			}
			r := warm.checks.cff
			if r == nil || len(r.indexes) == 0 {
				t.Fatal("the face kept nothing of its CFF table")
			}
			if c := warm.Clone(); c.cffReaderFor(r.data) != r {
				t.Error("a clone reads the CFF table again")
			}
		})
	}
}

// TestASharedCFFReadChargesWhatAColdOneDoes: what a subset is charged does not
// depend on whether the table was read for it, so a subset's limits mean what
// they meant.
func TestASharedCFFReadChargesWhatAColdOneDoes(t *testing.T) {
	for name, data := range cffFaces(t) {
		t.Run(name, func(t *testing.T) {
			cff := font.SFNTTables(data)["CFF "]
			f, err := Load(data)
			if err != nil {
				t.Fatal(err)
			}
			keep := make([]bool, f.NumGlyphs())
			for _, r := range "Measurement 0123456789 日本語" {
				if g, ok := f.CmapGlyph(r); ok {
					keep[g] = true
				}
			}
			spent := func(r *cffReader) (int, []byte) {
				b := fullBudget()
				k := append([]bool(nil), keep...)
				if err := cffSeacClosureFrom(r, k, b); err != nil {
					t.Fatal(err)
				}
				out, _, err := subsetCFFFrom(r, k, b)
				if err != nil {
					t.Fatal(err)
				}
				return b.Spent(), out
			}
			shared := newCFFReader(cff)
			coldSpent, coldOut := spent(shared)
			for i := 0; i < 2; i++ {
				if s, out := spent(shared); s != coldSpent || !bytes.Equal(out, coldOut) {
					t.Errorf("a subset from what was read already spent %d units, a cold one %d (same table: %v)",
						s, coldSpent, bytes.Equal(out, coldOut))
				}
			}
		})
	}
}

// TestClonesSubsetConcurrently: clones of one CFF face subset at once, each
// writing what it writes alone. Run with -race, it is the reader's lock that
// is tested.
func TestClonesSubsetConcurrently(t *testing.T) {
	data := fonttest.CFFFile(t, "SourceSans3-Regular.otf")
	texts := []string{"Measurement", "0123456789", "ÀÉÎõü", "the quick brown fox", "a", "QZ"}
	want := map[string][]byte{}
	for _, text := range texts {
		f, err := Load(data)
		if err != nil {
			t.Fatal(err)
		}
		f.ShapeGlyphs(text)
		if want[text], _, err = f.SubsetGlyphs(); err != nil {
			t.Fatal(err)
		}
	}
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 4*len(texts))
	for range 4 {
		for _, text := range texts {
			wg.Go(func() {
				c := f.Clone()
				c.ShapeGlyphs(text)
				got, _, err := c.SubsetGlyphs()
				switch {
				case err != nil:
					errs <- err
				case !bytes.Equal(got, want[text]):
					errs <- fmt.Errorf("%q: %d bytes, alone %d", text, len(got), len(want[text]))
				}
			})
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// TestTheCFFReaderKeepsNoMoreThanTheTable: INDEXes that overlap — every offset
// into a run of 0x01 bytes reads as one of 257 empty items — are read as often
// as they are asked for, and kept only while the items kept come to no more
// than the table's length.
func TestTheCFFReaderKeepsNoMoreThanTheTable(t *testing.T) {
	data := bytes.Repeat([]byte{1}, 303)
	r := newCFFReader(data)
	for off := 0; off < 40; off++ {
		idx, err := r.index(off)
		if err != nil {
			t.Fatalf("offset %d: %v", off, err)
		}
		if len(idx.items) != 257 {
			t.Fatalf("offset %d reads %d items, want 257", off, len(idx.items))
		}
	}
	if r.kept > len(data) || len(r.indexes) != 1 {
		t.Errorf("the reader keeps %d INDEXes of %d items, for a table of %d bytes", len(r.indexes), r.kept, len(data))
	}
}
