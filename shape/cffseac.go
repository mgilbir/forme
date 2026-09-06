package shape

import (
	"encoding/binary"
	"errors"

	"github.com/mgilbir/forme/font"
)

// The one way a CFF charstring names another glyph.
//
// CFF has no composite glyphs in the glyf sense, and the subsetter's comment
// used to say that a charstring reusing another shape does it "through seac or
// a subroutine, and both are carried along by keeping the subroutine INDEXes
// whole". Half of that is right. A subroutine is a piece of *this* charstring
// and travels with the INDEX; a seac is a reference to two other **glyphs**,
// and keeping every subroutine says nothing about them.
//
// Type 2 spells it as endchar reached with four or five arguments on the stack:
// the last four are adx, ady, bchar and achar, and the two chars are codes in
// the Adobe StandardEncoding — not glyph indices, and not codes in whatever
// encoding the font itself declares. So "Aacute" is drawn by naming "A" and
// "acute", and a subset that keeps Aacute and drops those two ships a
// charstring that draws nothing.
//
// # Why the stack has to be walked
//
// The four arguments may be pushed anywhere: by the charstring itself, by a
// subroutine it calls, or partly by each. Nothing short of tracking how deep
// the stack is when endchar is reached can tell a seac from an ordinary
// endchar, and an ordinary endchar is what almost every glyph ends with. The
// walk below tracks the depth and nothing else — it does not draw, does not
// keep coordinates, and stops at the first endchar.

// maxCharstringOps bounds one charstring's walk.
//
// A crafted font can write a subroutine that calls itself, and the depth bound
// alone does not stop a walk that returns and calls again. The number is far
// above any real charstring: the largest in the corpus is a few hundred
// operators.
const maxCharstringOps = 1 << 16

// cffSeac reports the two glyphs a charstring's seac names, as StandardEncoding
// codes, and whether it has one.
func cffSeac(code []byte, local, global [][]byte) (bchar, achar int, ok bool) {
	// frame is one charstring being read: the bytes, and how far in.
	type frame struct {
		code []byte
		at   int
	}
	stack := []frame{{code: code}}
	// depth is how many numbers are on the operand stack. The values of the
	// last four are what a seac needs, so those are kept and the rest counted.
	var operands []int
	push := func(v int) {
		operands = append(operands, v)
		if len(operands) > 48 {
			// The specification's own stack limit. A charstring that pushes
			// past it is malformed; dropping the oldest keeps the last four
			// right, which is all this reads.
			operands = operands[len(operands)-48:]
		}
	}

	for ops := 0; len(stack) > 0; ops++ {
		if ops > maxCharstringOps {
			return 0, 0, false
		}
		f := &stack[len(stack)-1]
		if f.at >= len(f.code) {
			stack = stack[:len(stack)-1]
			continue
		}
		v := int(f.code[f.at])
		switch {
		case v == 28:
			if f.at+3 > len(f.code) {
				return 0, 0, false
			}
			push(int(int16(binary.BigEndian.Uint16(f.code[f.at+1:]))))
			f.at += 3

		case v == 255:
			if f.at+5 > len(f.code) {
				return 0, 0, false
			}
			// A 16.16 fixed-point number. Only its integer part can matter
			// here, and a seac's arguments are integers.
			push(int(int32(binary.BigEndian.Uint32(f.code[f.at+1:]))) >> 16)
			f.at += 5

		case v >= 32 && v <= 246:
			push(v - 139)
			f.at++

		case v >= 247 && v <= 250:
			if f.at+2 > len(f.code) {
				return 0, 0, false
			}
			push((v-247)*256 + int(f.code[f.at+1]) + 108)
			f.at += 2

		case v >= 251 && v <= 254:
			if f.at+2 > len(f.code) {
				return 0, 0, false
			}
			push(-(v-251)*256 - int(f.code[f.at+1]) - 108)
			f.at += 2

		case v == 10 || v == 29: // callsubr, callgsubr
			idx := local
			if v == 29 {
				idx = global
			}
			if len(operands) == 0 {
				return 0, 0, false
			}
			n := operands[len(operands)-1] + font.CFFSubrBias(len(idx))
			operands = operands[:len(operands)-1]
			f.at++
			if n < 0 || n >= len(idx) || len(stack) >= maxCharstringDepth {
				return 0, 0, false
			}
			stack = append(stack, frame{code: idx[n]})

		case v == 11: // return
			stack = stack[:len(stack)-1]

		case v == 19 || v == 20: // hintmask, cntrmask
			// The mask's bytes follow, one bit per stem, and the operands not
			// yet consumed are an implicit vstem.
			stems := (len(operands) + 1) / 2
			operands = nil
			f.at += 1 + (stems+7)/8

		case v == 14: // endchar
			// Four or five: the fifth, when there is one, is the width that any
			// stack-clearing operator may carry.
			if len(operands) >= 4 {
				last := operands[len(operands)-4:]
				return last[2], last[3], true
			}
			return 0, 0, false

		case v == 12:
			// A two-byte operator. None of them is endchar and none pushes, so
			// what matters is only that the stack clears and the reading goes
			// on past both bytes.
			if f.at+2 > len(f.code) {
				return 0, 0, false
			}
			operands = nil
			f.at += 2

		default:
			// Every other one-byte operator: a path or hint operator, all of
			// which clear the stack.
			operands = nil
			f.at++
		}
	}
	return 0, 0, false
}

// maxCharstringDepth is the subroutine nesting Type 2 allows.
const maxCharstringDepth = 10

// cffSeacClosure adds to keep every glyph a kept charstring's seac names.
//
// It is the CFF answer to the glyf subsetter's component closure, and it exists
// for the same reason: a subset that keeps a glyph and drops what the glyph is
// drawn from ships a glyph that draws nothing. It is applied until nothing more
// is added, because the base of one accented letter may itself be one.
func cffSeacClosure(cff []byte, keep []bool) error {
	charStrings, local, global, sidToGID, err := cffSeacTables(cff, len(keep))
	if err != nil || charStrings == nil {
		return err
	}
	for again := true; again; {
		again = false
		for gid, k := range keep {
			if !k || gid >= len(charStrings) {
				continue
			}
			bchar, achar, ok := cffSeac(charStrings[gid], local, global)
			if !ok {
				continue
			}
			for _, code := range []int{bchar, achar} {
				if code < 0 || code > 0xFF {
					continue
				}
				name, named := font.StandardEncodingNames[byte(code)]
				if !named {
					continue
				}
				sid, standard := font.CFFStandardSID(name)
				if !standard {
					continue
				}
				g, have := sidToGID[sid]
				if !have || g < 0 || g >= len(keep) || keep[g] {
					continue
				}
				keep[g] = true
				again = true
			}
		}
	}
	return nil
}

// cffSeacTables reads what the walk above needs: the charstrings, the two
// subroutine INDEXes, and the charset inverted so a SID names a glyph.
//
// A CID-keyed font has none of this to answer with — its charset holds CIDs
// rather than names, and seac is a non-CID construct — so it comes back with
// nothing and no error.
func cffSeacTables(cff []byte, nGlyphs int) (charStrings, local, global [][]byte,
	sidToGID map[int]int, err error) {

	top, err := topDictOf(cff)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	csOff, charsetOff, privOff, privSize := -1, 0, -1, 0
	for _, e := range top {
		switch {
		case e.op == opROS:
			return nil, nil, nil, nil, nil // CID-keyed: no names, no seac
		case e.op == opCharStrings && len(e.operands) == 1:
			csOff = e.operands[0]
		case e.op == opCharset && len(e.operands) == 1:
			charsetOff = e.operands[0]
		case e.op == opPrivate && len(e.operands) == 2:
			privSize, privOff = e.operands[0], e.operands[1]
		}
	}
	if csOff <= 0 {
		return nil, nil, nil, nil, errors.New("fonts: the CFF names no CharStrings")
	}
	cs, err := readCFFIndex(cff, csOff)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	// The global subroutines are the fourth INDEX of the header, which
	// topDictOf has already walked past; finding them means walking the first
	// three again.
	if g, err := cffGlobalSubrs(cff); err == nil {
		global = g
	}
	if privSize > 0 && privOff > 0 && privOff+privSize <= len(cff) {
		privOps, _, err := parseCFFDict(cff[privOff : privOff+privSize])
		if err == nil {
			for _, e := range privOps {
				if e.op == opSubrs && len(e.operands) == 1 && e.operands[0] > 0 {
					if idx, err := readCFFIndex(cff, privOff+e.operands[0]); err == nil {
						local = idx.items
					}
				}
			}
		}
	}

	sids, err := cffCharsetSIDs(cff, charsetOff, nGlyphs)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	sidToGID = make(map[int]int, len(sids))
	for gid, sid := range sids {
		if _, seen := sidToGID[sid]; !seen {
			sidToGID[sid] = gid
		}
	}
	return cs.items, local, global, sidToGID, nil
}

// cffGlobalSubrs is the fourth INDEX of a CFF: Name, Top DICT, String, Global
// Subr.
func cffGlobalSubrs(cff []byte) ([][]byte, error) {
	if len(cff) < 4 {
		return nil, errors.New("fonts: the CFF is too short to hold a header")
	}
	at := int(cff[2]) // hdrSize
	for i := 0; i < 3; i++ {
		idx, err := readCFFIndex(cff, at)
		if err != nil {
			return nil, err
		}
		at = idx.end
	}
	idx, err := readCFFIndex(cff, at)
	if err != nil {
		return nil, err
	}
	return idx.items, nil
}

// cffCharsetSIDs is the charset read as what it is: the SID of each glyph.
//
// Glyph 0 is .notdef, SID 0, and is never listed. A predefined charset is the
// identity — ISOAdobe numbers its glyphs by SID — and Expert and Expert Subset
// are tables this does not carry, so a font naming one gets the identity too:
// the worst that costs is a seac whose components are not found, which is what
// happened to every font before this.
func cffCharsetSIDs(cff []byte, off, nGlyphs int) ([]int, error) {
	sids := make([]int, nGlyphs)
	for g := range sids {
		sids[g] = g
	}
	if off <= 2 {
		return sids, nil
	}
	if off >= len(cff) {
		return nil, errors.New("fonts: the CFF charset lies outside the font")
	}
	c := cff[off:]
	switch c[0] {
	case 0:
		for g := 1; g < nGlyphs; g++ {
			at := 1 + 2*(g-1)
			if at+2 > len(c) {
				return nil, errors.New("fonts: truncated CFF charset")
			}
			sids[g] = int(binary.BigEndian.Uint16(c[at:]))
		}
		return sids, nil
	case 1, 2:
		step := 3
		if c[0] == 2 {
			step = 4
		}
		g, at := 1, 1
		for g < nGlyphs {
			if at+step > len(c) {
				return nil, errors.New("fonts: truncated CFF charset")
			}
			first := int(binary.BigEndian.Uint16(c[at:]))
			nLeft := int(c[at+2])
			if step == 4 {
				nLeft = int(binary.BigEndian.Uint16(c[at+2:]))
			}
			for i := 0; i <= nLeft && g < nGlyphs; i++ {
				sids[g] = first + i
				g++
			}
			at += step
		}
		return sids, nil
	}
	return nil, errors.New("fonts: the CFF charset is in no format the specification defines")
}
