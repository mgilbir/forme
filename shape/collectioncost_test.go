package shape

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/internal/costtest"
)

// What a font collection costs when its offsets name the same bytes.
//
// A collection's header names each face's directory by an offset, a directory
// names each table by one, and a name table each string; nothing stops many
// of them from naming one place. Read in place, that costs nothing more; read
// or written once per name, it costs the place times the names, quadratic in
// the file. See font.SFNTTables, CollectionFaces and describeAllowance.

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

// aliasedSFNT is src's tables written out again with n more tables, X000 on,
// every one of them naming one range of size bytes after the rest, and as a
// collection of that one face where collection is set.
func aliasedSFNT(src []byte, n, size int, collection bool) []byte {
	tables := font.SFNTTables(src)
	tags := make([]string, 0, len(tables))
	for tag := range tables {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	dirAt := 0
	if collection {
		dirAt = 16
	}
	count := len(tags) + n
	out := make([]byte, dirAt+12+16*count)
	if collection {
		copy(out, "ttcf")
		binary.BigEndian.PutUint32(out[4:], 0x00010000)
		binary.BigEndian.PutUint32(out[8:], 1)
		binary.BigEndian.PutUint32(out[12:], uint32(dirAt))
	}
	copy(out[dirAt:], src[:4])
	binary.BigEndian.PutUint16(out[dirAt+4:], uint16(count))
	record := func(i int, tag string, off, length int) {
		rec := dirAt + 12 + 16*i
		copy(out[rec:], tag)
		binary.BigEndian.PutUint32(out[rec+8:], uint32(off))
		binary.BigEndian.PutUint32(out[rec+12:], uint32(length))
	}
	for i, tag := range tags {
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
		record(i, tag, len(out), len(tables[tag]))
		out = append(out, tables[tag]...)
	}
	for len(out)%4 != 0 {
		out = append(out, 0)
	}
	at := len(out)
	for i := 0; i < size; i++ {
		out = append(out, byte(i))
	}
	for i := 0; i < n; i++ {
		record(len(tags)+i, fmt.Sprintf("X%03d", i), at, size)
	}
	return out
}

// TestTablesNamingOneRangeAreWrittenWithinTheFile writes out a font whose n
// extra tables all name one range of 64n bytes, at 4n against n, along each
// path that copies a font's tables into a font of its own: the program of a
// face of a collection, an instance, and a CFF2 font's program. Each copied
// every table it was given, so the copies grew as n×64n, sixteen times for
// four: a 33 KB collection of 400 tables on one range of 25,600 bytes made a
// program of ten megabytes, and writing it allocated 56. A directory
// whose tables are more than twice its file is refused now (font.SFNTTables),
// so what is copied is at most twice the file. Counted by the bytes
// allocated, which every copy is.
func TestTablesNamingOneRangeAreWrittenWithinTheFile(t *testing.T) {
	small := fonttest.SFNT(fonttest.SFNTOptions{Name: "Cost", Glyphs: costGlyphs})
	for _, c := range []struct {
		path       string
		src        []byte
		collection bool
		write      func(data []byte) error
	}{
		{"a face of a collection's program", small, true, func(data []byte) error {
			f, err := LoadCollection(data, 0)
			if err == nil && len(f.Program()) == 0 {
				err = errors.New("no program")
			}
			return err
		}},
		{"an instance", harfbuzzFont(t, "VariedAxes.ttf"), false, func(data []byte) error {
			_, err := LoadInstance(data, nil)
			return err
		}},
		{"a CFF2 font's program", harfbuzzFont(t, "CFF2Blend.otf"), false, func(data []byte) error {
			f, err := Load(data)
			if err == nil && len(f.Program()) == 0 {
				err = errors.New("no program")
			}
			return err
		}},
	} {
		t.Run(c.path, func(t *testing.T) {
			// The path works on the font as it is, so that a refusal below is
			// about the aliasing and not about the font.
			if err := c.write(aliasedSFNT(c.src, 0, 0, c.collection)); err != nil {
				t.Fatalf("the font written out again: %v", err)
			}
			at := func(n int) func() {
				data := aliasedSFNT(c.src, n, 64*n, c.collection)
				return func() { c.write(data) }
			}
			what := c.path + " with n tables naming one range of 64n bytes, at 4n against n"
			if r := costtest.Allocated(t, what, at(100), at(400)); r > 8 {
				t.Errorf("%s: a factor of %.1f where 8 is the most the input allows", what, r)
			}
		})
	}
}

// TestNoRealFontsTablesAreMoreThanItsFile is the other half of
// TestTablesNamingOneRangeAreWrittenWithinTheFile: no font's honest tables
// overlap past its file, so the refusal, at twice the file, touches none.
// Every directory of every font in the tree and the corpora, single or a face
// of a collection, is within its file once, and read.
func TestNoRealFontsTablesAreMoreThanItsFile(t *testing.T) {
	dirs := 0
	for _, path := range realFonts(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		data, err := unwrapWOFF(raw)
		if err != nil {
			continue // a fixture of a malformed WOFF, refused before any directory
		}
		offsets := font.CollectionOffsets(data)
		read := func(i int) map[string][]byte { return font.CollectionTables(data, i) }
		if offsets == nil {
			offsets = []int{0}
			read = func(int) map[string][]byte { return font.SFNTTables(data) }
		}
		for i, at := range offsets {
			if at < 0 || at > len(data)-12 || 12+16*font.Be16(data, at+4) > len(data)-at {
				continue // a directory refused for running past the file
			}
			switch font.Be32(data, at) {
			case 0x00010000, 0x74727565, 0x4F54544F:
			default:
				continue
			}
			dirs++
			// What the records state, read here rather than by the reader under
			// test: each tag's last record, cut at the end of the file.
			lengths := map[string]int{}
			for r := 0; r < font.Be16(data, at+4); r++ {
				rec := at + 12 + 16*r
				off, n := int(font.Be32(data, rec+8)), int(font.Be32(data, rec+12))
				if off < len(data) {
					lengths[string(data[rec:rec+4])] = min(n, len(data)-off)
				}
			}
			sum := 0
			for _, n := range lengths {
				sum += n
			}
			if sum > len(data) {
				t.Errorf("%s, face %d: its tables are %d bytes, in a file of %d", filepath.Base(path), i, sum, len(data))
			}
			if read(i) == nil {
				t.Errorf("%s, face %d: its directory was refused", filepath.Base(path), i)
			}
		}
	}
	// The tree's own fonts are more than these; fewer means the walk found
	// nothing, and a test that reads nothing passes whatever the bound.
	if dirs < 100 {
		t.Fatalf("%d directories were read, which proves little", dirs)
	}
	t.Logf("%d directories within their files, and read", dirs)
}

// TestAFontRefusedForItsTablesSaysSo is the error a refused directory gets: a
// font whose tables overlap past twice its size is an sfnt, and calling it
// "not an sfnt" would send someone looking for a corrupt file.
func TestAFontRefusedForItsTablesSaysSo(t *testing.T) {
	small := fonttest.SFNT(fonttest.SFNTOptions{Name: "Cost", Glyphs: costGlyphs})
	data := aliasedSFNT(small, 8, 4*len(small), false)
	if font.SFNTTables(data) != nil {
		t.Fatal("the font's directory was read, so it says nothing about the refusal")
	}
	for _, c := range []struct {
		path string
		load func() error
	}{
		{"Load", func() error { _, err := Load(data); return err }},
		{"LoadInstance", func() error { _, err := LoadInstance(data, nil); return err }},
	} {
		err := c.load()
		if err == nil || !strings.Contains(err.Error(), "overlap") {
			t.Errorf("%s: %v, want the overlap named", c.path, err)
		}
	}
	if _, err := Load([]byte("this is not a font")); err == nil || !strings.Contains(err.Error(), "not an sfnt") {
		t.Errorf("Load of text: %v, want \"not an sfnt\"", err)
	}
}
