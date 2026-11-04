package shape

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/mgilbir/forme/font"
)

// Whether a glyph can be kept: the subsetter's decision about one glyph, asked
// before anything is drawn with it.
//
// A font can be sound as a whole and still have glyphs whose data a subset
// must not carry: an instruction length that runs past the glyph's record, a
// composite cut off part way through a component, a component the font does
// not have, a charstring that cannot be run. A document that meets such a
// glyph wants to keep the rest of the face as real text and route only the
// runs that use the bad glyph some other way — drawn as outlines, or kept as
// invisible text — and to do that it has to know before the glyph is drawn,
// since what is drawn is what the subset is made from.
//
// So the decision is made here once, per glyph, and the subset makes no other:
// Subset asks it of every glyph the face used, and refuses with a GlyphError
// naming those it was refused for. A glyph CheckGlyph accepts is one the subset
// carries as the font states it, with everything it is built from; one it
// refuses is never copied into a subset, verbatim or otherwise.
//
// # What makes a glyph keepable
//
// It depends on what the outlines are, and in each case the test is the one
// the face already applies to the same data elsewhere rather than a reading of
// its own:
//
//   - A glyf glyph is decoded as the instancer decodes it (decodeVarGlyph),
//     which reads the whole record against the specification: the header, the
//     contour ends, the instructions, every flag and coordinate of a simple
//     glyph, and every component record of a composite, each naming a glyph
//     the font has. Its components are then decided in turn, and a composite
//     built, through its components, from itself is refused: a subset that
//     copies it hands a reader a glyph with no bottom. HarfBuzz reads a glyph
//     more leniently than this — it skips a component past the end of the font
//     and draws the rest — and that is the right reading for drawing a glyph
//     and the wrong one for deciding to carry it: a subset that copies the
//     record copies the fault along with it, to readers less lenient than
//     HarfBuzz.
//   - A glyph a VARC table composes, in a glyf face, is written into the subset
//     as the outline it draws (varcsubset.go), and is keepable when that
//     outline can be written. Its own glyf entry, a placeholder, is not
//     carried, and neither are the components that entry names. A glyf
//     composite naming one as a component cannot be kept: it would draw the
//     placeholder in the subset and the composition in the face.
//   - A CFF glyph's charstring is a program, and is keepable when the face can
//     run it, as it runs it to measure and draw it (cffink.go): within the
//     stack, the subroutines it calls present, nested no deeper than ten, no
//     more operators than HarfBuzz runs, and a seac's two glyphs present and
//     runnable themselves. A seac is the one way a CFF glyph is built from
//     others, and running it runs them.
//   - A CFF2 glyph is carried as the CFF charstring it is written as
//     (cff2cff.go), and is keepable when it could be written.
//   - A glyph a VARC table composes, in a CFF or CFF2 face, cannot be kept: a
//     CFF charstring cannot carry what VARC draws.
//
// # What is not a glyph's fault
//
// A face whose tables the subsetter cannot take apart at all — a glyf face with
// too short a loca, a CFF table the face's own reader refuses, a standard face
// with no program — has no glyph that can be kept, and every glyph is refused
// with the same error, which is not a GlyphError: no choice of glyphs would
// help. The same goes for running out of the work a face may spend, which is a
// limit and not a fault in any one glyph: CFF2 writes its glyphs under one
// allowance for the face, and a glyph written after it ran out is refused with
// an error saying so. A subset can also fail on the face's tables after every
// glyph has been decided — the CFF subsetter reads structures no glyph's check
// needs — and that error is not a GlyphError either.
//
// # .notdef
//
// Glyph 0 is in every subset whether the face used it or not, because the
// format requires it. When the face did not use it and it cannot be kept, it
// is written as an empty glyph rather than refusing the subset: nothing the
// document drew draws it, and an empty .notdef is a valid one. When the face
// did use it, it is decided like any other glyph.
//
// # What it costs
//
// A glyph is decided once per face and the answer kept, shared by the face's
// clones as the outlines are, so it is cheap to ask as each glyph is first
// drawn. A simple glyph is decided from its own record; a composite walks its
// components, each of which is then decided too. A CFF glyph's charstring is
// run under an allowance of its own, as its points are (cffPointSegments), so
// that deciding glyphs spends nothing the face measures or draws with.

// GlyphError is a subset refused because of glyphs whose data cannot be
// carried as the font states it. GIDs are the glyphs, among those the face
// used, that could not be kept, in ascending order, and Err says why the
// first of them could not. CheckGlyph says why for any one of them.
//
// A caller that gets one can stop using those glyphs — draw the text that
// needs them some other way — and subset a face that has not used them.
type GlyphError struct {
	GIDs []int
	Err  error
}

func (e *GlyphError) Error() string {
	if len(e.GIDs) == 1 {
		return e.Err.Error()
	}
	shown := e.GIDs[:min(len(e.GIDs), 8)]
	parts := make([]string, len(shown))
	for i, g := range shown {
		parts[i] = fmt.Sprint(g)
	}
	more := ""
	if len(e.GIDs) > len(shown) {
		more = ", …"
	}
	return fmt.Sprintf("fonts: %d glyphs cannot be kept in a subset (%s%s); the first: %v",
		len(e.GIDs), strings.Join(parts, ", "), more, e.Err)
}

func (e *GlyphError) Unwrap() error { return e.Err }

// CheckGlyph reports whether a subset can keep glyph gid as the font states
// it, with everything it is built from: nil when it can, and otherwise why
// not. A refusal that is the glyph's own — its data, or that of a glyph it is
// built from — is a *GlyphError naming gid; one that would refuse every glyph
// of the face, or that is a limit the face ran into rather than a fault of the
// glyph's, is not. See glyphcheck.go for what is checked.
//
// It is the decision Subset and SubsetGlyphs make for each glyph the face used,
// so a face that has used only glyphs CheckGlyph accepts is refused by neither
// with a GlyphError. It is cheap to ask as each glyph is first drawn: each
// glyph is decided once per face, and its clones share the answers.
func (f *Face) CheckGlyph(gid int) error {
	if err := f.subsettable(); err != nil {
		return err
	}
	if gid < 0 || gid >= f.prog.NumGlyphs {
		return &GlyphError{GIDs: []int{gid},
			Err: fmt.Errorf("fonts: glyph %d is not one of the face's %d glyphs", gid, f.prog.NumGlyphs)}
	}
	fault, err := f.glyphFault(gid)
	if err != nil {
		return err
	}
	if fault != nil {
		return &GlyphError{GIDs: []int{gid}, Err: fault.describe(gid)}
	}
	return nil
}

// subsettable is why no glyph of the face can be kept, or nil.
func (f *Face) subsettable() error {
	switch {
	case f.std != nil || f.prog == nil:
		return errors.New("fonts: a standard font has no program to subset")
	case f.bitmapOnly:
		return errors.New("fonts: the font's glyphs are only bitmaps, and a font program " +
			"embedded in a document has to carry outlines")
	case f.prog.NumGlyphs <= 0:
		// A font declaring no glyphs at all. Every sfnt has .notdef at index
		// zero, so this is a malformed maxp rather than an empty font.
		return fmt.Errorf("fonts: the font declares %d glyphs; every font has at least .notdef", f.prog.NumGlyphs)
	case f.cff2 != nil:
		return nil
	case f.cff:
		if f.ink == nil {
			return errors.New("fonts: the font no longer carries a CFF table")
		}
		if f.ink.load(); f.ink.outlines == nil {
			return errors.New("fonts: the CFF table is one HarfBuzz's reader refuses, so none of its glyphs can be run")
		}
		return nil
	}
	_, err := f.glyfTables()
	return err
}

// glyphFault is glyph gid's verdict: nil when it can be kept, the fault when it
// cannot, and an error for a refusal that is not the glyph's (see CheckGlyph).
// The face must be subsettable and gid one of its glyphs.
func (f *Face) glyphFault(gid int) (*glyphFault, error) {
	c := f.checks
	if c == nil {
		c = newGlyphChecks()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if v, ok := c.verdicts[gid]; ok {
		return v, nil
	}
	if f.varc != nil && f.varc.t.coverageIndex(gid) >= 0 && (f.cff2 != nil || f.cff) {
		v := &glyphFault{gid: gid, err: errors.New("is a variable composite (VARC) in a CFF face, " +
			"whose drawn outline a CFF program cannot carry")}
		c.verdicts[gid] = v
		return v, nil
	}
	switch {
	case f.cff2 != nil:
		err := f.cff2.check(gid)
		var lim *cff2Limit
		if errors.As(err, &lim) {
			return nil, err
		}
		var v *glyphFault
		if err != nil {
			v = &glyphFault{gid: gid, err: err}
		}
		c.verdicts[gid] = v
		return v, nil
	case f.cff:
		v := f.cffFault(gid)
		c.verdicts[gid] = v
		return v, nil
	}
	t, err := f.glyfTables()
	if err != nil {
		return nil, err
	}
	return c.glyf(f, t, gid), nil
}

// glyphChecks is what a face has decided about its glyphs, shared by its
// clones: the decision is about the font's own data, which no document
// changes. The lock is there because it is filled in lazily.
type glyphChecks struct {
	mu sync.Mutex
	// verdicts holds each glyph decided so far: nil for one that can be kept,
	// and the fault for one that cannot. A refusal that is not a fault of the
	// glyph's is not kept here, and is asked again.
	verdicts map[int]*glyphFault
}

func newGlyphChecks() *glyphChecks {
	return &glyphChecks{verdicts: map[int]*glyphFault{}}
}

// glyphFault is why a glyph cannot be kept: the glyph whose own data is at
// fault — the glyph asked about, or one it is built from — and what is wrong
// with it.
type glyphFault struct {
	gid int
	err error
}

// describe is the fault as the reason glyph gid cannot be kept.
func (v *glyphFault) describe(gid int) error {
	if v.gid == gid {
		return fmt.Errorf("fonts: glyph %d %w", gid, v.err)
	}
	return fmt.Errorf("fonts: glyph %d is built from glyph %d, which %w", gid, v.gid, v.err)
}

// glyf decides a glyph of a TrueType face, and every glyph it is built from.
//
// It is a depth-first walk over the components, made without recursion so
// that a chain of composites as long as the font allows costs a slice and not
// a stack, and it decides each glyph it reaches once: a glyph is kept when its
// own record is sound and every component is, and refused, with the fault of
// whichever glyph it is, when one of them is not. A component already on the
// walk's path closes a cycle, and every glyph on the path is built from it.
func (c *glyphChecks) glyf(f *Face, t glyfTables, root int) *glyphFault {
	if v, ok := c.verdicts[root]; ok {
		return v
	}
	type frame struct {
		gid   int
		comps []int
		next  int
	}
	var path []frame
	onPath := map[int]bool{}
	// fail decides every glyph on the path, which are all built from the
	// glyph at fault.
	fail := func(v *glyphFault) *glyphFault {
		for _, fr := range path {
			c.verdicts[fr.gid] = v
		}
		return v
	}
	// enter decides gid as far as its own record goes, and for a composite
	// puts it on the path for its components to be decided. done is false
	// while it is on the path.
	enter := func(gid int) (v *glyphFault, done bool) {
		comps, err := f.glyfOwnFault(t, gid)
		if err != nil {
			v = &glyphFault{gid: gid, err: err}
			c.verdicts[gid] = v
			return v, true
		}
		if len(comps) == 0 {
			c.verdicts[gid] = nil
			return nil, true
		}
		path = append(path, frame{gid: gid, comps: comps})
		onPath[gid] = true
		return nil, false
	}
	if v, done := enter(root); done {
		return v
	}
	for len(path) > 0 {
		top := &path[len(path)-1]
		if top.next == len(top.comps) {
			c.verdicts[top.gid] = nil
			delete(onPath, top.gid)
			path = path[:len(path)-1]
			continue
		}
		comp := top.comps[top.next]
		top.next++
		if onPath[comp] {
			return fail(&glyphFault{gid: top.gid, err: fmt.Errorf("names glyph %d as a component, "+
				"which is built, through its components, from this glyph in turn", comp)})
		}
		if v, ok := c.verdicts[comp]; ok {
			if v != nil {
				return fail(v)
			}
			continue
		}
		if v, done := enter(comp); done && v != nil {
			return fail(v)
		}
	}
	return c.verdicts[root]
}

// glyfOwnFault decides a glyph of a TrueType face by its own record alone,
// and returns the glyphs it names as components, which are decided in turn.
func (f *Face) glyfOwnFault(t glyfTables, gid int) (comps []int, err error) {
	if f.varc != nil && f.varc.t.coverageIndex(gid) >= 0 {
		// Written into the subset as what VARC draws, under an allowance of
		// its own.
		work := int64(maxVarcWork)
		if _, err := f.varc.flatEntry(gid, &work); err != nil {
			return nil, fmt.Errorf("is a variable composite (VARC) that cannot be written out as a glyf outline: %w", err)
		}
		return nil, nil
	}
	return f.glyfComponents(t, gid, false)
}

// glyfComponents reads a glyph's glyf record whole and returns the glyphs it
// is built from: none for a simple glyph, and none for a glyph VARC composes,
// whose glyf entry a subset does not carry.
//
// With decided set, the glyph has been decided keepable already, and a simple
// glyph is not read again: its header says it names no components, and
// reading the rest is the decision over, at the cost of every point it has.
func (f *Face) glyfComponents(t glyfTables, gid int, decided bool) (comps []int, err error) {
	if f.varc != nil && f.varc.t.coverageIndex(gid) >= 0 {
		return nil, nil
	}
	b, err := t.entry(gid)
	if err != nil {
		return nil, err
	}
	if decided && (len(b) < 2 || int16(font.Be16(b, 0)) >= 0) {
		return nil, nil
	}
	g, err := decodeVarGlyph(b, t.numGlyphs)
	if err != nil {
		return nil, fmt.Errorf("cannot be read: %w", err)
	}
	for _, comp := range g.comps {
		if f.varc != nil && f.varc.t.coverageIndex(comp.glyph) >= 0 {
			return nil, fmt.Errorf("has glyph %d as a glyf component, which draws that glyph's own glyf "+
				"entry where the glyph itself draws its variable composite (VARC); a subset cannot hold "+
				"both under one glyph index", comp.glyph)
		}
		comps = append(comps, comp.glyph)
	}
	return comps, nil
}

// cffFault decides a glyph of a CFF face by running its charstring, as the face
// runs it to draw it, under an allowance of the glyph's own.
func (f *Face) cffFault(gid int) *glyphFault {
	ink := f.ink
	o := ink.outlines
	if gid >= len(o.charStrings) {
		return &glyphFault{gid: gid, err: fmt.Errorf("has no charstring, the CFF table holding %d", len(o.charStrings))}
	}
	r := t2Run{o: o, budget: font.NewBudget(maxFontWork)}
	// A seac looks its two glyphs up in the font's charset, which the reader
	// fills in lazily.
	ink.mu.Lock()
	_, ok := r.bounds(gid, false)
	ink.mu.Unlock()
	switch {
	case r.capped:
		return &glyphFault{gid: gid, err: fmt.Errorf("has a charstring that runs past the %d operators "+
			"HarfBuzz runs of one glyph", cffMaxOps)}
	case r.spent:
		return &glyphFault{gid: gid, err: errors.New("has a charstring that runs past the work one glyph may cost")}
	case !ok:
		return &glyphFault{gid: gid, err: errors.New("has a charstring that cannot be run: an operator " +
			"that runs off the stack, a subroutine that is not there, calls nested too deep, or a seac " +
			"naming a glyph that is not there or cannot be run itself")}
	}
	return nil
}

// usedVerdicts decides every glyph the face used, and .notdef. It returns a
// GlyphError naming the used glyphs that cannot be kept, or the error that
// refuses them all; notdef is false when .notdef cannot be kept and the face
// did not use it, and is to be written empty.
func (f *Face) usedVerdicts() (notdef bool, err error) {
	if err := f.subsettable(); err != nil {
		return false, err
	}
	n := f.prog.NumGlyphs
	used := make([]int, 0, len(f.used))
	for gid := range f.used {
		if gid >= 0 && gid < n {
			used = append(used, gid)
		}
	}
	sort.Ints(used)
	var refused *GlyphError
	for _, gid := range used {
		fault, err := f.glyphFault(gid)
		if err != nil {
			return false, err
		}
		if fault == nil {
			continue
		}
		if refused == nil {
			refused = &GlyphError{Err: fault.describe(gid)}
		}
		refused.GIDs = append(refused.GIDs, gid)
	}
	if refused != nil {
		return false, refused
	}
	if f.used[0] {
		return true, nil
	}
	fault, err := f.glyphFault(0)
	if err != nil {
		return false, err
	}
	return fault == nil, nil
}
