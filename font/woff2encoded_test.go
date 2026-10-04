package font

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// WOFF 2 collections google/woff2's encoder made (testdata/woff2-collections,
// see make.py there): each beside the collection it was made from, and what
// google/woff2's decoder makes of it. Those are the only WOFF 2 collections
// here not built by this module's own fixture builder, so they are what says
// the decoder reads the format as the format's own tools write it.

// encodedCollections is each NAME of testdata/woff2-collections: NAME.woff2
// and the NAME.ttc it was made from.
func encodedCollections(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", "woff2-collections", "*.woff2"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 3 {
		t.Fatalf("%d encoded collections in testdata/woff2-collections; make.py makes three", len(files))
	}
	var names []string
	for _, f := range files {
		names = append(names, strings.TrimSuffix(filepath.Base(f), ".woff2"))
	}
	sort.Strings(names)
	return names
}

func readEncoded(t *testing.T, name, ext string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "woff2-collections", name+ext))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestAnEncodedWOFF2CollectionRebuildsToWhatTheReferenceSays holds each
// collection DecodeWOFF2 rebuilds to the one google/woff2's decoder rebuilds,
// byte for byte: the same tables at the same offsets, the shared ones once,
// and the same checksums.
func TestAnEncodedWOFF2CollectionRebuildsToWhatTheReferenceSays(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "woff2-collections", "expected.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	expected := map[string][2]string{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			t.Fatalf("expected.txt: %q is not <file> <length> <digest>", line)
		}
		expected[fields[0]] = [2]string{fields[1], fields[2]}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	for _, name := range encodedCollections(t) {
		want, ok := expected[name+".woff2"]
		if !ok {
			t.Errorf("%s.woff2 is not in expected.txt", name)
			continue
		}
		got, err := DecodeWOFF2(readEncoded(t, name, ".woff2"))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		sum := sha256.Sum256(got)
		if strconv.Itoa(len(got)) != want[0] || hex.EncodeToString(sum[:])[:16] != want[1] {
			t.Errorf("%s: rebuilt to %d bytes hashing to %s, and the reference decoder's is %s bytes hashing to %s",
				name, len(got), hex.EncodeToString(sum[:])[:16], want[0], want[1])
		}
	}
}

// TestAnEncodedWOFF2CollectionIsTheCollectionItWasMadeFrom holds each face
// of each collection, decoded, to the face the encoder was given: the same
// tables, each the same bytes but for three a WOFF 2 round trip may change —
// head's checkSumAdjustment, which is of the file as written; the flag bit 11
// the format has a decoder set where it undid a transform; and glyf and loca,
// which the decoder writes out again from the transformed glyf, and which are
// held to the same glyphs rather than the same bytes.
func TestAnEncodedWOFF2CollectionIsTheCollectionItWasMadeFrom(t *testing.T) {
	transformed := 0
	for _, name := range encodedCollections(t) {
		src := readEncoded(t, name, ".ttc")
		got, err := DecodeWOFF2(readEncoded(t, name, ".woff2"))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(CollectionOffsets(got)) != len(CollectionOffsets(src)) {
			t.Fatalf("%s: %d fonts, made from %d", name, len(CollectionOffsets(got)), len(CollectionOffsets(src)))
		}
		for i := range CollectionOffsets(src) {
			want, have := CollectionTables(src, i), CollectionTables(got, i)
			label := name + " face " + strconv.Itoa(i)
			if len(want) != len(have) {
				t.Errorf("%s: %d tables, made from %d", label, len(have), len(want))
			}
			for tag, w := range want {
				g, ok := have[tag]
				switch {
				case !ok:
					t.Errorf("%s: no %s", label, tag)
				case tag == "head":
					if !sameHead(g, w) {
						t.Errorf("%s: head differs beyond its adjustment and flag 11", label)
					}
				case tag == "glyf" || tag == "loca":
				case !bytes.Equal(g, w):
					t.Errorf("%s: %s differs", label, tag)
				}
			}
			if want["glyf"] == nil {
				continue
			}
			transformed++
			wg, hg := glyphs(t, want), glyphs(t, have)
			if len(wg) != len(hg) {
				t.Errorf("%s: %d glyphs, made from %d", label, len(hg), len(wg))
				continue
			}
			for gid := range wg {
				if !sameGlyph(wg[gid], hg[gid]) {
					t.Errorf("%s: glyph %d is not the glyph it was made from", label, gid)
				}
			}
		}
	}
	if transformed == 0 {
		t.Fatal("no face had a glyf, so this test does not reach the transform")
	}
}

// sameHead compares two heads but for checkSumAdjustment, at 8, and bit 11 of
// flags, at 16.
func sameHead(a, b []byte) bool {
	if len(a) != len(b) || len(a) < 18 {
		return false
	}
	a, b = append([]byte(nil), a...), append([]byte(nil), b...)
	for _, h := range [][]byte{a, b} {
		copy(h[8:12], []byte{0, 0, 0, 0})
		h[16] &^= 0x08
	}
	return bytes.Equal(a, b)
}

// glyphs is a face's glyf cut into its glyphs by its loca.
func glyphs(t *testing.T, tabs map[string][]byte) [][]byte {
	t.Helper()
	head, loca, glyf, maxp := tabs["head"], tabs["loca"], tabs["glyf"], tabs["maxp"]
	if len(head) < 52 || len(maxp) < 6 {
		t.Fatal("no head or maxp to read glyf by")
	}
	long := binary.BigEndian.Uint16(head[50:]) == 1
	n := int(binary.BigEndian.Uint16(maxp[4:]))
	at := func(i int) int {
		if long {
			return int(binary.BigEndian.Uint32(loca[4*i:]))
		}
		return 2 * int(binary.BigEndian.Uint16(loca[2*i:]))
	}
	if size := map[bool]int{true: 4, false: 2}[long]; len(loca) < size*(n+1) {
		t.Fatalf("loca holds %d bytes for %d glyphs", len(loca), n)
	}
	out := make([][]byte, n)
	for i := range out {
		lo, hi := at(i), at(i+1)
		if lo > hi || hi > len(glyf) {
			t.Fatalf("glyph %d runs from %d to %d of a glyf of %d", i, lo, hi, len(glyf))
		}
		out[i] = glyf[lo:hi]
	}
	return out
}

// sameGlyph says two glyphs are the same glyph: a composite the same
// components and instructions, as the transform keeps a composite whole; a
// simple glyph the same box, contours, instructions and points, each point
// where it is and on or off the curve, whichever way the two spell their flags
// and coordinates. Each is read to its own end, so that what pads it out to a
// loca boundary is not read as the glyph — and nothing of the glyph is taken
// for padding, as trimming its trailing zeros took the last coordinate where
// it was zero.
func sameGlyph(a, b []byte) bool {
	if len(a) < 10 || len(b) < 10 {
		return len(bytes.Trim(a, "\x00")) == 0 && len(bytes.Trim(b, "\x00")) == 0
	}
	if int16(binary.BigEndian.Uint16(a)) < 0 {
		ea, oka := compositeEnd(a)
		eb, okb := compositeEnd(b)
		return oka && okb && bytes.Equal(a[:ea], b[:eb])
	}
	pa, oka := simpleGlyph(a)
	pb, okb := simpleGlyph(b)
	return oka && okb && bytes.Equal(a[:10], b[:10]) && pa == pb
}

// compositeEnd is where a composite glyph ends: after its last component, and
// its instructions where it has them.
func compositeEnd(g []byte) (int, bool) {
	at := 10
	for {
		if len(g) < at+4 {
			return 0, false
		}
		flags := binary.BigEndian.Uint16(g[at:])
		at += 4
		if flags&0x0001 != 0 {
			at += 4
		} else {
			at += 2
		}
		switch {
		case flags&0x0008 != 0:
			at += 2
		case flags&0x0040 != 0:
			at += 4
		case flags&0x0080 != 0:
			at += 8
		}
		if flags&0x0020 != 0 {
			continue
		}
		if flags&0x0100 != 0 {
			if len(g) < at+2 {
				return 0, false
			}
			at += 2 + int(binary.BigEndian.Uint16(g[at:]))
		}
		if at > len(g) {
			return 0, false
		}
		return at, true
	}
}

// simpleGlyph reads a simple glyph into a comparable string: its end points,
// its instructions, and each point's coordinates and curve flag.
func simpleGlyph(g []byte) (string, bool) {
	var out strings.Builder
	n := int(binary.BigEndian.Uint16(g))
	at := 10
	if len(g) < at+2*n+2 {
		return "", false
	}
	points := 0
	for i := range n {
		end := int(binary.BigEndian.Uint16(g[at+2*i:]))
		out.WriteString("e" + strconv.Itoa(end))
		points = end + 1
	}
	at += 2 * n
	ins := int(binary.BigEndian.Uint16(g[at:]))
	at += 2
	if len(g) < at+ins {
		return "", false
	}
	out.WriteString("i" + hex.EncodeToString(g[at:at+ins]))
	at += ins
	flags := make([]byte, 0, points)
	for len(flags) < points {
		if at >= len(g) {
			return "", false
		}
		f := g[at]
		at++
		flags = append(flags, f)
		if f&0x08 != 0 {
			if at >= len(g) {
				return "", false
			}
			for range int(g[at]) {
				flags = append(flags, f)
			}
			at++
		}
	}
	flags = flags[:points]
	coord := func(short, same byte) ([]int, bool) {
		v, out := 0, make([]int, points)
		for i, f := range flags {
			switch {
			case f&short != 0:
				if at >= len(g) {
					return nil, false
				}
				d := int(g[at])
				at++
				if f&same == 0 {
					d = -d
				}
				v += d
			case f&same == 0:
				if at+2 > len(g) {
					return nil, false
				}
				v += int(int16(binary.BigEndian.Uint16(g[at:])))
				at += 2
			}
			out[i] = v
		}
		return out, true
	}
	xs, okx := coord(0x02, 0x10)
	ys, oky := coord(0x04, 0x20)
	if !okx || !oky {
		return "", false
	}
	for i, f := range flags {
		out.WriteString(" " + strconv.Itoa(xs[i]) + "," + strconv.Itoa(ys[i]) + "," + strconv.Itoa(int(f&0x41)))
	}
	return out.String(), true
}
