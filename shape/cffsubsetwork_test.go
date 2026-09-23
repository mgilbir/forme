package shape

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// What the CFF subsetter reads again, and what it copies.
//
// Load reads a font under one font.Budget, and what it can be made to repeat is
// charged there. Subset read the font again — every kept glyph's charstring for
// a seac, every Font DICT's Private DICT and subroutines — under none, and
// copied a Private DICT out once for every Font DICT that named it.

// fullBudget is the allowance Subset gives itself.
func fullBudget() *font.Budget { return font.NewBudget(maxFontWork) }

// hintedSeac is a charstring that declares stems, masks them, and ends in a
// seac of 65 and 66: before, then 0 0 65 66 endchar.
func hintedSeac(before ...[]byte) []byte {
	var c []byte
	for _, b := range before {
		c = append(c, b...)
	}
	c = append(c, cffNumber(0)...)
	c = append(c, cffNumber(0)...)
	c = append(c, cffNumber(65)...)
	c = append(c, cffNumber(66)...)
	return append(c, 14) // endchar
}

// stems is n pairs of operands for a stem operator.
func stems(n int) []byte {
	var c []byte
	for i := 0; i < 2*n; i++ {
		c = append(c, cffNumber(10)...)
	}
	return c
}

// TestASeacAfterHintsIsFound is audit C78. A hintmask is followed by one bit
// for every stem declared so far — by hstem, vstem, hstemhm, vstemhm and the
// hintmask's own implicit vstem — rounded up to bytes. The walk counted only the
// operands in front of the hintmask, so after any stem operator it skipped too
// few bytes, read the mask as a number, and found no seac: the subset then
// dropped both glyphs the accented letter is drawn from.
//
// Each mask here is 0x1C bytes, which read as an operand are the three-byte
// shortint prefix and swallow what follows.
func TestASeacAfterHintsIsFound(t *testing.T) {
	const (
		hstem, vstem, hstemhm, vstemhm = 1, 3, 18, 23
		hintmask, cntrmask             = 19, 20
	)
	op := func(b ...byte) []byte { return b }
	for _, tc := range []struct {
		what string
		code []byte
	}{
		{"the audit's charstring: three hstemhm stems and a one-byte mask",
			hintedSeac(stems(3), op(hstemhm, hintmask, 0x1C))},
		{"a width and two hstems, seven vstems: nine stems and a two-byte mask",
			hintedSeac(cffNumber(500), stems(2), op(hstem), stems(7), op(vstem, hintmask, 0x1C, 0x1C))},
		{"eight hstemhm stems and an implicit vstem at the hintmask: nine, two bytes",
			hintedSeac(stems(8), op(hstemhm), stems(1), op(hintmask, 0x1C, 0x1C))},
		{"a second mask counts every stem before it, not the operands in front of it",
			hintedSeac(stems(4), op(hstemhm), stems(5), op(vstemhm, hintmask, 0xFF, 0x80),
				op(cntrmask, 0x1C, 0x1C))},
	} {
		b, a, ok := cffSeac(tc.code, nil, nil, fullBudget())
		if !ok || b != 65 || a != 66 {
			t.Errorf("%s: seac=%v (%d, %d), want true (65, 66)", tc.what, ok, b, a)
		}
	}
}

// TestASubsetKeepsWhatAHintedSeacIsDrawnFrom is the same through Subset: an
// accented letter whose charstring declares stems before its seac.
func TestASubsetKeepsWhatAHintedSeacIsDrawnFrom(t *testing.T) {
	hints := append(stems(3), 18, 19, 0x1C) // hstemhm, hintmask, one mask byte
	f, gidA, gidAcute, gidAacute := seacFaceAfter(t, hints)
	if _, missing := f.Encode("Á"); missing != 0 {
		t.Fatalf("%d runes of the fixture are missing", missing)
	}
	data, _, err := f.subset()
	if err != nil {
		t.Fatalf("subsetting: %v", err)
	}
	cs, err := CharStringsForTest(font.SFNTTables(data)["CFF "])
	if err != nil {
		t.Fatalf("reading the subset's charstrings: %v", err)
	}
	for _, gid := range []int{gidAacute, gidA, gidAcute} {
		if len(cs[gid]) <= 1 {
			t.Errorf("glyph %d came out of the subsetter as a bare endchar; the hinted "+
				"accented letter is drawn from glyphs the subset does not have", gid)
		}
	}
}

// seacChain is a CFF in which glyph k+1 is a seac of glyph k, n links long:
// glyph 1 draws, and every glyph after it names the one before as both base
// and accent. The glyphs are named by StandardEncoding codes 65 upwards, which
// the charstring holds in one byte up to 107.
func seacChain(t *testing.T, n int) []byte {
	t.Helper()
	if n < 1 || 65+n-1 > 107 {
		t.Fatalf("a chain of %d does not fit one-byte codes", n)
	}
	const endchar = 14
	sids := make([]int, n)
	charstrings := [][]byte{{endchar}}
	for k := 0; k < n; k++ {
		name, ok := font.StandardEncodingName(byte(65 + k))
		if !ok {
			t.Fatalf("StandardEncoding names nothing at %d", 65+k)
		}
		sid, ok := font.CFFStandardSID(name)
		if !ok {
			t.Fatalf("%q is not a predefined CFF string", name)
		}
		sids[k] = sid
		if k == 0 {
			charstrings = append(charstrings, []byte{cffNumber(100)[0], 22, endchar})
			continue
		}
		prev := cffNumber(65 + k - 1)
		c := append(append(cffNumber(0), cffNumber(0)...), prev...)
		charstrings = append(charstrings, append(append(c, prev...), endchar))
	}
	return fonttest.CFF(fonttest.CFFOptions{Glyphs: n + 1, CharsetSIDs: sids, Charstrings: charstrings})
}

// TestTheSeacClosureWalksEachGlyphOnce. The closure was a fixed point: every
// kept charstring walked again each round, and a round adds one link of a
// chain, so a chain of n cost n²/2 walks. It is a worklist now, and what it
// spends is charged to the budget, so the spending is the count.
func TestTheSeacClosureWalksEachGlyphOnce(t *testing.T) {
	spent := func(n int) int {
		cff := seacChain(t, n)
		keep := make([]bool, n+1)
		keep[0], keep[n] = true, true // .notdef and the top of the chain
		b := fullBudget()
		if err := cffSeacClosure(cff, keep, b); err != nil {
			t.Fatalf("a chain of %d: %v", n, err)
		}
		for g, k := range keep {
			if !k {
				t.Fatalf("a chain of %d: glyph %d was not kept, and the top of the "+
					"chain is drawn from it", n, g)
			}
		}
		return b.Spent()
	}
	short, long := spent(10), spent(40)
	// Four times the chain is four times the work walked once, sixteen times it
	// walked per round.
	if ratio := float64(long) / float64(short); ratio > 5 {
		t.Errorf("a chain four times as long cost %.1f times as much (%d against %d); "+
			"the closure walks glyphs it has walked before", ratio, long, short)
	}
}

// TestTheSeacClosureStopsWhereTheBudgetDoes: a walk the budget refuses is an
// error, not a seac quietly unread and its components dropped.
func TestTheSeacClosureStopsWhereTheBudgetDoes(t *testing.T) {
	cff := seacChain(t, 20)
	keep := make([]bool, 21)
	keep[20] = true
	if err := cffSeacClosure(cff, keep, font.NewBudget(30)); err == nil ||
		!strings.Contains(err.Error(), "units of work") {
		t.Errorf("a closure given 30 units for a chain of 20 seacs returned %v, "+
			"want the budget's error", err)
	}
}

// TestASubsetOfSeacWalksPastTheBudgetIsRefused goes through Load and Subset. Each
// glyph's charstring sets its width with an hmoveto — which is as far as Load
// reads — and then calls a subroutine tree the seac walk follows to its
// operator bound. Sixty-four such glyphs are the allowance; this uses a hundred.
func TestASubsetOfSeacWalksPastTheBudgetIsRefused(t *testing.T) {
	const fanOut, depth, callsubr, ret, endchar = 16, 4, 10, 11, 14
	operand := func(subr int) byte { return byte(subr - 107 + 139) }
	subrs := make([][]byte, depth+1)
	for i := 0; i < depth; i++ {
		for k := 0; k < fanOut; k++ {
			subrs[i] = append(subrs[i], operand(i+1), callsubr)
		}
		subrs[i] = append(subrs[i], ret)
	}
	subrs[depth] = []byte{ret}

	const n = 100
	charstrings := [][]byte{{endchar}}
	var glyphs []fonttest.Glyph
	var text []rune
	for len(charstrings) <= n {
		charstrings = append(charstrings, []byte{cffNumber(0)[0], 22, operand(0), callsubr, endchar})
		r := rune(0x4E00 + len(glyphs))
		glyphs = append(glyphs, fonttest.Glyph{Rune: r, Advance: 500, HasShape: true})
		text = append(text, r)
	}
	cff := fonttest.CFF(fonttest.CFFOptions{Glyphs: n + 1, Subrs: subrs, Charstrings: charstrings})
	load := func() *Face {
		f, err := Load(fonttest.OTTO(cff, fonttest.SFNTOptions{Glyphs: glyphs}))
		if err != nil {
			t.Fatalf("loading: %v", err)
		}
		return f
	}

	// The control: a subset of forty of them is within the allowance.
	f := load()
	if _, missing := f.Encode(string(text[:40])); missing != 0 {
		t.Fatalf("%d runes of the fixture are missing", missing)
	}
	if _, err := f.Subset(); err != nil {
		t.Fatalf("a subset of forty glyphs: %v; the fixture is not the one meant", err)
	}

	f = load()
	if _, missing := f.Encode(string(text)); missing != 0 {
		t.Fatalf("%d runes of the fixture are missing", missing)
	}
	if _, err := f.Subset(); err == nil || !strings.Contains(err.Error(), "units of work") {
		t.Errorf("a subset whose seac walks come to %d times the operator bound "+
			"returned %v, want the budget's error", n, err)
	}
}

// cidPrivate is where a Font DICT's Private DICT is: at bytes into the
// privates cidCFF puts at the end of the font, size bytes long.
type cidPrivate struct{ at, size int }

// cidCFF builds a CID-keyed CFF of the given number of glyphs, every one in
// Font DICT 0, with one Font DICT for each of fds and privates at the end.
func cidCFF(glyphs int, fds []cidPrivate, privates []byte) []byte {
	topDict := func(cs, fdArray, fdSelect int) []byte {
		var d []byte
		for i := 0; i < 3; i++ {
			d = append(d, cffInt(0)...) // Registry, Ordering, Supplement
		}
		d = append(d, 12, 30) // ROS
		d = append(d, cffInt(cs)...)
		d = append(d, byte(opCharStrings))
		d = append(d, cffInt(fdArray)...)
		d = append(d, 12, 36)
		d = append(d, cffInt(fdSelect)...)
		return append(d, 12, 37)
	}
	fontDicts := func(base int) []byte {
		items := make([][]byte, len(fds))
		for i, fd := range fds {
			d := append(cffInt(fd.size), cffInt(base+fd.at)...)
			items[i] = append(d, byte(opPrivate))
		}
		return writeCFFIndex(items)
	}
	charstrings := make([][]byte, glyphs)
	for i := range charstrings {
		charstrings[i] = []byte{14}
	}
	head := []byte{1, 0, 4, 4}
	name := writeCFFIndex([][]byte{[]byte("CID")})
	rest := writeCFFIndex(nil) // String INDEX, and the Global Subr INDEX after it
	topLen := len(writeCFFIndex([][]byte{topDict(0, 0, 0)}))
	csAt := len(head) + len(name) + topLen + 2*len(rest)
	cs := writeCFFIndex(charstrings)
	fdSelectAt := csAt + len(cs)
	fdSelect := make([]byte, 1+glyphs) // format 0, every glyph in Font DICT 0
	fdArrayAt := fdSelectAt + len(fdSelect)
	privAt := fdArrayAt + len(fontDicts(0))

	var out []byte
	out = append(out, head...)
	out = append(out, name...)
	out = append(out, writeCFFIndex([][]byte{topDict(csAt, fdArrayAt, fdSelectAt)})...)
	out = append(out, rest...)
	out = append(out, rest...)
	out = append(out, cs...)
	out = append(out, fdSelect...)
	out = append(out, fontDicts(privAt)...)
	return append(out, privates...)
}

// privateWithSubrs is a Private DICT naming local subroutines at subrsAt from
// its start: six bytes.
func privateWithSubrs(subrsAt int) []byte {
	return append(cffInt(subrsAt), byte(opSubrs))
}

// returns is an INDEX of n subroutines that only return.
func returns(n int) []byte {
	items := make([][]byte, n)
	for i := range items {
		items[i] = []byte{11}
	}
	return writeCFFIndex(items)
}

// subsetPrivates reads back, for each Font DICT of a CID-keyed subset, where
// its Private DICT is and how many local subroutines it names.
func subsetPrivates(t *testing.T, cff []byte) (offsets []int, subrs []int) {
	t.Helper()
	top, err := topDictOf(cff)
	if err != nil {
		t.Fatalf("reading the subset's Top DICT: %v", err)
	}
	fdArray := 0
	for _, e := range top {
		if e.op == opFDArray && len(e.operands) == 1 {
			fdArray = e.operands[0]
		}
	}
	idx, err := readCFFIndex(cff, fdArray)
	if err != nil {
		t.Fatalf("reading the subset's FDArray: %v", err)
	}
	for i, fd := range idx.items {
		ops, _, err := parseCFFDict(fd)
		if err != nil {
			t.Fatalf("Font DICT %d: %v", i, err)
		}
		size, off := 0, 0
		for _, e := range ops {
			if e.op == opPrivate && len(e.operands) == 2 {
				size, off = e.operands[0], e.operands[1]
			}
		}
		priv, _, err := parseCFFDict(cff[off : off+size])
		if err != nil {
			t.Fatalf("Font DICT %d's Private DICT: %v", i, err)
		}
		n := 0
		for _, e := range priv {
			if e.op == opSubrs && len(e.operands) == 1 {
				sub, err := readCFFIndex(cff, off+e.operands[0])
				if err != nil {
					t.Fatalf("Font DICT %d's local subroutines: %v", i, err)
				}
				n = len(sub.items)
			}
		}
		offsets = append(offsets, off)
		subrs = append(subrs, n)
	}
	return offsets, subrs
}

func keepAll(n int) []bool {
	keep := make([]bool, n)
	for i := range keep {
		keep[i] = true
	}
	return keep
}

// TestACIDSubsetKeepsItsSubroutinesWithTheirDict is the control for the three
// below: a CID-keyed font whose two Font DICTs each have a Private DICT and
// subroutines of their own, one with bytes between the DICT and its INDEX.
func TestACIDSubsetKeepsItsSubroutinesWithTheirDict(t *testing.T) {
	first := append(privateWithSubrs(6), returns(3)...)
	second := append(append(privateWithSubrs(6+5), make([]byte, 5)...), returns(4)...)
	cff := cidCFF(3, []cidPrivate{{0, 6}, {len(first), 6}}, append(first, second...))
	out, err := subsetCFF(cff, keepAll(3), fullBudget())
	if err != nil {
		t.Fatalf("subsetting: %v", err)
	}
	offsets, subrs := subsetPrivates(t, out)
	if len(subrs) != 2 || subrs[0] != 3 || subrs[1] != 4 || offsets[0] == offsets[1] {
		t.Errorf("the subset's Font DICTs name Private DICTs at %v with %v subroutines, "+
			"want two apart with 3 and 4", offsets, subrs)
	}
}

// TestACIDSubsetRefusesSubroutinesInsideTheirDict is audit C182: a Subrs
// operand pointing inside its own Private DICT. The non-CID path refused it;
// the CID-keyed path copied from the DICT's start to that INDEX's end — here
// five bytes of an eight-byte DICT — while the Font DICT went on saying eight,
// so a reader of the subset took three bytes of whatever came next for the end
// of the DICT.
func TestACIDSubsetRefusesSubroutinesInsideTheirDict(t *testing.T) {
	// defaultWidthX 0, then Subrs at 3: the zero bytes of its own operand,
	// which read as an empty INDEX ending five bytes in.
	priv := append([]byte{139, 20}, privateWithSubrs(3)...)
	cff := cidCFF(2, []cidPrivate{{0, len(priv)}}, priv)
	if _, err := subsetCFF(cff, keepAll(2), fullBudget()); err == nil ||
		!strings.Contains(err.Error(), "inside itself") {
		t.Errorf("a CID-keyed Private DICT naming subroutines inside itself was "+
			"subsetted (%v); the Top DICT's Private is refused for it", err)
	}
}

// TestFontDictsSharingAPrivateDictShareItsCopy. Three hundred Font DICTs naming
// one Private DICT and its three thousand subroutines were three hundred copies
// of it in the subset: a font of 13KB made a subset of 3MB.
func TestFontDictsSharingAPrivateDictShareItsCopy(t *testing.T) {
	const fds, n = 300, 3000
	priv := append(privateWithSubrs(6), returns(n)...)
	shared := make([]cidPrivate, fds)
	for i := range shared {
		shared[i] = cidPrivate{0, 6}
	}
	cff := cidCFF(2, shared, priv)
	out, err := subsetCFF(cff, keepAll(2), fullBudget())
	if err != nil {
		t.Fatalf("subsetting: %v", err)
	}
	if len(out) > len(cff)+len(cff)/10 {
		t.Errorf("a font of %d bytes made a subset of %d; the Private DICT the Font "+
			"DICTs share was copied for each", len(cff), len(out))
	}
	offsets, subrs := subsetPrivates(t, out)
	for i := range offsets {
		if offsets[i] != offsets[0] || subrs[i] != n {
			t.Fatalf("Font DICT %d names a Private DICT at %d with %d subroutines; "+
				"want the one at %d with %d", i, offsets[i], subrs[i], offsets[0], n)
		}
	}
}

// TestOverlappingPrivateDictsAreRefused: fifty Private DICTs six bytes apart,
// each naming the one subroutine INDEX behind them all. They are fifty regions,
// each running to the end of that INDEX, and copying each made the subset
// fifty times the INDEX.
func TestOverlappingPrivateDictsAreRefused(t *testing.T) {
	const k, n = 50, 3000
	var privates []byte
	fds := make([]cidPrivate, k)
	for i := range fds {
		fds[i] = cidPrivate{6 * i, 6}
		privates = append(privates, privateWithSubrs(6*(k-i))...)
	}
	privates = append(privates, returns(n)...)
	cff := cidCFF(2, fds, privates)
	out, err := subsetCFF(cff, keepAll(2), fullBudget())
	if err == nil {
		t.Errorf("fifty overlapping Private DICTs were subsetted, from %d bytes to %d",
			len(cff), len(out))
	} else if !strings.Contains(err.Error(), "overlap") {
		t.Errorf("fifty overlapping Private DICTs: %v, want the overlap refused", err)
	}
}

// TestTheCIDSubsetChargesWhatItReads: the Font DICTs, Private DICTs and
// subroutine INDEXes are charged to the budget the subset is given.
func TestTheCIDSubsetChargesWhatItReads(t *testing.T) {
	const fds = 40
	var privates []byte
	spec := make([]cidPrivate, fds)
	for i := range spec {
		region := append(privateWithSubrs(6), returns(100)...)
		spec[i] = cidPrivate{len(privates), 6}
		privates = append(privates, region...)
	}
	cff := cidCFF(2, spec, privates)
	b := fullBudget()
	if _, err := subsetCFF(cff, keepAll(2), b); err != nil {
		t.Fatalf("subsetting: %v", err)
	}
	if b.Spent() < fds*100 {
		t.Errorf("the subset of %d Font DICTs of a hundred subroutines each spent %d "+
			"units; it reads at least %d INDEX entries", fds, b.Spent(), fds*100)
	}
	if _, err := subsetCFF(cff, keepAll(2), font.NewBudget(1000)); err == nil ||
		!strings.Contains(err.Error(), "units of work") {
		t.Errorf("a subset given 1000 units for 4000 subroutines returned %v, "+
			"want the budget's error", err)
	}
}
