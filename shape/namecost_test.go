package shape

import (
	"encoding/binary"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/internal/costtest"
)

// What reading a name table costs when its records share one string.
//
// A name record finds its string by an offset, and nothing stops every record
// from pointing at one string: each is bytes the table has. The readers decoded
// each record's string whole, so n records on a string of length L cost n×L —
// quadratic in the table, since both grow with it — and Load reads the table
// five times. See nameReadAllowance.

// sharedStringName is a name table of n Windows English records, cycling
// through the IDs Load and Descriptor read, every one pointing at one string of
// strLen bytes of NUL, which decodes to nothing. A string that reads as nothing
// wins nothing, so no reader can stop early on it.
func sharedStringName(n, strLen int) []byte {
	ids := []int{1, 2, 6, 16, 17}
	storage := 6 + 12*n
	b := u16(nil, 0, n, storage)
	for i := 0; i < n; i++ {
		b = u16(b, 3, 1, 0x409, ids[i%len(ids)], strLen, 0)
	}
	return append(b, make([]byte, strLen)...)
}

// TestNameRecordsSharingAStringAreReadWithinTheTablesSize loads a face whose
// n records share one string of 16n bytes, at 4n against n: the table grows
// four times and the reads sixteen, unbounded. Counted by the bytes allocated,
// which every decode of a string allocates for.
func TestNameRecordsSharingAStringAreReadWithinTheTablesSize(t *testing.T) {
	load := func(n int) func() {
		data := fonttest.SFNT(fonttest.SFNTOptions{Name: "Cost", Glyphs: costGlyphs,
			Extra: map[string][]byte{"name": sharedStringName(n, 16*n)}})
		return func() {
			f, err := Load(data)
			if err != nil {
				t.Fatal(err)
			}
			f.NamedInstance(func(string) bool { return false })
		}
	}
	const what = "loading a face whose n name records share one string of 16n bytes, at 4n against n"
	if r := costtest.Allocated(t, what, load(1000), load(4000)); r > 8 {
		t.Errorf("%s: a factor of %.1f where 8 is the most the input allows", what, r)
	}
}

// TestEachNameReaderIsLinearInTheTable asks each reader directly, at 4n
// against n, rather than through Load: NamedInstance reads every record with
// nameStrings, but returns before it for a face with no design space, so Load
// alone never reaches it.
func TestEachNameReaderIsLinearInTheTable(t *testing.T) {
	for _, c := range []struct {
		reader string
		read   func([]byte)
		// timed: nameByID keeps only the ASCII of a string, which a string of
		// NUL has none of, so it allocates nothing and its cost is its time.
		timed bool
	}{
		{"readName", func(name []byte) { readName(name, 16) }, false},
		{"nameByID", func(name []byte) { nameByID(name, 6) }, true},
		{"nameStrings", func(name []byte) { nameStrings(name) }, false},
	} {
		t.Run(c.reader, func(t *testing.T) {
			at := func(n int) func() {
				name := sharedStringName(n, 16*n)
				return func() { c.read(name) }
			}
			what := c.reader + " over n records sharing one string of 16n bytes, at 4n against n"
			var r float64
			if c.timed {
				r = costtest.Time(t, what, at(1000), at(4000)).Ratio
			} else {
				r = costtest.Allocated(t, what, at(1000), at(4000))
			}
			if r > 8 {
				t.Errorf("%s: a factor of %.1f where 8 is the most the input allows", what, r)
			}
		})
	}
}

// TestARebuiltNameTableWritesASharedStringOnce is the same shape in the
// rewriter an instance is named by, which copied each record's string: a
// thousand records on one 60 KB string became sixty megabytes of storage
// before a check refused it. Shared strings are written once now, so the table
// rebuilds, at the size of what it says, and every record still names its
// string.
func TestARebuiltNameTableWritesASharedStringOnce(t *testing.T) {
	const n, strLen = 1000, 60000
	name := sharedStringName(n, strLen)
	// A readable PostScript name to be replaced, stated by a record of its own
	// at the end of storage.
	ps := u16(nil, 'O', 'l', 'd')
	binary.BigEndian.PutUint16(name[6+12*2+8:], uint16(len(ps)))
	binary.BigEndian.PutUint16(name[6+12*2+10:], uint16(strLen))
	name = append(name, ps...)

	out := replacePostScriptName(name, "New-Name")
	if out == nil {
		t.Fatal("a table whose records share one string was not rebuilt")
	}
	if len(out) > len(name)+64 {
		t.Errorf("the rebuilt table is %d bytes from %d: the shared string was copied", len(out), len(name))
	}
	if got := postScriptName(out); got != "New-Name" {
		t.Errorf("the rebuilt table names %q, want New-Name", got)
	}
	count, storage := font.Be16(out, 2), font.Be16(out, 4)
	for i := 0; i < count; i++ {
		rec := 6 + 12*i
		if font.Be16(out, rec+6) == 6 {
			continue
		}
		length, off := font.Be16(out, rec+8), storage+font.Be16(out, rec+10)
		if length != strLen || off+length > len(out) {
			t.Fatalf("record %d names %d bytes at %d in a table of %d", i, length, off, len(out))
		}
	}
}

// TestNoRealFontReachesTheNameAllowance is the other half: the allowance is
// there for records that overlap, and no font's honest records may reach it.
// Every name table in the tree, and in the corpora where they are fetched,
// names less between all its records than one read of it may decode.
func TestNoRealFontReachesTheNameAllowance(t *testing.T) {
	var files []string
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
	// The Google Fonts checkout, where it has been fetched, is walked whole:
	// it is the 3,874 fonts nameReadAllowance was measured against.
	filepath.WalkDir("../testdata/googlefonts/", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && (strings.HasSuffix(path, ".ttf") || strings.HasSuffix(path, ".otf")) {
			files = append(files, path)
		}
		return nil
	})
	tables := 0
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		name := font.SFNTTables(data)["name"]
		if len(name) < 6 {
			continue
		}
		tables++
		named := 0
		count, storage := font.Be16(name, 2), font.Be16(name, 4)
		for i := 0; i < count && 6+12*i+12 <= len(name); i++ {
			rec := 6 + 12*i
			if length := font.Be16(name, rec+8); storage+font.Be16(name, rec+10)+length <= len(name) {
				named += length
			}
		}
		if named > nameReadAllowance(name) {
			t.Errorf("%s: its records name %d bytes, past the %d one read may decode",
				filepath.Base(path), named, nameReadAllowance(name))
		}
	}
	// The tree's own fonts hold these; fewer means the glob found nothing,
	// and a test that reads nothing passes whatever the bound.
	if tables < 15 {
		t.Fatalf("only %d name tables were read, which proves little", tables)
	}
	t.Logf("%d name tables within the allowance", tables)
}
