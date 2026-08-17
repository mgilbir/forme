package font

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// WOFF 1.0.
//
// The format is an sfnt taken apart and deflated table by table, so the thing to
// prove is that what comes out is what went in — and the thing to guard is that
// a file's stated sizes never become an allocation, since origLength is the
// output length of a deflate stream and a forty-byte table can claim four
// gigabytes.

func woffTables(t *testing.T, woff []byte) map[string][]byte {
	t.Helper()
	sfnt, err := DecodeWOFF(woff)
	if err != nil {
		t.Fatalf("DecodeWOFF: %v", err)
	}
	tables := SFNTTables(sfnt)
	if tables == nil {
		t.Fatalf("the decoded font is not an sfnt")
	}
	return tables
}

// TestWOFFGivesBackWhatWentIn, table for table and byte for byte.
func TestWOFFGivesBackWhatWentIn(t *testing.T) {
	want := map[string][]byte{
		"head": []byte("this is the head table, and it is long enough to deflate well"),
		"glyf": make([]byte, 5000), // compresses hugely
		"cmap": {1, 2, 3},          // too short to be worth deflating: stored
		"OS/2": []byte("a tag with a slash and a digit in it"),
	}
	var in []fonttest.WOFFTable
	for tag, data := range want {
		in = append(in, fonttest.WOFFTable{Tag: tag, Data: data})
	}
	got := woffTables(t, fonttest.WOFF(fonttest.WOFFOptions{Tables: in}))
	if len(got) != len(want) {
		t.Fatalf("%d tables out, %d in: %v", len(got), len(want), keysOf(got))
	}
	for tag, w := range want {
		g, ok := got[tag]
		if !ok {
			t.Errorf("table %q is missing", tag)
			continue
		}
		if len(g) != len(w) {
			t.Errorf("table %q is %d bytes, was %d", tag, len(g), len(w))
			continue
		}
		if string(g) != string(w) {
			t.Errorf("table %q came back with different content", tag)
		}
	}
}

// TestWOFFTablesAreFourByteAligned. An sfnt's table offsets must be multiples of
// four — some readers map tables directly and a misaligned one is a fault on
// architectures that care — and a WOFF stores no padding, so the decoder is what
// puts it back.
func TestWOFFTablesAreFourByteAligned(t *testing.T) {
	// Lengths chosen to leave every possible remainder mod 4.
	var in []fonttest.WOFFTable
	for i, n := range []int{1, 2, 3, 4, 5} {
		in = append(in, fonttest.WOFFTable{
			Tag: string(rune('a'+i)) + "bcd", Data: make([]byte, n), Store: true,
		})
	}
	sfnt, err := DecodeWOFF(fonttest.WOFF(fonttest.WOFFOptions{Tables: in}))
	if err != nil {
		t.Fatalf("DecodeWOFF: %v", err)
	}
	n := int(binary.BigEndian.Uint16(sfnt[4:]))
	for i := 0; i < n; i++ {
		rec := 12 + 16*i
		off := binary.BigEndian.Uint32(sfnt[rec+8:])
		length := binary.BigEndian.Uint32(sfnt[rec+12:])
		if off%4 != 0 {
			t.Errorf("table %d starts at %d, which is not a multiple of four", i, off)
		}
		if uint64(off)+uint64(length) > uint64(len(sfnt)) {
			t.Errorf("table %d runs past the end of the font", i)
		}
	}
}

// TestWOFFSortsItsDirectory. An sfnt's records must be in tag order because a
// reader may binary-search them. A WOFF whose directory is merely out of order
// is still perfectly readable, so it is sorted rather than refused — refusing
// would lose a face over the order of a list nothing has read yet.
func TestWOFFSortsItsDirectory(t *testing.T) {
	sfnt, err := DecodeWOFF(fonttest.WOFF(fonttest.WOFFOptions{
		Unsorted: true,
		Tables: []fonttest.WOFFTable{
			{Tag: "post", Data: []byte("p")},
			{Tag: "cmap", Data: []byte("c")},
			{Tag: "head", Data: []byte("h")},
		},
	}))
	if err != nil {
		t.Fatalf("an out-of-order directory was refused: %v", err)
	}
	n := int(binary.BigEndian.Uint16(sfnt[4:]))
	var prev uint32
	for i := 0; i < n; i++ {
		tag := binary.BigEndian.Uint32(sfnt[12+16*i:])
		if i > 0 && tag <= prev {
			t.Errorf("record %d has tag %08x after %08x; the records are not sorted",
				i, tag, prev)
		}
		prev = tag
	}
}

// TestWOFFRefusesADecompressionBomb is the one that matters.
//
// The table deflates from four megabytes of zeros into a few kilobytes and
// declares an origLength of 65536 — above the compressed length, so the
// container is well formed and nothing is wrong until the bytes arrive. A
// decoder that trusted the header would
// read the stream to its end into a buffer that grew as it went; one that sized
// a buffer from origLength and stopped would silently truncate a table and hand
// on a font built from part of one.
func TestWOFFRefusesADecompressionBomb(t *testing.T) {
	_, err := DecodeWOFF(fonttest.WOFFBomb(4<<20, 1<<16))
	if err == nil {
		t.Fatal("a table that decompressed to four megabytes while declaring 65536 " +
			"bytes was accepted")
	}
	if !strings.Contains(err.Error(), "declared") {
		t.Errorf("the bomb was refused with %q, which does not say what was wrong", err)
	}
}

// TestWOFFRefusesATableShorterThanItDeclared. The other direction: a stream that
// ends early leaves a table padded with whatever the buffer held, which is a
// font assembled from bytes no one wrote.
func TestWOFFRefusesATableShorterThanItDeclared(t *testing.T) {
	if _, err := DecodeWOFF(fonttest.WOFFBomb(64, 1<<16)); err == nil {
		t.Fatal("a table that decompressed to 64 bytes while declaring 65536 was accepted")
	}
}

// TestWOFFRefusesADeclaredTotalPastTheCap is the one that guards the
// *allocation*, and it exists because the obvious version of this test did not.
//
// The output is built into a buffer whose capacity is the sum of the tables'
// declared origLengths. Every one of those is a number the file states about
// itself, so the sum is attacker-controlled to the full width of a uint32 times
// the table count — terabytes from a file of a few kilobytes. It has to be
// refused before it is ever handed to make.
//
// The tables here carry almost nothing and each claims four gigabytes. A decoder
// that checks only the bytes it has actually decompressed never gets that far:
// it dies in the allocation, and the running check it was relying on never runs.
func TestWOFFRefusesADeclaredTotalPastTheCap(t *testing.T) {
	var in []fonttest.WOFFTable
	for i := 0; i < 32; i++ {
		in = append(in, fonttest.WOFFTable{
			Tag:  string(rune('A'+i)) + "wxy",
			Data: make([]byte, 4096),
		})
	}
	_, err := DecodeWOFF(fonttest.WOFF(fonttest.WOFFOptions{
		Tables:             in,
		LieAboutOrigLength: 0xFFFFFF00,
	}))
	if err == nil {
		t.Fatal("a font declaring 128 GB of tables was accepted")
	}
}

// TestWOFFRefusesATotalPastTheCap: many tables, each honest, together past what
// this engine will hold. The cap has to be on the total and not per table.
func TestWOFFRefusesATotalPastTheCap(t *testing.T) {
	var in []fonttest.WOFFTable
	for i := 0; i < 64; i++ {
		in = append(in, fonttest.WOFFTable{
			Tag:  string(rune('A'+i/26)) + string(rune('a'+i%26)) + "xy",
			Data: make([]byte, 2<<20),
		})
	}
	if _, err := DecodeWOFF(fonttest.WOFF(fonttest.WOFFOptions{Tables: in})); err == nil {
		t.Fatal("128 MB of tables was accepted")
	}
}

// TestWOFF2IsRefusedByName. WOFF 2 shares the name and is a different format —
// Brotli, and a glyf table that is re-encoded rather than compressed. Saying so
// is the point: "not an sfnt" would send someone looking for a corrupt file.
func TestWOFF2IsRefusedByName(t *testing.T) {
	w2 := fonttest.WOFF(fonttest.WOFFOptions{
		Signature: 0x774F4632, // "wOF2"
		Tables:    []fonttest.WOFFTable{{Tag: "head", Data: []byte("x")}},
	})
	if !IsWOFF2(w2) {
		t.Fatal("IsWOFF2 did not recognise a wOF2 signature")
	}
	if IsWOFF(w2) {
		t.Error("IsWOFF accepted a WOFF 2")
	}
	_, err := DecodeWOFF(w2)
	if err == nil {
		t.Fatal("a WOFF 2 was decoded as a WOFF 1")
	}
	if !strings.Contains(err.Error(), "WOFF 2") {
		t.Errorf("a WOFF 2 was refused with %q, which does not name the format", err)
	}
}

// TestWOFFRefusesAMalformedContainer. Each of these is a number a file states
// about itself, and each would be an allocation, a read past the end or a font
// built from the wrong bytes if it were believed.
func TestWOFFRefusesAMalformedContainer(t *testing.T) {
	good := fonttest.WOFF(fonttest.WOFFOptions{Tables: []fonttest.WOFFTable{
		{Tag: "cmap", Data: []byte("cccc")},
		{Tag: "head", Data: []byte("hhhh")},
	}})
	bad := map[string][]byte{
		"header cut short": good[:40],
		"no tables at all": func() []byte {
			b := append([]byte(nil), good...)
			binary.BigEndian.PutUint16(b[12:], 0)
			return b
		}(),
		"more tables than an sfnt can address": func() []byte {
			b := append([]byte(nil), good...)
			binary.BigEndian.PutUint16(b[12:], 0xFFFF)
			return b
		}(),
		"directory past the end": func() []byte {
			b := append([]byte(nil), good...)
			binary.BigEndian.PutUint16(b[12:], 200)
			return b
		}(),
		"a table outside the file": func() []byte {
			b := append([]byte(nil), good...)
			binary.BigEndian.PutUint32(b[44+4:], uint32(len(b)))
			binary.BigEndian.PutUint32(b[44+8:], 4096)
			return b
		}(),
		"an offset that wraps": func() []byte {
			b := append([]byte(nil), good...)
			binary.BigEndian.PutUint32(b[44+4:], 0xFFFFFFF0)
			binary.BigEndian.PutUint32(b[44+8:], 0x20)
			return b
		}(),
		"compressed to more than the original": func() []byte {
			b := append([]byte(nil), good...)
			binary.BigEndian.PutUint32(b[44+12:], 1) // origLength below compLength
			return b
		}(),
		"the same table twice": func() []byte {
			b := append([]byte(nil), good...)
			binary.BigEndian.PutUint32(b[44+20:], binary.BigEndian.Uint32(b[44:]))
			return b
		}(),
		"not a font at all": []byte("%PDF-2.0\n"),
		"empty":             {},
	}
	for name, b := range bad {
		if _, err := DecodeWOFF(b); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// TestWOFFDecodesAsFontToolsDoes is the independent check: the same bytes read
// by a decoder this repository did not write.
//
// Everything above is this module agreeing with itself — the fixture writes what
// the decoder expects to read, so a shared misunderstanding of the format
// survives all of it. fontTools has none of this code, and testdata/woff-tables
// records what it made of every WOFF in the suite.
//
// Entries are keyed by the file's own hash, so a font the suite replaces
// upstream drops out rather than failing. That could quietly erode to nothing,
// so the floor below is what keeps the check honest.
func TestWOFFDecodesAsFontToolsDoes(t *testing.T) {
	f, err := os.Open("testdata/woff-tables.txt")
	if err != nil {
		t.Fatalf("the oracle is missing: %v", err)
	}
	defer f.Close()

	type table struct {
		length int
		sum    string
	}
	want := map[string]map[string]table{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p := strings.Fields(line)
		if len(p) != 4 {
			t.Fatalf("cannot read %q", line)
		}
		n, err := strconv.Atoi(p[2])
		if err != nil {
			t.Fatalf("cannot read the length in %q", line)
		}
		if want[p[0]] == nil {
			want[p[0]] = map[string]table{}
		}
		want[p[0]][p[1]] = table{n, p[3]}
	}

	dir := os.Getenv("WPT_TESTS")
	if dir == "" {
		t.Skip("set WPT_TESTS (or run `make test-wpt`) to check WOFF against fontTools")
	}
	files, tables, missing := 0, 0, 0
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || filepath.Ext(p) != ".woff" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(data)
		key := hex.EncodeToString(sum[:])[:16]
		w, ok := want[key]
		if !ok {
			missing++ // replaced upstream since the oracle was taken
			return nil
		}
		sfnt, err := DecodeWOFF(data)
		if err != nil {
			t.Errorf("%s: DecodeWOFF: %v", filepath.Base(p), err)
			return nil
		}
		got := SFNTTables(sfnt)
		if got == nil {
			t.Errorf("%s: decoded to something that is not an sfnt", filepath.Base(p))
			return nil
		}
		files++
		if len(got) != len(w) {
			t.Errorf("%s: %d tables, fontTools read %d", filepath.Base(p), len(got), len(w))
		}
		for tag, wt := range w {
			// fontTools reports a tag as it is spelled rather than as it is
			// stored, so "cvt " and "CFF " come back without their padding.
			for len(tag) < 4 {
				tag += " "
			}
			g, ok := got[tag]
			if !ok {
				t.Errorf("%s: table %q missing", filepath.Base(p), tag)
				continue
			}
			tables++
			if len(g) != wt.length {
				t.Errorf("%s %s: %d bytes, fontTools %d", filepath.Base(p), tag, len(g), wt.length)
				continue
			}
			s := sha256.Sum256(g)
			if hex.EncodeToString(s[:])[:16] != wt.sum {
				t.Errorf("%s %s: content differs from what fontTools read", filepath.Base(p), tag)
			}
		}
		return nil
	})

	// The floor. Without it this passes just as well when the corpus has moved
	// out from under every entry and nothing was compared at all.
	const leastFiles, leastTables = 60, 700
	if files < leastFiles || tables < leastTables {
		t.Errorf("compared %d files and %d tables, want at least %d and %d; %d files "+
			"were not in the oracle, so it needs regenerating against the current suite",
			files, tables, leastFiles, leastTables, missing)
	}
	t.Logf("%d WOFF files, %d tables, all identical to fontTools (%d not in the oracle)",
		files, tables, missing)
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
