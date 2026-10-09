package shape

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/internal/costtest"
)

// overlappingStore is an item variation store of g groups whose offsets are six
// bytes apart over one block of the words 0, 0, k repeated, k being 3g. The
// three words at each offset read as a group of no items and k region indices,
// and the indices it reads are the block's own words, all of them less than
// the k+1 regions the store declares. Every group is well-formed by itself,
// and none is at an offset another is: a store of about 12g bytes that names
// 3g² region indices between its groups.
func overlappingStore(g int) []byte {
	k := 3 * g
	be32 := func(v int) []byte { return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)} }
	regionsOff := 8 + 4*g
	t := u16(nil, 1)
	t = append(t, be32(regionsOff)...)
	t = u16(t, g)
	for i := 0; i < g; i++ {
		t = append(t, be32(regionsOff+4+6*i)...)
	}
	t = u16(t, 0, k+1) // no axes, so k+1 regions cost no bytes
	for i := 0; i < g+k; i++ {
		t = u16(t, 0, 0, k)
	}
	return t
}

// TestVariationStoreGroupsOverlappingAreChargedToTheStoresSize is a defect the
// memo in parseVarStore does not reach: it shares a group between offsets that
// are equal, and these are each a few bytes from the last. Every group read
// its region list into a slice of its own, so four times the groups was
// sixteen times the memory — 99 MB for a 56 KB store, and eleven gigabytes for
// the largest the format allows in under a megabyte.
//
// What the groups of a store may name between them is bounded by its size, so
// that the memory follows the bytes: four times the store is about four times
// the allocation.
func TestVariationStoreGroupsOverlappingAreChargedToTheStoresSize(t *testing.T) {
	load := func(g int) func() {
		data := overlappingStore(g)
		return func() {
			// The store is refused once its allowance runs out; what the
			// cost is bounded by is asked, and the refusal is checked below.
			parseVarStore(data)
		}
	}
	const what = "parsing a store of n overlapping groups, at 4n against n"
	if r := costtest.Allocated(t, what, load(500), load(2000)); r > 8 {
		t.Errorf("%s: a factor of %.1f where 8 is the most the input allows", what, r)
	}
	if _, err := parseVarStore(overlappingStore(2000)); err == nil {
		t.Error("a store naming 3g² regions in 12g bytes was read whole")
	}
}

// TestNoRealFontReachesTheVariationStoreAllowance is the other half: the
// allowance is for a store whose groups overlap, and every store in the tree
// and the corpora — HVAR, VVAR, MVAR, the GDEF's, COLR's and CFF2's — must be
// read whole, with room to spare.
func TestNoRealFontReachesTheVariationStoreAllowance(t *testing.T) {
	files, _ := filepath.Glob("../fonts/notosans/*.ttf")
	for _, dir := range []string{"../testdata/harfbuzz/fonts", os.Getenv("NOTO_FONTS"),
		os.Getenv("NOTO_CJK"), os.Getenv("CFF_FONTS"), os.Getenv("EMOJI_FONTS")} {
		if dir == "" {
			continue
		}
		for _, ext := range []string{"*.ttf", "*.otf"} {
			m, _ := filepath.Glob(filepath.Join(dir, ext))
			files = append(files, m...)
		}
	}
	// The Google Fonts checkout, where it has been fetched, holds most of the
	// variable fonts there are, and is walked whole.
	filepath.WalkDir("../testdata/googlefonts/", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && (strings.HasSuffix(path, ".ttf") || strings.HasSuffix(path, ".otf")) {
			files = append(files, path)
		}
		return nil
	})
	stores, cff2s, worst := 0, 0, 0.0
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		tables := font.SFNTTables(data)
		var found [][]byte
		for _, tag := range []string{"HVAR", "VVAR"} {
			if h := tables[tag]; len(h) >= 8 {
				if off := int(font.Be32(h, 4)); off > 0 && off < len(h) {
					found = append(found, h[off:])
				}
			}
		}
		if m := tables["MVAR"]; len(m) >= 12 {
			if off := font.Be16(m, 10); off > 0 && off < len(m) {
				found = append(found, m[off:])
			}
		}
		if g := tables["GDEF"]; len(g) >= 18 && font.Be16(g, 0) == 1 && font.Be16(g, 2) >= 3 {
			if off := int(font.Be32(g, 14)); off > 0 && off < len(g) {
				found = append(found, g[off:])
			}
		}
		// COLR's store, at the offset version 1 states at byte 30.
		if c := tables["COLR"]; len(c) >= 34 && font.Be16(c, 0) >= 1 {
			if off := int(font.Be32(c, 30)); off > 0 && off < len(c) {
				found = append(found, c[off:])
			}
		}
		// CFF2's store is found through its top DICT, which readCFF2 reads;
		// one it refuses refuses the font.
		if c := tables["CFF2"]; c != nil {
			if maxp := tables["maxp"]; len(maxp) >= 6 {
				cff2s++
				if _, err := readCFF2(c, font.Be16(maxp, 4), font.NewBudget(1<<30)); err != nil &&
					strings.Contains(err.Error(), "variation store") {
					t.Errorf("%s: its CFF2 store was refused: %v", filepath.Base(path), err)
				}
			}
		}
		for _, store := range found {
			stores++
			s, err := parseVarStore(store)
			if err != nil {
				t.Errorf("%s: a store was refused: %v", filepath.Base(path), err)
				continue
			}
			named := 0
			for _, d := range s.data {
				named += len(d.regions)
			}
			worst = max(worst, float64(named)/float64(varStoreRegionAllowance(store)))
		}
	}
	// The tree's Noto Sans is variable and has an HVAR and an MVAR; none read
	// means the glob stopped finding them, and a test that reads nothing passes
	// whatever the bound.
	if stores == 0 {
		t.Fatal("no variation store was read, which proves little")
	}
	t.Logf("%d stores read whole, the most using %.3f of the allowance, and %d CFF2 tables",
		stores, worst, cff2s)
}
