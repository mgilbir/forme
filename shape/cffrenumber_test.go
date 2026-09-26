package shape

import (
	"encoding/binary"
	"fmt"
	"reflect"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// A CID-keyed CFF is renumbered when it is subsetted: only the kept glyphs, a
// charset giving each its CID, an FDSelect giving each its Font DICT, and
// subroutine INDEXes holding only what the kept glyphs call. See
// cffrenumber.go.

// cidFixture describes a CID-keyed CFF with a Font DICT per entry of locals,
// each with its own local subroutines. It is written here rather than in
// fonttest because the structures under test — FDArray, FDSelect, a charset of
// CIDs, subroutines per Font DICT — are exactly what the fixtures there do not
// build.
type cidFixture struct {
	glyphs [][]byte   // charstrings, .notdef first
	cids   []int      // the CID of each glyph; cids[0] is .notdef's, 0
	fds    []int      // the Font DICT of each glyph
	locals [][][]byte // each Font DICT's local subroutines
	global [][]byte
}

// cff writes the program. Every offset is in the five-byte form, so the Top
// DICT and the Font DICTs are the same size whatever they point at.
func (c cidFixture) cff() []byte {
	private := func(fd int) []byte {
		d := append(cffInt(500), 20)            // defaultWidthX
		d = append(append(d, cffInt(0)...), 21) // nominalWidthX
		if len(c.locals[fd]) > 0 {
			d = append(append(d, cffInt(len(d)+6)...), 19) // Subrs, right after the DICT
		}
		return d
	}
	top := func(charset, charStrings, fdArray, fdSelect int) []byte {
		d := append(cffInt(391), cffInt(392)...)
		d = append(append(d, cffInt(0)...), 12, 30) // ROS Adobe-Identity-0
		d = append(append(d, cffInt(charset)...), 15)
		d = append(append(d, cffInt(charStrings)...), 17)
		d = append(append(d, cffInt(fdArray)...), 12, 36)
		d = append(append(d, cffInt(fdSelect)...), 12, 37)
		return append(append(d, cffInt(65535)...), 12, 34) // CIDCount
	}
	layout := func(charset, charStrings, fdArray, fdSelect int, privAt []int) []byte {
		out := []byte{1, 0, 4, 4}
		out = append(out, writeCFFIndex([][]byte{[]byte("CIDFixture")})...)
		out = append(out, writeCFFIndex([][]byte{top(charset, charStrings, fdArray, fdSelect)})...)
		out = append(out, writeCFFIndex([][]byte{[]byte("Adobe"), []byte("Identity")})...)
		out = append(out, writeCFFIndex(c.global)...)
		return out
	}
	charset := []byte{0}
	for _, cid := range c.cids[1:] {
		charset = binary.BigEndian.AppendUint16(charset, uint16(cid))
	}
	fdSelect := []byte{0}
	for _, fd := range c.fds {
		fdSelect = append(fdSelect, byte(fd))
	}
	fontDicts := func(privAt []int) []byte {
		var dicts [][]byte
		for fd := range c.locals {
			d := append(cffInt(len(private(fd))), cffInt(privAt[fd])...)
			dicts = append(dicts, append(d, 18))
		}
		return writeCFFIndex(dicts)
	}
	charStrings := writeCFFIndex(c.glyphs)

	head := layout(0, 0, 0, 0, nil)
	charsetAt := len(head)
	fdSelectAt := charsetAt + len(charset)
	charStringsAt := fdSelectAt + len(fdSelect)
	fdArrayAt := charStringsAt + len(charStrings)
	privAt := make([]int, len(c.locals))
	at := fdArrayAt + len(fontDicts(privAt))
	for fd := range c.locals {
		privAt[fd] = at
		at += len(private(fd))
		if len(c.locals[fd]) > 0 {
			at += len(writeCFFIndex(c.locals[fd]))
		}
	}
	out := layout(charsetAt, charStringsAt, fdArrayAt, fdSelectAt, privAt)
	out = append(out, charset...)
	out = append(out, fdSelect...)
	out = append(out, charStrings...)
	out = append(out, fontDicts(privAt)...)
	for fd := range c.locals {
		out = append(out, private(fd)...)
		if len(c.locals[fd]) > 0 {
			out = append(out, writeCFFIndex(c.locals[fd])...)
		}
	}
	if len(out) != at {
		panic("cidFixture: the layout did not come out where it was measured")
	}
	return out
}

// face wraps the program in an OpenType font whose glyph g (from 1) is
// runes[g-1], each advancing 100·g units so that a glyph's metrics say which
// glyph it is.
func (c cidFixture) face(t *testing.T, runes []rune) *Face {
	t.Helper()
	var glyphs []fonttest.Glyph
	for i, r := range runes {
		glyphs = append(glyphs, fonttest.Glyph{Rune: r, Advance: 100 * (i + 1), HasShape: true})
	}
	f, err := Load(fonttest.OTTO(c.cff(), fonttest.SFNTOptions{Glyphs: glyphs}))
	if err != nil {
		t.Fatalf("the fixture does not load: %v", err)
	}
	if !f.IsCIDKeyed() {
		t.Fatal("the fixture is not CID-keyed")
	}
	return f
}

// Charstring pieces. A subroutine number is written as the index less the
// bias, and the bias is 107 for fewer than 1240 subroutines.
func t2(v int) []byte { return type2Int(v) }

func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

var (
	opRlineto   = []byte{5}
	opRmoveto   = []byte{21}
	opCallsubr  = []byte{10}
	opCallgsubr = []byte{29}
	opReturn    = []byte{11}
	opEndchar   = []byte{14}
)

// line is a subroutine that draws one distinctive segment and returns.
func line(dx, dy int) []byte { return cat(t2(dx), t2(dy), opRlineto, opReturn) }

// flatten runs a charstring as a reader does, inlining every call, and returns
// the numbers and operators it draws with — the oracle for "draws the same".
// It is written from the specification here rather than taken from the code
// under test: the bias is 107, 1131 or 32768 by the INDEX's size, and the
// number naming a subroutine is whatever is on top of the stack.
func flatten(t *testing.T, code []byte, local, global [][]byte) []string {
	t.Helper()
	bias := func(n int) int {
		switch {
		case n < 1240:
			return 107
		case n < 33900:
			return 1131
		}
		return 32768
	}
	var out []string
	var nums []int
	var run func(code []byte, depth int) bool
	run = func(code []byte, depth int) bool {
		if depth > 10 {
			t.Fatal("flatten: nested too deeply")
		}
		for at := 0; at < len(code); {
			v := int(code[at])
			switch {
			case v == 28:
				nums = append(nums, int(int16(binary.BigEndian.Uint16(code[at+1:]))))
				at += 3
				continue
			case v == 255: // 16.16 fixed; a whole number here
				nums = append(nums, int(int32(binary.BigEndian.Uint32(code[at+1:])))>>16)
				at += 5
				continue
			case v >= 32 && v <= 246:
				nums = append(nums, v-139)
				at++
				continue
			case v >= 247 && v <= 250:
				nums = append(nums, (v-247)*256+int(code[at+1])+108)
				at += 2
				continue
			case v >= 251 && v <= 254:
				nums = append(nums, -(v-251)*256-int(code[at+1])-108)
				at += 2
				continue
			}
			at++
			switch v {
			case 10, 29:
				idx := local
				if v == 29 {
					idx = global
				}
				k := nums[len(nums)-1] + bias(len(idx))
				nums = nums[:len(nums)-1]
				if k < 0 || k >= len(idx) {
					t.Fatalf("flatten: a call names subroutine %d of %d", k, len(idx))
				}
				if run(idx[k], depth+1) {
					return true
				}
			case 11:
				return false
			case 14:
				out = append(out, fmt.Sprint(nums), "endchar")
				return true
			default:
				out = append(out, fmt.Sprint(nums), fmt.Sprintf("op%d", v))
				nums = nil
			}
		}
		return false
	}
	run(code, 0)
	return out
}

// cffParts reads what the subset's glyph g is drawn from: its charstring, its
// Font DICT's local subroutines, and the global ones.
type cffParts struct {
	glyphs [][]byte
	global [][]byte
	locals [][][]byte // per Font DICT
	fds    []int      // per glyph
}

func readCFFParts(t *testing.T, cff []byte) cffParts {
	t.Helper()
	var p cffParts
	var err error
	if p.glyphs, err = CharStringsForTest(cff); err != nil {
		t.Fatal(err)
	}
	if p.global, err = cffGlobalSubrs(cff); err != nil {
		t.Fatal(err)
	}
	top, err := topDictOf(cff)
	if err != nil {
		t.Fatal(err)
	}
	fdArrayAt := -1
	for _, e := range top {
		if e.op == opFDArray {
			fdArrayAt = e.operands[0]
		}
	}
	fdIndex, err := readCFFIndex(cff, fdArrayAt)
	if err != nil {
		t.Fatal(err)
	}
	for _, fd := range fdIndex.items {
		ops, _, err := parseCFFDict(fd)
		if err != nil {
			t.Fatal(err)
		}
		var subrs [][]byte
		for _, e := range ops {
			if e.op == opPrivate {
				size, off := e.operands[0], e.operands[1]
				_, _, subrs, err = privateParts(cff[off:], size)
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		p.locals = append(p.locals, subrs)
	}
	prog := font.ParseCFF(cff)
	if prog == nil {
		t.Fatal("the font package cannot read the CFF")
	}
	p.fds = prog.GIDToFD
	return p
}

// checkRenumbered holds a subset to the original it came from: glyph i of the
// subset is kept[i] of the original, carries its CID, sits in its Font DICT,
// and draws exactly what it drew.
func checkRenumbered(t *testing.T, orig, sub []byte, kept []int) (o, s cffParts) {
	t.Helper()
	origCFF, subCFF := font.SFNTTables(orig)["CFF "], font.SFNTTables(sub)["CFF "]
	o, s = readCFFParts(t, origCFF), readCFFParts(t, subCFF)
	if len(s.glyphs) != len(kept) {
		t.Fatalf("the subset holds %d glyphs and kept %d", len(s.glyphs), len(kept))
	}
	before, after := font.ParseCFF(origCFF), font.ParseCFF(subCFF)
	for i, g := range kept {
		if after.GIDToCID[i] != before.GIDToCID[g] {
			t.Errorf("glyph %d (was %d) has CID %d, and had %d", i, g, after.GIDToCID[i], before.GIDToCID[g])
		}
		if s.fds[i] != o.fds[g] {
			t.Errorf("glyph %d (was %d) is in Font DICT %d, and was in %d", i, g, s.fds[i], o.fds[g])
		}
		want := flatten(t, o.glyphs[g], o.locals[o.fds[g]], o.global)
		got := flatten(t, s.glyphs[i], s.locals[s.fds[i]], s.global)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("glyph %d (was %d) draws %v, and drew %v", i, g, got, want)
		}
	}
	return o, s
}

// twoFontDicts is a font whose glyphs call local subroutines of two Font
// DICTs and global ones, so that each INDEX has some subroutines only a
// dropped glyph uses.
func twoFontDicts() cidFixture {
	move := cat(t2(10), t2(20), opRmoveto)
	return cidFixture{
		glyphs: [][]byte{
			opEndchar,
			cat(move, t2(2-107), opCallsubr, t2(1-107), opCallgsubr, opEndchar), // FD 0
			cat(move, t2(0-107), opCallsubr, opEndchar),                         // FD 1
			cat(move, t2(0-107), opCallsubr, t2(0-107), opCallgsubr,
				t2(2-107), opCallgsubr, opEndchar), // FD 0, dropped
			cat(move, t2(1-107), opCallsubr, t2(2-107), opCallsubr, opEndchar), // FD 1
		},
		cids: []int{0, 700, 20220, 31, 900},
		fds:  []int{0, 0, 1, 0, 1},
		locals: [][][]byte{
			{line(1, 0), line(2, 0), line(3, 0)},
			{line(0, 1), line(0, 2), line(0, 3)},
		},
		global: [][]byte{line(4, 4), line(5, 5), line(6, 6)},
	}
}

func TestACIDKeyedSubsetKeepsOnlyItsGlyphsAndSubroutines(t *testing.T) {
	fix := twoFontDicts()
	// The last glyph's character is past the BMP, so the character map written
	// for the subset needs its format 12 subtable.
	runes := []rune{'a', 'b', 'c', 0x20000}
	f := fix.face(t, runes)
	orig := f.Program()
	codes, _ := f.Encode("ab\U00020000")
	prog, kept, err := f.SubsetGlyphs()
	if err != nil {
		t.Fatalf("SubsetGlyphs: %v", err)
	}
	if want := []int{0, 1, 2, 4}; !reflect.DeepEqual(kept, want) {
		t.Fatalf("kept %v, want %v", kept, want)
	}
	_, s := checkRenumbered(t, orig, prog, kept)

	// Only what the kept glyphs call: global 1 (glyph 1), local 2 of Font
	// DICT 0 (glyph 1), locals 0, 1 and 2 of Font DICT 1 (glyphs 2 and 4).
	if len(s.global) != 1 || len(s.locals[0]) != 1 || len(s.locals[1]) != 3 {
		t.Errorf("the subset carries %d global and %d, %d local subroutines; want 1, and 1 and 3",
			len(s.global), len(s.locals[0]), len(s.locals[1]))
	}

	// The sfnt around it is numbered the same way, and the codes a document
	// holds still name the same glyphs.
	g, err := Load(prog)
	if err != nil {
		t.Fatalf("the subset does not load: %v", err)
	}
	if g.NumGlyphs() != len(kept) {
		t.Errorf("the subset's maxp says %d glyphs and it holds %d", g.NumGlyphs(), len(kept))
	}
	for i, old := range kept {
		if a, b := g.GlyphAdvance(i), f.GlyphAdvance(old); a != b {
			t.Errorf("glyph %d advances %v and glyph %d of the face %v", i, a, old, b)
		}
		if g.GlyphCode(i) != f.GlyphCode(old) {
			t.Errorf("glyph %d has code %d and glyph %d of the face %d", i, g.GlyphCode(i), old, f.GlyphCode(old))
		}
	}
	for r, old := range map[rune]int{'a': 1, 'b': 2, 0x20000: 4} {
		gid, ok := g.GlyphID(r)
		if !ok || kept[gid] != old {
			t.Errorf("U+%04X maps to glyph %d (%v) of the subset, which is not the face's glyph %d", r, gid, ok, old)
		}
	}
	if _, ok := g.GlyphID('c'); ok {
		t.Error("the subset maps a character whose glyph it dropped")
	}
	if got, _ := g.Encode("ab\U00020000"); string(got) != string(codes) {
		t.Errorf("the subset encodes % x where the face encoded % x", got, codes)
	}
}

// TestRenumberingSubroutinesRecomputesTheBias: an INDEX of 1,240 subroutines or
// more is biased by 1131, and one of fewer by 107, so keeping two of 1,300
// changes the number every surviving call is written with.
func TestRenumberingSubroutinesRecomputesTheBias(t *testing.T) {
	global := make([][]byte, 1300)
	for i := range global {
		global[i] = line(i%100+1, i/100+1)
	}
	fix := cidFixture{
		glyphs: [][]byte{
			opEndchar,
			cat(t2(1299-1131), opCallgsubr, t2(5-1131), opCallgsubr, opEndchar),
		},
		cids:   []int{0, 1},
		fds:    []int{0, 0},
		locals: [][][]byte{nil},
		global: global,
	}
	f := fix.face(t, []rune{'a'})
	f.Encode("a")
	prog, kept, err := f.SubsetGlyphs()
	if err != nil {
		t.Fatalf("SubsetGlyphs: %v", err)
	}
	_, s := checkRenumbered(t, f.Program(), prog, kept)
	if len(s.global) != 2 {
		t.Errorf("the subset carries %d global subroutines, want 2", len(s.global))
	}
}

// The subroutines are kept whole, and the glyphs still renumbered, where a
// call cannot be renumbered exactly.
func TestSubroutinesAreKeptWholeWhereACallCannotBeRenumbered(t *testing.T) {
	move := cat(t2(10), t2(20), opRmoveto)
	for _, c := range []struct {
		name string
		fix  cidFixture
	}{
		{
			// The number naming the global subroutine is pushed by a local
			// one, so its bytes are not the glyph's to rewrite.
			"a call named by a number a subroutine pushed",
			cidFixture{
				glyphs: [][]byte{
					opEndchar,
					cat(move, t2(0-107), opCallsubr, opCallgsubr, opEndchar),
					cat(move, t2(1-107), opCallgsubr, opEndchar),
				},
				cids:   []int{0, 5, 6},
				fds:    []int{0, 0, 0},
				locals: [][][]byte{{cat(t2(2-107), opReturn)}},
				global: [][]byte{line(1, 1), line(2, 2), line(3, 3)},
			},
		},
		{
			// The number is written as 16.16 fixed rather than as an integer.
			// A reader takes it as 1, and it is not an integer operand to be
			// rewritten; were it read as one, this INDEX is large enough that
			// a wrong reading names a subroutine that exists.
			"a call named by a fixed-point number",
			cidFixture{
				glyphs: [][]byte{
					opEndchar,
					cat(move, []byte{255}, t2Fixed(1-107),
						opCallgsubr, opEndchar),
					cat(move, t2(5-107), opCallgsubr, opEndchar),
				},
				cids:   []int{0, 5, 6},
				fds:    []int{0, 0, 0},
				locals: [][][]byte{nil},
				global: func() [][]byte {
					g := make([][]byte, 200)
					for i := range g {
						g[i] = line(i+1, 1)
					}
					return g
				}(),
			},
		},
		{
			// A global subroutine calls local subroutine 1, which is local 1
			// of whichever Font DICT the glyph drawing it is in. Font DICT 0
			// keeps its locals 0 and 1 and Font DICT 1 only its local 1, so
			// the one call site would need two numbers.
			"one call site that would need two numbers",
			cidFixture{
				glyphs: [][]byte{
					opEndchar,
					cat(move, t2(0-107), opCallsubr, t2(0-107), opCallgsubr, opEndchar), // FD 0
					cat(move, t2(0-107), opCallgsubr, opEndchar),                        // FD 1
				},
				cids: []int{0, 5, 6},
				fds:  []int{0, 0, 1},
				locals: [][][]byte{
					{line(1, 0), line(2, 0)},
					{line(0, 1), line(0, 2)},
				},
				global: [][]byte{cat(t2(1-107), opCallsubr, opReturn), line(9, 9)},
			},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := c.fix.face(t, []rune{'a', 'b'})
			f.Encode("ab")
			prog, kept, err := f.SubsetGlyphs()
			if err != nil {
				t.Fatalf("SubsetGlyphs: %v", err)
			}
			o, s := checkRenumbered(t, f.Program(), prog, kept)
			if len(s.global) != len(o.global) {
				t.Errorf("the subset carries %d global subroutines of %d; they had to be kept whole",
					len(s.global), len(o.global))
			}
			for fd := range o.locals {
				if len(s.locals[fd]) != len(o.locals[fd]) {
					t.Errorf("Font DICT %d carries %d local subroutines of %d; they had to be kept whole",
						fd, len(s.locals[fd]), len(o.locals[fd]))
				}
			}
		})
	}
}

// TestASubroutineIsRunOncePerStateItIsEnteredIn is the cost shape of running
// the kept glyphs: a chain of subroutines, each calling the next fanOut times,
// is fanOut^depth calls when every call is run and fanOut·depth operators when
// a subroutine entered as before is not run again. So doubling fanOut about
// doubles the work, where running every call multiplies it by 2^depth — and
// already at fanOut 4 that is past the bound on one glyph's operators, which
// the subset reports as an error.
func TestASubroutineIsRunOncePerStateItIsEnteredIn(t *testing.T) {
	const depth = 8
	spent := func(fanOut int) int {
		global := make([][]byte, depth+1)
		for i := 0; i < depth; i++ {
			for k := 0; k < fanOut; k++ {
				global[i] = cat(global[i], t2(i+1-107), opCallgsubr)
			}
			global[i] = cat(global[i], opReturn)
		}
		global[depth] = line(1, 1)
		fix := cidFixture{
			glyphs: [][]byte{opEndchar, cat(t2(10), t2(10), opRmoveto, t2(0-107), opCallgsubr, opEndchar)},
			cids:   []int{0, 1},
			fds:    []int{0, 0},
			locals: [][][]byte{nil},
			global: global,
		}
		keep := []bool{true, true}
		b := font.NewBudget(maxFontWork)
		if _, _, err := subsetCFF(fix.cff(), keep, b); err != nil {
			t.Fatalf("fanOut %d: %v", fanOut, err)
		}
		return b.Spent()
	}
	small, large := spent(4), spent(8)
	if ratio := float64(large) / float64(small); ratio > 3 {
		t.Errorf("doubling the fan-out took the work from %d to %d units, %.1fx; running each "+
			"subroutine once per entry state is about 2x", small, large, ratio)
	}
}

// TestACIDKeyedFaceSubsetIsTheSizeOfItsGlyphs is the issue's own case: three
// characters of Noto Sans JP. The subset was 1,680,632 bytes — every glyph
// slot, every charset entry, every subroutine, and the whole cmap and hmtx —
// and is now 4,548.
func TestACIDKeyedFaceSubsetIsTheSizeOfItsGlyphs(t *testing.T) {
	data := cidKeyedFace(t)
	f, err := Load(data)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	f.Encode("ｱ日本")
	prog, kept, err := f.SubsetGlyphs()
	if err != nil {
		t.Fatalf("SubsetGlyphs: %v", err)
	}
	if len(prog) > 8<<10 {
		t.Errorf("three characters subset to %d bytes of a %d-byte face", len(prog), len(data))
	}
	checkRenumbered(t, data, prog, kept)
}

// t2Fixed is v as the four bytes of a Type 2 16.16 fixed-point operand.
func t2Fixed(v int) []byte {
	return binary.BigEndian.AppendUint32(nil, uint32(int32(v)<<16))
}

// TestAnINDEXHoldsItsLastOffset: an INDEX's offsets run from 1 to one past
// its data, so data of 255 bytes ends at offset 256, which one byte cannot
// hold. The offset size was chosen as though the largest offset were the
// data's length, and an INDEX whose data came to 255, 65,535 or 16,777,215
// bytes wrote its last offset in too few bytes. Copying INDEXes whole never
// wrote one; rebuilding thousands of subroutine INDEXes wrote one in Noto Sans
// HK, which fontTools could not read back.
func TestAnINDEXHoldsItsLastOffset(t *testing.T) {
	for _, size := range []int{254, 255, 256, 65534, 65535, 65536} {
		items := [][]byte{make([]byte, size/2), make([]byte, size-size/2)}
		b := writeCFFIndex(items)
		idx, err := readCFFIndex(b, 0)
		if err != nil {
			t.Errorf("an INDEX of %d bytes of data does not read back: %v", size, err)
			continue
		}
		if idx.end != len(b) || len(idx.items) != 2 ||
			len(idx.items[0]) != size/2 || len(idx.items[1]) != size-size/2 {
			t.Errorf("an INDEX of %d bytes of data reads back as %d items ending at %d of %d",
				size, len(idx.items), idx.end, len(b))
		}
	}
}
