package shape

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// The subroutines a name-keyed CFF's subset keeps, held to fontTools.
//
// testdata/harfbuzz/cffsubrs.py runs fontTools' subsetter, keeping
// subroutines rather than inlining them, over sets of glyphs of three
// name-keyed faces — the CFFInk.otf fixture, and the static Source Sans 3 and
// Source Serif 4 (CFF_FONTS) — and writes which of the original global and
// local subroutines survive each. The answers are checked in as
// cffsubrs.expected.txt. What survives is what some kept glyph reaches,
// however deeply, so fontTools and this package have to agree subroutine for
// subroutine.
//
// And what is kept has to draw what it drew: every kept glyph of every subset
// is run before and after, and its outline — each move, line and curve, point
// for point — has to be the one it had. Renumbering a subroutine changes the
// number at every call site that names it, and a call site read wrongly draws
// some other piece of the font.

// cffSubrsFaces reads each face the expectations were generated from.
var cffSubrsFaces = map[string]func(t *testing.T) []byte{
	"CFFInk.otf":               func(t *testing.T) []byte { return harfbuzzFont(t, "CFFInk.otf") },
	"SourceSans3-Regular.otf":  func(t *testing.T) []byte { return fonttest.CFFFile(t, "SourceSans3-Regular.otf") },
	"SourceSerif4-Regular.otf": func(t *testing.T) []byte { return fonttest.CFFFile(t, "SourceSerif4-Regular.otf") },
}

// subrsCase is one set of glyphs asked about: what fontTools kept, and the
// subroutines it kept with them, by their original index.
type subrsCase struct {
	label         string
	glyphs        []int
	global, local []int
}

type subrsFace struct {
	name, sum string
	cases     []subrsCase
}

func readSubrsGolden(t *testing.T) []*subrsFace {
	t.Helper()
	path := filepath.Join(harfbuzzDir, "cffsubrs.expected.txt")
	refuseUnpinnedOracle(t, path)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("%v (make cffsubrs)", err)
	}
	defer f.Close()
	ints := func(s string) []int {
		var out []int
		for _, w := range strings.Fields(s) {
			v, err := strconv.Atoi(w)
			if err != nil {
				t.Fatalf("%s: %q is not a number", path, w)
			}
			out = append(out, v)
		}
		return out
	}
	var faces []*subrsFace
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		key, rest, _ := strings.Cut(line, " ")
		var c *subrsCase
		if n := len(faces); n > 0 && len(faces[n-1].cases) > 0 {
			c = &faces[n-1].cases[len(faces[n-1].cases)-1]
		}
		switch key {
		case "face":
			name, sum, _ := strings.Cut(rest, " ")
			faces = append(faces, &subrsFace{name: name, sum: sum})
		case "case":
			faces[len(faces)-1].cases = append(faces[len(faces)-1].cases, subrsCase{label: rest})
		case "glyphs":
			c.glyphs = ints(rest)
		case "global":
			c.global = ints(rest)
		case "local":
			c.local = ints(rest)
		default:
			t.Fatalf("%s: a line this does not read: %q", path, line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(faces) != len(cffSubrsFaces) {
		t.Fatalf("%s holds %d faces and the test reads %d", path, len(faces), len(cffSubrsFaces))
	}
	return faces
}

// cffOutline is every segment glyph gid of a CFF program draws, and whether it
// can be drawn at all.
func cffOutline(o *cffOutlines, gid int) ([]t2Seg, bool) {
	var segs []t2Seg
	r := t2Run{o: o, budget: font.NewBudget(maxFontWork), draw: true,
		path: func(s t2Seg) { segs = append(segs, s) }}
	_, ok := r.bounds(gid, false)
	return segs, ok
}

// usedSubrs is which of the subroutines the glyphs keep reaches, by original
// index, as the subsetter's walk finds them — or refused, the glyph whose
// charstring the walk cannot read exactly, for which the subset keeps every
// subroutine whole.
func usedSubrs(t *testing.T, cff []byte, keep []bool) (global, local []int, refused int) {
	t.Helper()
	charStrings, err := CharStringsForTest(cff)
	if err != nil {
		t.Fatal(err)
	}
	gsubrs, err := cffGlobalSubrs(cff)
	if err != nil {
		t.Fatal(err)
	}
	var locals [][][]byte
	region := -1
	if priv, err := PrivateDictForTest(cff); err != nil {
		t.Fatal(err)
	} else if priv != nil {
		top, _ := topDictOf(cff)
		for _, e := range top {
			if e.op == opPrivate {
				_, _, subrs, err := privateParts(cff[e.operands[1]:], e.operands[0])
				if err != nil {
					t.Fatal(err)
				}
				locals, region = [][][]byte{subrs}, 0
			}
		}
	}
	p := newSubrPruning(gsubrs, locals)
	for g, k := range keep {
		if k {
			if err := p.walk(g, charStrings[g], region, font.NewBudget(maxFontWork)); err != nil {
				if errors.Is(err, errKeepSubrs) {
					return nil, nil, g
				}
				t.Fatalf("walking glyph %d: %v", g, err)
			}
		}
	}
	for i, u := range p.usedG {
		if u {
			global = append(global, i)
		}
	}
	if region == 0 {
		for i, u := range p.usedL[0] {
			if u {
				local = append(local, i)
			}
		}
	}
	return global, local, -1
}

// localSubrsOf is a name-keyed CFF's local subroutines.
func localSubrsOf(t *testing.T, cff []byte) [][]byte {
	t.Helper()
	top, err := topDictOf(cff)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range top {
		if e.op == opPrivate && len(e.operands) == 2 {
			_, _, subrs, err := privateParts(cff[e.operands[1]:], e.operands[0])
			if err != nil {
				t.Fatal(err)
			}
			return subrs
		}
	}
	return nil
}

// refusedByTheWalk are the glyphs whose calls the subsetter's walk does not
// try to renumber, so that a subset keeping one keeps every subroutine whole
// where fontTools prunes. Each is a fixture glyph built past what Type 2
// allows: far, far.left, stack.fits and stack.full (69, 70, 75, 74) push more
// than the 48 operands its stack holds; reserved (72) runs operators it
// reserves; return.top (76) returns from a glyph; subr.negative (91) calls a
// subroutine the font does not have; and subr.depth11 (94) nests eleven deep,
// one past the limit. fontTools reads them anyway. The walk will not guess
// what a reader that follows the format would make of them, and gives up the
// saving instead — the subset is larger and exactly as correct. No real face
// has one.
var refusedByTheWalk = map[string]map[int]bool{
	"CFFInk.otf": {69: true, 70: true, 72: true, 74: true, 75: true, 76: true, 91: true, 94: true},
}

func allIndices(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

func mustGlobal(t *testing.T, cff []byte) [][]byte {
	t.Helper()
	g, err := cffGlobalSubrs(cff)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestANameKeyedSubsetKeepsTheSubroutinesFontToolsKeeps(t *testing.T) {
	for _, face := range readSubrsGolden(t) {
		t.Run(face.name, func(t *testing.T) {
			load, ok := cffSubrsFaces[face.name]
			if !ok {
				t.Fatalf("the expectations name %s, which the test does not read", face.name)
			}
			data := load(t)
			sum := sha256.Sum256(data)
			if hex.EncodeToString(sum[:]) != face.sum {
				t.Fatalf("%s is not the face the expectations were generated from", face.name)
			}
			cff := font.SFNTTables(data)["CFF "]
			charStrings, err := CharStringsForTest(cff)
			if err != nil {
				t.Fatal(err)
			}
			n := len(charStrings)
			before, err := readCFFOutlines(cff, n)
			if err != nil {
				t.Fatal(err)
			}
			// Which glyphs, asked about alone, the walk refused: exactly the
			// ones refusedByTheWalk names, so that neither a new refusal nor
			// the list itself can drift.
			seenRefused := map[int]bool{}
			defer func() {
				if !maps.Equal(seenRefused, refusedByTheWalk[face.name]) && !t.Failed() {
					t.Errorf("the walk refused glyphs %v on their own; refusedByTheWalk names %v",
						slices.Sorted(maps.Keys(seenRefused)), slices.Sorted(maps.Keys(refusedByTheWalk[face.name])))
				}
			}()
			for _, c := range face.cases {
				keep := make([]bool, n)
				for _, g := range c.glyphs {
					keep[g] = true
				}
				closed := slices.Clone(keep)
				if err := cffSeacClosure(cff, closed, fullBudget()); err != nil {
					t.Fatalf("%s: the seac closure: %v", c.label, err)
				}
				if !slices.Equal(closed, keep) {
					t.Errorf("%s: the seac closure of what fontTools kept is larger than it", c.label)
				}

				global, local, refused := usedSubrs(t, cff, keep)
				wantG, wantL := c.global, c.local
				if refused >= 0 && len(c.glyphs) <= 2 {
					seenRefused[refused] = true
				}
				if refused >= 0 {
					if !refusedByTheWalk[face.name][refused] {
						t.Errorf("%s: the walk cannot renumber glyph %d's calls", c.label, refused)
					}
					// Kept whole: every subroutine the font has.
					wantG, wantL = allIndices(len(mustGlobal(t, cff))), allIndices(len(localSubrsOf(t, cff)))
				} else if !slices.Equal(global, c.global) || !slices.Equal(local, c.local) {
					t.Errorf("%s: the walk reaches global %v and local %v;\nfontTools keeps global %v and local %v",
						c.label, global, local, c.global, c.local)
				}

				sub, order, err := subsetCFF(cff, keep, fullBudget())
				if err != nil {
					t.Fatalf("%s: subsetting: %v", c.label, err)
				}
				if order != nil {
					t.Fatalf("%s: a name-keyed subset was renumbered", c.label)
				}
				gotG, err := cffGlobalSubrs(sub)
				if err != nil {
					t.Fatal(err)
				}
				gotL := localSubrsOf(t, sub)
				if len(gotG) != len(wantG) || len(gotL) != len(wantL) {
					t.Errorf("%s: the subset carries %d global and %d local subroutines; "+
						"want %d and %d", c.label, len(gotG), len(gotL), len(wantG), len(wantL))
				}

				after, err := readCFFOutlines(sub, n)
				if err != nil {
					t.Fatalf("%s: the subset cannot be read: %v", c.label, err)
				}
				for _, g := range c.glyphs {
					want, wantOK := cffOutline(before, g)
					got, gotOK := cffOutline(after, g)
					if gotOK != wantOK || !slices.Equal(got, want) {
						t.Errorf("%s: glyph %d draws %d segments (ok %v) and drew %d (ok %v)",
							c.label, g, len(got), gotOK, len(want), wantOK)
					}
				}
			}
		})
	}
}

// TestANameKeyedSubsetRunsEachSubroutineOncePerState is the cost shape of the
// walk for a name-keyed font, as TestASubroutineIsRunOncePerStateItIsEnteredIn
// is for a CID-keyed one: a chain of subroutines each calling the next fanOut
// times is fanOut^depth calls run in full and fanOut·depth remembered. So
// doubling fanOut about doubles what the budget is charged, where running
// every call would multiply it by 2^depth.
func TestANameKeyedSubsetRunsEachSubroutineOncePerState(t *testing.T) {
	const depth = 8
	spent := func(fanOut int) int {
		subrs := make([][]byte, depth+1)
		for i := 0; i < depth; i++ {
			for k := 0; k < fanOut; k++ {
				subrs[i] = cat(subrs[i], t2(i+1-107), opCallsubr)
			}
			subrs[i] = cat(subrs[i], opReturn)
		}
		subrs[depth] = line(1, 1)
		cff := fonttest.CFF(fonttest.CFFOptions{
			Glyphs: 2,
			Subrs:  subrs,
			Charstrings: [][]byte{opEndchar,
				cat(t2(10), t2(10), opRmoveto, t2(0-107), opCallsubr, opEndchar)},
		})
		b := font.NewBudget(maxFontWork)
		sub, _, err := subsetCFF(cff, []bool{true, true}, b)
		if err != nil {
			t.Fatalf("fanOut %d: %v", fanOut, err)
		}
		if got := len(localSubrsOf(t, sub)); got != depth+1 {
			t.Fatalf("fanOut %d: the subset carries %d local subroutines of %d, all of them called",
				fanOut, got, depth+1)
		}
		return b.Spent()
	}
	small, large := spent(4), spent(8)
	if ratio := float64(large) / float64(small); ratio > 3 {
		t.Errorf("doubling the fan-out took the work from %d to %d units, %.1fx; running each "+
			"subroutine once per entry state is about 2x", small, large, ratio)
	}
}

// TestANameKeyedSubsetIsRefusedPastItsBudget: a walk that runs out of budget
// is refused with the budget's error, not answered from what was walked — a
// subset that dropped the subroutines the unwalked glyphs call would draw
// nothing for them. The budget is set one unit short of what the subset
// spends, and then to exactly that.
func TestANameKeyedSubsetIsRefusedPastItsBudget(t *testing.T) {
	subrs := [][]byte{cat(t2(1-107), opCallsubr, t2(1-107), opCallsubr, opReturn), line(1, 1)}
	glyphs := [][]byte{opEndchar}
	for g := 0; g < 40; g++ {
		glyphs = append(glyphs, cat(t2(10), t2(g), opRmoveto, t2(g%2-107), opCallsubr, opEndchar))
	}
	cff := fonttest.CFF(fonttest.CFFOptions{Glyphs: len(glyphs), Subrs: subrs, Charstrings: glyphs})
	keep := make([]bool, len(glyphs))
	for g := range keep {
		keep[g] = true
	}
	b := fullBudget()
	if _, _, err := subsetCFF(cff, keep, b); err != nil {
		t.Fatalf("the subset with the whole allowance: %v", err)
	}
	need := b.Spent()
	if _, _, err := subsetCFF(cff, keep, font.NewBudget(need-1)); err == nil {
		t.Errorf("a subset whose walk needs %d units was written with %d", need, need-1)
	}
	if _, _, err := subsetCFF(cff, keep, font.NewBudget(need)); err != nil {
		t.Errorf("a subset whose walk needs %d units was refused with exactly that: %v", need, err)
	}
}
