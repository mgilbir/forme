package shape

import (
	"encoding/binary"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/internal/costtest"
)

// What a font collection costs when its offsets name the same bytes.
//
// A collection's header names each face's directory by an offset, a directory
// names each table by one, and a name table each string; nothing stops many
// of them from naming one place. Read in place, that costs nothing more; read
// or written once per name, it costs the place times the names, quadratic in
// the file. See CollectionFaces and describeAllowance.

// rawCollection is a collection header naming a directory at each of at, each
// an offset into body, which follows the header.
func rawCollection(at []int, body []byte) []byte {
	head := 12 + 4*len(at)
	out := make([]byte, head, head+len(body))
	copy(out, "ttcf")
	binary.BigEndian.PutUint32(out[4:], 0x00010000)
	binary.BigEndian.PutUint32(out[8:], uint32(len(at)))
	for i, a := range at {
		binary.BigEndian.PutUint32(out[12+4*i:], uint32(head+a))
	}
	return append(out, body...)
}

// directoryOf is a table directory of the tables given, by tag, each at the
// offset given from the start of the collection.
func directoryOf(tables map[string][2]int) []byte {
	tags := make([]string, 0, len(tables))
	for tag := range tables {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	dir := u16(binary.BigEndian.AppendUint32(nil, 0x00010000), len(tags), 0, 0, 0)
	for _, tag := range tags {
		dir = append(dir, tag...)
		dir = binary.BigEndian.AppendUint32(dir, 0)
		dir = binary.BigEndian.AppendUint32(dir, uint32(tables[tag][0]))
		dir = binary.BigEndian.AppendUint32(dir, uint32(tables[tag][1]))
	}
	return dir
}

// describing measures CollectionFaces over a collection built at n and at 4n,
// and requires the larger to cost at most 8 times the smaller, counted by the
// bytes allocated; check, where given, is asked of each description.
func describing(t *testing.T, what string, build func(n int) []byte, n int, check func(n int, faces []CollectionFace)) {
	t.Helper()
	at := func(n int) func() {
		data := build(n)
		return func() {
			faces, err := CollectionFaces(data)
			if err != nil {
				t.Fatal(err)
			}
			if check != nil {
				check(n, faces)
			}
		}
	}
	if r := costtest.Allocated(t, what, at(n), at(4*n)); r > 8 {
		t.Errorf("%s: a factor of %.1f where 8 is the most the input allows", what, r)
	}
}

// TestEachFaceOfACollectionReadsItsOwnOffset describes a collection of n faces
// whose directories cannot be read, at 4n against n. Each face read every
// face's offset to find its own, n² for a header that may state a face for
// every four bytes of the file.
func TestEachFaceOfACollectionReadsItsOwnOffset(t *testing.T) {
	describing(t, "describing n faces at n offsets with nothing there, at 4n against n", func(n int) []byte {
		at := make([]int, n)
		for i := range at {
			at[i] = i
		}
		return rawCollection(at, make([]byte, n+16))
	}, 1000, nil)
}

// TestFacesOnOneDirectoryReadItOnce describes n faces all naming one directory
// of n tables, at 4n against n: each read the directory afresh, n×n. Read once,
// every face is still described, as the one directory says.
func TestFacesOnOneDirectoryReadItOnce(t *testing.T) {
	describing(t, "describing n faces on one directory of n tables, at 4n against n", func(n int) []byte {
		name := nameTableOf(win(6, "Shared-Directory"))
		tables := map[string][2]int{}
		for i := 0; i < n-1; i++ {
			tables[fmt.Sprintf("X%03d", i)] = [2]int{0, 0}
		}
		head := 12 + 4*n
		tables["name"] = [2]int{head + 12 + 16*n, len(name)}
		return rawCollection(make([]int, n), append(directoryOf(tables), name...))
	}, 250, func(n int, faces []CollectionFace) {
		for i, f := range faces {
			if f.Index != i || f.Name != "Shared-Directory" {
				t.Fatalf("face %d of %d on one directory is described as %+v", i, n, f)
			}
		}
	})
}

// TestFacesOnOneNameTableReadItOnce describes n faces, each with a directory of
// its own, all naming one name table whose family name is 8n characters,
// at 4n against n: each face read the table afresh, n reads of a table the
// size of n. Read once, every face is still named by it.
func TestFacesOnOneNameTableReadItOnce(t *testing.T) {
	long := func(n int) string { return strings.Repeat("N", 8*n) }
	describing(t, "describing n faces naming one name table of 16n bytes, at 4n against n", func(n int) []byte {
		name := nameTableOf(win(6, "Shared-Name"), win(1, long(n)))
		head := 12 + 4*n
		at := make([]int, n)
		var body []byte
		for i := range at {
			at[i] = 28 * i
			body = append(body, directoryOf(map[string][2]int{"name": {head + 28*n, len(name)}})...)
		}
		return rawCollection(at, append(body, name...))
	}, 250, func(n int, faces []CollectionFace) {
		family := long(n)
		for i, f := range faces {
			if f.Index != i || f.Name != "Shared-Name" || f.Family != family {
				t.Fatalf("face %d of %d on one name table is described as %q, of a family of %d characters",
					i, n, f.Name, len(f.Family))
			}
		}
	})
}

// TestOverlappingDirectoriesAreReadWithinTheAllowance describes n faces whose
// directories overlap, at 4n against n: every sixteen bytes of the body is a
// directory header stating n tables, which are the next n sixteen-byte blocks,
// each read as a header too. No two faces share an offset, so reading each
// directory once does not help: n faces read n records each. What is read is
// charged against describeAllowance, and a face past it described by its
// index.
func TestOverlappingDirectoriesAreReadWithinTheAllowance(t *testing.T) {
	describing(t, "describing n faces on overlapping directories of n tables, at 4n against n", func(n int) []byte {
		block := u16(binary.BigEndian.AppendUint32(nil, 0x74727565), n, 0, 0, 0, 0, 0) // 'true'
		at := make([]int, n)
		var body []byte
		for i := range at {
			at[i] = 16 * i
			body = append(body, block...)
		}
		for range n {
			body = append(body, block...)
		}
		return rawCollection(at, body)
	}, 250, nil)
}

// realFonts is every font file in the tree, and in the corpora where they are
// fetched: single fonts, collections, and either wrapped as WOFF or WOFF 2.
func realFonts(t *testing.T) []string {
	t.Helper()
	var files []string
	walk := func(root string) {
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			switch strings.ToLower(filepath.Ext(path)) {
			case ".ttf", ".otf", ".ttc", ".otc", ".woff", ".woff2":
				files = append(files, path)
			}
			return nil
		})
	}
	// The Google Fonts checkout is walked from its own root, with the
	// trailing slash, since WalkDir does not follow a symlinked one.
	for _, dir := range []string{"../testdata/", "../fonts/", "../font/testdata/", "../testdata/googlefonts/",
		os.Getenv("NOTO_FONTS"), os.Getenv("NOTO_CJK"), os.Getenv("CFF_FONTS"), os.Getenv("EMOJI_FONTS")} {
		if dir != "" {
			walk(dir + string(filepath.Separator))
		}
	}
	return files
}

// TestNoRealCollectionIsDescribedPastTheAllowance is the other half of the
// tests above: every face of every collection in the tree and the corpora is
// described as it is read alone, by its own directory and name table, so
// neither reading a directory or a name table once for the faces sharing it
// nor describeAllowance changes what an honest collection is described as.
func TestNoRealCollectionIsDescribedPastTheAllowance(t *testing.T) {
	collections, faces := 0, 0
	for _, path := range realFonts(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		data, err := unwrapWOFF(raw)
		if err != nil || font.CollectionOffsets(data) == nil {
			continue // a single font, or a fixture of a malformed WOFF
		}
		collections++
		got, err := CollectionFaces(raw)
		if err != nil {
			t.Fatalf("%s: %v", filepath.Base(path), err)
		}
		for i := range got {
			faces++
			want := CollectionFace{Index: i}
			if tables := font.CollectionTables(data, i); tables != nil {
				want = describeTables(i, tables, readFaceNames(tables["name"]))
			}
			if !reflect.DeepEqual(got[i], want) {
				t.Errorf("%s: face %d is described as %+v, and read alone as %+v", filepath.Base(path), i, got[i], want)
			}
		}
	}
	// The tree holds fifteen; fewer means the walk found nothing, and a test
	// that reads nothing passes whatever the bound.
	if collections < 15 {
		t.Fatalf("%d collections were read, which proves little", collections)
	}
	t.Logf("%d faces of %d collections described as read alone", faces, collections)
}
